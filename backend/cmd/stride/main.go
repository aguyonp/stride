// Command stride runs the Stride API with the embedded SPA.
//
//	stride [serve]      start the HTTP server (default)
//	stride healthcheck  GET /health/ready on the local port, exit 0/1 (container healthcheck)
//	stride version      print the version
//	stride doctor       full SQLite integrity and foreign-key check, exit 0/1
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/valkey-io/valkey-go"

	"stride/internal/admin"
	"stride/internal/apimeter"
	"stride/internal/applog"
	"stride/internal/auth"
	"stride/internal/buildinfo"
	"stride/internal/cache"
	"stride/internal/database"
	"stride/internal/domain"
	"stride/internal/env"
	"stride/internal/feedback"
	"stride/internal/httpapi"
	"stride/internal/logx"
	"stride/internal/observability"
	"stride/internal/provider/strava"
	"stride/internal/provider/weather"
	"stride/internal/scheduler"
	"stride/internal/secret"
	"stride/internal/sse"
	"stride/internal/store"
	"stride/internal/web"
)

func main() {
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "serve":
		if err := run(); err != nil {
			fmt.Fprintln(os.Stderr, "❌", err)
			os.Exit(1)
		}
	case "healthcheck":
		os.Exit(healthcheck())
	case "version", "--version", "-v":
		fmt.Println(buildinfo.Get().Label)
	case "doctor":
		os.Exit(doctor())
	default:
		fmt.Fprintf(os.Stderr, "commande inconnue %q (attendu : serve | healthcheck | version | doctor)\n", cmd)
		os.Exit(2)
	}
}

func healthcheck() int {
	port := "8080"
	if addr := os.Getenv("STRIDE_LISTEN_ADDR"); addr != "" {
		if _, p, err := net.SplitHostPort(addr); err == nil && p != "" {
			port = p
		}
	}
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get("http://127.0.0.1:" + port + "/health/ready")
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck: HTTP", resp.StatusCode)
		return 1
	}
	return 0
}

// doctor runs the full integrity and foreign-key checks that boot skips.
func doctor() int {
	cfg, err := env.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "❌", err)
		return 1
	}
	ctx := context.Background()
	st, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "❌", err)
		return 1
	}
	defer st.Close()
	problems, err := database.CheckIntegrity(ctx, st.DB)
	if err != nil {
		fmt.Fprintln(os.Stderr, "❌", err)
		return 1
	}
	for _, p := range problems {
		fmt.Println("⚠️", p)
	}
	if len(problems) > 0 {
		fmt.Printf("❌ %d problème(s) détecté(s) dans %s\n", len(problems), cfg.DBPath)
		return 1
	}
	fmt.Println("✅ Base SQLite intègre :", cfg.DBPath)
	return 0
}

// warnForeignKeys logs orphan rows in the background: they never block the boot.
func warnForeignKeys(ctx context.Context, st *store.Store, log *slog.Logger) {
	go func() {
		v, err := database.ForeignKeyViolations(ctx, st.DB)
		if err != nil {
			log.Warn("⚠️ Contrôle des clés étrangères impossible", "err", err.Error())
			return
		}
		if len(v) > 0 {
			log.Warn("⚠️ Lignes orphelines détectées, lancez « stride doctor »", "nombre", len(v), "exemple", v[0])
		}
	}()
}

