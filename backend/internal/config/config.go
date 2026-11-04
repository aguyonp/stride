// Package config charge la configuration (variables d'environnement, fichier .env)
// et gère les réglages persistés (settings.json : clé API saisie dans l'UI, fuseau, profil).
package config

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"
)

// DefaultTimezone est le fuseau par défaut de l'application.
const DefaultTimezone = "Europe/Paris"

// Config regroupe la configuration issue de l'environnement.
type Config struct {
	Port         int
	DataDir      string
	Timezone     string
	LogLevel     string
	LogFormat    string
	AuthUser     string
	AuthPassword string
	// EnvAPIKey est la clé Garmin trouvée dans l'environnement (jamais loggée).
	EnvAPIKey string
	// EnvAPIKeyVar est le nom de la variable d'où provient la clé ("" si aucune).
	EnvAPIKeyVar string
	// EnvFileLoaded est le chemin du fichier .env chargé ("" si aucun).
	EnvFileLoaded string
	// EnvFileVars est le nombre de variables effectivement injectées depuis le .env.
	EnvFileVars int
}

// AuthEnabled indique si l'authentification HTTP Basic est active.
func (c Config) AuthEnabled() bool { return c.AuthUser != "" && c.AuthPassword != "" }

// APIKeyEnvVars liste, par ordre de priorité, les variables acceptées pour la clé Garmin.
var APIKeyEnvVars = []string{"GARMIN_API_TOKEN", "GARMIN_PERSONAL_API_KEY", "GARMIN_API_KEY"}

// Load lit le fichier .env (ENV_FILE ou ./.env s'il existe) sans écraser les variables
// déjà définies, puis construit la configuration.
func Load() (Config, error) {
	var cfg Config
	envFile := os.Getenv("ENV_FILE")
	explicit := envFile != ""
	if envFile == "" {
		envFile = ".env"
	}
	n, err := LoadDotEnv(envFile)
	switch {
	case err == nil:
		cfg.EnvFileLoaded = envFile
		cfg.EnvFileVars = n
	case errors.Is(err, fs.ErrNotExist) && !explicit:
		// pas de .env local : normal en Docker
	default:
		return cfg, fmt.Errorf("lecture du fichier d'environnement %q : %w", envFile, err)
	}

	cfg.Port = 8080
	if p := strings.TrimSpace(os.Getenv("PORT")); p != "" {
		v, err := strconv.Atoi(p)
		if err != nil || v <= 0 || v > 65535 {
			return cfg, fmt.Errorf("PORT invalide : %q", p)
		}
		cfg.Port = v
	}
	cfg.DataDir = envOr("DATA_DIR", "./data")
	cfg.Timezone = firstNonEmpty(os.Getenv("APP_TIMEZONE"), os.Getenv("TZ"), DefaultTimezone)
	if _, err := time.LoadLocation(cfg.Timezone); err != nil {
		cfg.Timezone = DefaultTimezone
	}
	cfg.LogLevel = envOr("LOG_LEVEL", "info")
	cfg.LogFormat = envOr("LOG_FORMAT", "pretty")
	cfg.AuthUser = os.Getenv("APP_AUTH_USER")
	cfg.AuthPassword = os.Getenv("APP_AUTH_PASSWORD")
	for _, name := range APIKeyEnvVars {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			cfg.EnvAPIKey = v
			cfg.EnvAPIKeyVar = name
			break
		}
	}
	return cfg, nil
}

// LoadDotEnv charge un fichier KEY=VALUE dans l'environnement du process, sans écraser
// les variables existantes. Retourne le nombre de variables injectées.
// Gère : commentaires (#), lignes vides, préfixe "export ", guillemets simples/doubles,
// commentaires en fin de ligne pour les valeurs non quotées (" #").
func LoadDotEnv(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	vars, err := ParseDotEnv(bufio.NewScanner(f))
	if err != nil {
		return 0, err
	}
	n := 0
	for _, kv := range vars {
		if _, exists := os.LookupEnv(kv[0]); exists {
			continue
		}
		if err := os.Setenv(kv[0], kv[1]); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// ParseDotEnv analyse le contenu d'un .env et retourne les paires dans l'ordre.
func ParseDotEnv(sc *bufio.Scanner) ([][2]string, error) {
	var out [][2]string
	line := 0
	for sc.Scan() {
		line++
		s := strings.TrimSpace(sc.Text())
		if line == 1 {
			s = strings.TrimPrefix(s, "\ufeff")
		}
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		s = strings.TrimPrefix(s, "export ")
		k, v, ok := strings.Cut(s, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if k == "" || strings.ContainsAny(k, " \t") {
			continue
		}
		v = strings.TrimSpace(v)
		switch {
		case len(v) >= 2 && v[0] == '"':
			if end := strings.LastIndexByte(v, '"'); end > 0 {
				v = v[1:end]
				v = strings.NewReplacer(`\n`, "\n", `\"`, `"`, `\\`, `\`).Replace(v)
			}
		case len(v) >= 2 && v[0] == '\'':
			if end := strings.LastIndexByte(v, '\''); end > 0 {
				v = v[1:end]
			}
		default:
			if i := strings.Index(v, " #"); i >= 0 {
				v = strings.TrimSpace(v[:i])
			}
		}
		out = append(out, [2]string{k, v})
	}
	return out, sc.Err()
}

func envOr(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
