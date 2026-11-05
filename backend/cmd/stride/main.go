// Command stride runs the Stride API with the embedded SPA.
//
//	stride [serve]      start the HTTP server (default)
//	stride healthcheck  GET /health/ready on the local port, exit 0/1 (container healthcheck)
//	stride version      print the version
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

	"stride/internal/admin"
	"stride/internal/apimeter"
	"stride/internal/auth"
	"stride/internal/cache"
	"stride/internal/domain"
	"stride/internal/env"
	"stride/internal/httpapi"
	"stride/internal/logx"
	"stride/internal/provider/strava"
	"stride/internal/provider/weather"
	"stride/internal/scheduler"
	"stride/internal/secret"
	"stride/internal/store"
	"stride/internal/web"
)

// version is set at build time: -ldflags "-X main.version=2.0.0".
var version = "dev"

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
		fmt.Println(version)
	default:
		fmt.Fprintf(os.Stderr, "commande inconnue %q (attendu : serve | healthcheck | version)\n", cmd)
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

func run() error {
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
	if err := st.EnsureSetting(ctx, store.SettingRegistrationMode, cfg.RegistrationMode, time.Now()); err != nil {
		return err
	}

	kv, cacheBackend, err := openCache(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer kv.Close()

	sealer, err := secret.New(cfg.MasterKey, cfg.MasterKeyPrevious)
	if err != nil {
		return err
	}

	meter := apimeter.NewLocal(apimeter.Options{Log: log, Recent: recentErrors(ctx, st, log), Sink: persistErrors(st)})
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

	dom, err := domain.New(domain.Deps{Env: cfg, Store: st, Cache: kv, Sealer: sealer, Meter: meter, Strava: stravaAPI, Weather: wx, Log: log})
	if err != nil {
		return err
	}
	if err := dom.DedupOnce(ctx); err != nil {
		log.Warn("⚠️ Dédoublonnage des activités en échec", "err", err.Error())
	}
	authSvc, err := auth.New(auth.Options{Env: cfg, Store: st, Cache: kv, Log: log,
		OnUserCreated: dom.OnUserCreated, OnUserDeleted: dom.PurgeUserCache})
	if err != nil {
		return err
	}
	handler := httpapi.New(httpapi.Deps{
		Store: st, Cache: kv, Auth: authSvc, Domain: dom, Log: log, Version: version, HTTPS: cfg.CookieSecure, Static: web.New(),
		Admin: admin.New(admin.Deps{Store: st, Cache: kv, Meter: meter, Auth: authSvc, PublicURL: cfg.PublicURL, Log: log}),
	})

	sched := scheduler.New(scheduler.Options{
		Hour: cfg.SyncHour, JitterMin: cfg.SyncJitterMin, JitterMax: cfg.SyncJitterMax, Location: loc, Log: log,
		Users:    dom.ConnectedUserIDs,
		LastSync: dom.LastSyncAt,
		Sync:     dom.MorningSync,
		Purge:    maintenance(st, log),
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

	mode, _ := st.RegistrationMode(ctx)
	absDB, _ := filepath.Abs(cfg.DBPath)
	log.Info(fmt.Sprintf("🚀 Stride %s démarré", version),
		"écoute", cfg.ListenAddr, "urlPublique", cfg.PublicURL, "db", absDB, "cache", cacheBackend,
		"inscription", mode, "strava", onOff(stravaAPI != nil), "météo", onOff(wx != nil),
		"tz", cfg.Timezone, "syncMatinale", fmt.Sprintf("%02d:00", cfg.SyncHour), "frontend", web.HasFrontend())

	var wg sync.WaitGroup
	schedCtx, stopSched := context.WithCancel(context.Background())
	wg.Add(1)
	go func() {
		defer wg.Done()
		sched.Run(schedCtx)
	}()
	errCh := make(chan error, 1)
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

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
	stopSched()
	wg.Wait()
	dom.Wait()
	log.Info("🛑 Serveur arrêté")
	return serveErr
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
		return cache.NewMemory(), "mémoire", nil
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

func maintenance(st *store.Store, log *slog.Logger) func(context.Context) error {
	const keep = 7 * 24 * time.Hour
	return func(ctx context.Context) error {
		now := time.Now()
		sessions, err1 := st.PurgeExpiredSessions(ctx, now)
		invitations, err2 := st.PurgeInvitations(ctx, now, keep)
		events, err3 := st.PurgeAPIEvents(ctx, now.Add(-keep))
		if sessions+invitations+events > 0 {
			log.Info("🧹 Maintenance", "sessionsExpirées", sessions, "invitations", invitations, "événementsAPI", events)
		}
		return errors.Join(err1, err2, err3)
	}
}