func run() error {
	startedAt := time.Now()
	cfg, err := env.Load()
	if err != nil {
		return err
	}
	log := logx.New(logx.Options{Level: cfg.LogLevel, Format: cfg.LogFormat})
	slog.SetDefault(log)
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := ensureWritable(cfg.DataDir); err != nil {
		return fmt.Errorf("STRIDE_DATA_DIR %q non inscriptible (droits du volume ?) : %w", cfg.DataDir, err)
	}
	st, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return fmt.Errorf("base SQLite %s : %w", cfg.DBPath, err)
	}
	defer st.Close()
	warnForeignKeys(ctx, st, log)
	if err := st.EnsureSetting(ctx, store.SettingRegistrationMode, cfg.RegistrationMode, time.Now()); err != nil {
		return err
	}

	metrics := observability.New()
	kv, cacheBackend, err := openCache(ctx, cfg, log)
	if err != nil {
		return err
	}
	var valkeyClient valkey.Client
	if vk, ok := kv.(*cache.Valkey); ok {
		valkeyClient = vk.Client()
	}
	kv = cache.NewInstrumented(kv, cacheBackend, metrics)
	defer kv.Close()

	sealer, err := secret.New(cfg.MasterKey, cfg.MasterKeyPrevious)
	if err != nil {
		return err
	}

	logs := applog.New(st.DB, log)
	defer logs.Close()
	meter := apimeter.NewLocal(apimeter.Options{Log: log, Recent: recentErrors(ctx, st, log), Sink: persistErrors(st), OnCall: auditCall(logs),
		Metrics: metrics})
	defer meter.Close()

	var stravaAPI strava.API
	if cfg.StravaEnabled() {
		stravaAPI = strava.NewClient(strava.Config{ClientID: cfg.StravaClientID, ClientSecret: cfg.StravaClientSecret, Meter: meter})
	}
	var wx weather.Provider
	if cfg.WeatherProvider != "off" {
		if cfg.WeatherProvider != "open-meteo" {
			log.Warn("⚠️ STRIDE_WEATHER inconnu, Open-Meteo utilisé", "valeur", cfg.WeatherProvider)
		}
		wx = weather.NewOpenMeteo(weather.OpenMeteoOptions{Meter: meter})
	}

	sseHub := sse.NewHub(sse.Options{
		Metrics: metrics,
		Valkey:  valkeyClient,
		Log:     log,
	})
	defer sseHub.Close()

	dom, err := domain.New(domain.Deps{
		Env:     cfg,
		Store:   st,
		Cache:   kv,
		Sealer:  sealer,
		Meter:   meter,
		Strava:  stravaAPI,
		Weather: wx,
		SSE:     sseHub,
		Metrics: metrics,
		Log:     log,
	})
	if err != nil {
		return err
	}
	if err := dom.DedupOnce(ctx); err != nil {
		log.Warn("⚠️ Dédoublonnage des activités en échec", "err", err.Error())
	}
	dom.SeedFirstAdmin(ctx)
	authSvc, err := auth.New(auth.Options{Env: cfg, Store: st, Cache: kv, Log: log,
		OnUserCreated: dom.OnUserCreated, OnUserDeleted: dom.PurgeUserCache})
	if err != nil {
		return err
	}
	handler := httpapi.New(httpapi.Deps{
		Store:  st,
		Cache:  kv,
		Auth:   authSvc,
		Domain: dom,
		Log:    log,
		Logs:   logs,
		HTTPS:  cfg.CookieSecure,
		Static: web.New(),
		Admin: admin.New(admin.Deps{Store: st, Cache: kv, Meter: meter, Auth: authSvc, PublicURL: cfg.PublicURL, Log: log,
			Domain: dom, SSE: sseHub, Logs: logs, DBPath: cfg.DBPath, Runtime: admin.RuntimeFromEnv(cfg, cacheBackend),
			StartedAt: startedAt, Maintenance: purgeExpired(st, dom, logs)}),
		SSE:      sseHub,
		Metrics:  metrics,
		Feedback: feedback.New(feedback.Deps{DB: st.DB, Auth: authSvc, Log: log}),
	})

	sched := scheduler.New(scheduler.Options{
		Hour: cfg.SyncHour, JitterMin: cfg.SyncJitterMin, JitterMax: cfg.SyncJitterMax, Location: loc, Log: log,
		Users:    dom.ConnectedUserIDs,
		LastSync: dom.LastSyncAt,
		Sync:     dom.MorningSync,
		Purge:    maintenance(st, dom, logs, log),
	})

	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      150 * time.Second, // LLM calls may take up to 120 s
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
	ln, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("écoute sur %s impossible : %w", cfg.ListenAddr, err)
	}
	var metricsLn net.Listener
	if cfg.MetricsAddr != "" {
		if metricsLn, err = net.Listen("tcp", cfg.MetricsAddr); err != nil {
			return fmt.Errorf("écoute des métriques sur %s impossible : %w", cfg.MetricsAddr, err)
		}
	}

	mode, _ := st.RegistrationMode(ctx)
	absDB, _ := filepath.Abs(cfg.DBPath)
	log.Info(fmt.Sprintf("🚀 Stride %s démarré", buildinfo.Get().Label),
		"écoute", cfg.ListenAddr, "urlPublique", cfg.PublicURL, "db", absDB, "cache", cacheBackend,
		"inscription", mode, "strava", onOff(stravaAPI != nil), "météo", onOff(wx != nil),
		"tz", cfg.Timezone, "syncMatinale", fmt.Sprintf("%02d:00", cfg.SyncHour), "frontend", web.HasFrontend(),
		"métriques", onOff(cfg.MetricsAddr != ""))

	var wg sync.WaitGroup
	schedCtx, stopSched := context.WithCancel(context.Background())
	wg.Add(1)
	go func() {
		defer wg.Done()
		sched.Run(schedCtx)
	}()
	errCh := make(chan error, 2)
	serve := func(s *http.Server, l net.Listener) {
		if err := s.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}
	go serve(srv, ln)
	metricsSrv := serveMetrics(metricsLn, metrics, serve)

	var serveErr error
	select {
	case serveErr = <-errCh:
		if serveErr != nil {
			log.Error("❌ Serveur HTTP arrêté sur erreur", "err", serveErr.Error())
		}
	case <-ctx.Done():
		log.Info("🛑 Arrêt demandé, fermeture des connexions…")
	}
	shCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shCtx); err != nil {
		log.Warn("⚠️ Arrêt forcé", "err", err.Error())
	}
	if metricsSrv != nil {
		_ = metricsSrv.Shutdown(shCtx)
	}
	stopSched()
	wg.Wait()
	dom.Close(shCtx)
	log.Info("🛑 Serveur arrêté")
	return serveErr
}

