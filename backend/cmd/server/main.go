// Commande server : API Patella + SPA embarqué.
//
//	server              démarre le serveur HTTP
//	server healthcheck  GET http://127.0.0.1:$PORT/api/health → exit 0/1 (HEALTHCHECK Docker)
//	server version      affiche la version
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
	"strconv"
	"syscall"
	"time"
	_ "time/tzdata" // fuseaux embarqués (image distroless sans tzdata)

	"patella/internal/analysis"
	"patella/internal/config"
	"patella/internal/fitdata"
	"patella/internal/garmin"
	"patella/internal/httpapi"
	"patella/internal/logx"
	"patella/internal/report"
	"patella/internal/web"
)

// version est injectée au build : -ldflags "-X main.version=1.0.0".
var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "healthcheck":
			os.Exit(healthcheck())
		case "version", "--version", "-version", "-v":
			fmt.Println(appVersion())
			return
		default:
			fmt.Fprintf(os.Stderr, "commande inconnue %q (attendu : healthcheck | version)\n", os.Args[1])
			os.Exit(2)
		}
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "❌", err)
		os.Exit(1)
	}
}

func appVersion() string {
	if version == "dev" {
		if v := os.Getenv("APP_VERSION"); v != "" {
			return v
		}
	}
	return version
}

func healthcheck() int {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	if _, err := strconv.Atoi(port); err != nil {
		port = "8080"
	}
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get("http://127.0.0.1:" + port + "/api/health")
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
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := logx.New(logx.Options{Level: cfg.LogLevel, Format: cfg.LogFormat})
	slog.SetDefault(log)

	if err := ensureDataDir(cfg.DataDir); err != nil {
		log.Error("❌ Répertoire de données inutilisable", "dataDir", cfg.DataDir, "err", err.Error())
		return fmt.Errorf("DATA_DIR %q non inscriptible (droits du volume ?) : %w", cfg.DataDir, err)
	}
	settings, err := config.NewStore(cfg.DataDir, cfg)
	if err != nil {
		return err
	}

	g := garmin.New(garmin.Options{
		DataDir:  cfg.DataDir,
		Key:      func() string { k, _ := settings.APIKey(); return k },
		Logger:   log,
		Location: settings.Location,
	})
	svc := &analysis.Service{
		G:        g,
		FIT:      fitdata.NewStore(g, log),
		Settings: settings,
		Reporter: report.Reporter{},
		Log:      log,
	}
	srvAPI := &httpapi.Server{
		Cfg:      cfg,
		Settings: settings,
		G:        g,
		Svc:      svc,
		Log:      log,
		Version:  appVersion(),
		Static:   web.New(),
	}

	_, keySrc := settings.APIKey()
	keyDesc := keySrc
	if keySrc == "env" {
		keyDesc = "env (" + cfg.EnvAPIKeyVar + ")"
	}
	auth := "off"
	if cfg.AuthEnabled() {
		auth = "on"
	}
	envFile := "aucun"
	if cfg.EnvFileLoaded != "" {
		envFile = fmt.Sprintf("%s (%d variables)", cfg.EnvFileLoaded, cfg.EnvFileVars)
	}
	absData, _ := filepath.Abs(cfg.DataDir)
	log.Info(fmt.Sprintf("🚀 Patella %s démarré sur :%d", appVersion(), cfg.Port),
		"dataDir", absData, "cléGarmin", keyDesc, "auth", auth, "tz", settings.Timezone(),
		"envFile", envFile, "frontend", web.HasFrontend())
	if keySrc == "none" {
		log.Warn("🔑 Aucune clé Garmin configurée : renseignez-la dans Réglages ou via GARMIN_API_TOKEN")
	} else {
		log.Info("🔑 Clé Garmin chargée", "source", keyDesc)
	}

	srv := &http.Server{
		Addr:              net.JoinHostPort("", strconv.Itoa(cfg.Port)),
		Handler:           srvAPI.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		if err != nil {
			log.Error("❌ Serveur HTTP arrêté sur erreur", "err", err.Error())
			return err
		}
		return nil
	case <-ctx.Done():
	}
	log.Info("🛑 Arrêt demandé, fermeture des connexions…")
	shCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shCtx); err != nil {
		log.Warn("⚠️ Arrêt forcé", "err", err.Error())
	}
	log.Info("🛑 Serveur arrêté")
	return nil
}

// ensureDataDir crée DATA_DIR (0700) et vérifie qu'il est inscriptible.
func ensureDataDir(dir string) error {
	if err := os.MkdirAll(filepath.Join(dir, "cache"), 0o700); err != nil {
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