// serveMetrics exposes GET /metrics on its own listener (STRIDE_METRICS_ADDR, never the public one).
func serveMetrics(ln net.Listener, m *observability.Metrics, serve func(*http.Server, net.Listener)) *http.Server {
	if ln == nil {
		return nil
	}
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", m.Handler())
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 30 * time.Second}
	go serve(srv, ln)
	return srv
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func ensureWritable(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".write-test-*")
	if err != nil {
		return err
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}

// openCache connects to Valkey with backoff (it may still be starting) or falls back to memory when not configured.
func openCache(ctx context.Context, cfg env.Config, log *slog.Logger) (cache.Cache, string, error) {
	if cfg.ValkeyURL == "" {
		return cache.NewMemory(), "memory", nil
	}
	delay := 500 * time.Millisecond
	deadline := time.Now().Add(60 * time.Second)
	for attempt := 1; ; attempt++ {
		v, err := cache.NewValkey(cfg.ValkeyURL)
		if err == nil {
			pctx, cancel := context.WithTimeout(ctx, 2*time.Second)
			err = v.Ping(pctx)
			cancel()
			if err == nil {
				return v, "valkey", nil
			}
			v.Close()
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return nil, "", fmt.Errorf("valkey injoignable après %d tentatives : %w", attempt, err)
		}
		log.Warn("⏳ Valkey pas encore prêt, nouvelle tentative", "tentative", attempt, "dans", delay.String())
		select {
		case <-ctx.Done():
			return nil, "", ctx.Err()
		case <-time.After(delay):
		}
		delay = min(delay*2, 5*time.Second)
	}
}

func recentErrors(ctx context.Context, st *store.Store, log *slog.Logger) []apimeter.ErrorEvent {
	evs, err := st.RecentAPIEvents(ctx, 50)
	if err != nil {
		log.Warn("⚠️ Historique des erreurs API illisible", "err", err.Error())
		return nil
	}
	out := make([]apimeter.ErrorEvent, 0, len(evs))
	for _, e := range evs {
		out = append(out, apimeter.ErrorEvent{Time: e.At, Provider: e.Provider, UserID: e.UserID, Endpoint: e.Endpoint, Status: e.Status, Message: e.Message})
	}
	return out
}

func persistErrors(st *store.Store) func(context.Context, []apimeter.ErrorEvent) error {
	return func(ctx context.Context, evs []apimeter.ErrorEvent) error {
		rows := make([]store.APIEvent, 0, len(evs))
		for _, e := range evs {
			rows = append(rows, store.APIEvent{At: e.Time, Provider: e.Provider, UserID: e.UserID, Endpoint: e.Endpoint, Status: e.Status, Message: e.Message})
		}
		return st.InsertAPIEvents(ctx, rows)
	}
}

// purgeExpired removes expired sessions, old invitations and API events, purged logs, finished jobs and
// dead share links; it backs the hourly maintenance and the admin « Purger » action.
func purgeExpired(st *store.Store, dom *domain.Services, logs *applog.Log) func(context.Context) (map[string]int64, error) {
	const keep = 7 * 24 * time.Hour
	return func(ctx context.Context) (map[string]int64, error) {
		now := time.Now()
		sessions, err1 := st.PurgeExpiredSessions(ctx, now)
		invitations, err2 := st.PurgeInvitations(ctx, now, keep)
		events, err3 := st.PurgeAPIEvents(ctx, now.Add(-keep))
		auditRows, err4 := logs.Purge(ctx, now)
		jobs, links, err5 := dom.PurgeStale(ctx)
		return map[string]int64{"sessions": sessions, "invitations": invitations, "apiEvents": events, "logs": auditRows,
			"jobs": jobs, "shareLinks": links}, errors.Join(err1, err2, err3, err4, err5)
	}
}

func maintenance(st *store.Store, dom *domain.Services, logs *applog.Log, log *slog.Logger) func(context.Context) error {
	purge := purgeExpired(st, dom, logs)
	return func(ctx context.Context) error {
		n, err := purge(ctx)
		if n["sessions"]+n["invitations"]+n["apiEvents"]+n["logs"]+n["jobs"]+n["shareLinks"] > 0 {
			log.Info("🧹 Maintenance", "sessionsExpirées", n["sessions"], "invitations", n["invitations"], "événementsAPI", n["apiEvents"],
				"journaux", n["logs"], "tâches", n["jobs"], "liensPartage", n["shareLinks"])
		}
		return err
	}
}

// auditCall records outbound provider calls; error texts are never stored (they may embed URLs with secrets).
func auditCall(logs *applog.Log) func(apimeter.Call) {
	return func(c apimeter.Call) {
		e := applog.Entry{Category: applog.CategoryRequest, Method: "OUT", Target: c.Endpoint, Provider: c.Provider,
			Status: c.Status, DurationMs: c.Duration.Milliseconds()}
		if c.UserID > 0 {
			e.UserID = &c.UserID
		}
		switch {
		case c.Status == http.StatusTooManyRequests:
			e.Level, e.Message = applog.LevelError, "limite de débit atteinte"
		case c.Err != nil && errors.Is(c.Err, context.DeadlineExceeded):
			e.Level, e.Message = applog.LevelError, "délai dépassé"
		case c.Err != nil && c.Status == 0:
			e.Level, e.Message = applog.LevelError, "erreur réseau"
		case c.Status >= 400:
			e.Level = applog.LevelError
		}
		logs.Add(e)
	}
}
