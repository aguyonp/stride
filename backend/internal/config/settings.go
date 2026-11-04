package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Profile regroupe les seuils personnels (voir §3 / §5 du contrat).
type Profile struct {
	InjuredSide                string  `json:"injuredSide"`
	CadenceTargetMin           float64 `json:"cadenceTargetMin"`
	CadenceTargetMax           float64 `json:"cadenceTargetMax"`
	GctBalanceAlertPct         float64 `json:"gctBalanceAlertPct"`
	VerticalOscillationAlertCm float64 `json:"verticalOscillationAlertCm"`
	DescentAlertM              float64 `json:"descentAlertM"`
	RhrAlertDeltaBpm           float64 `json:"rhrAlertDeltaBpm"`
	Pace10kMinSecPerKm         float64 `json:"pace10kMinSecPerKm"`
	Pace10kMaxSecPerKm         float64 `json:"pace10kMaxSecPerKm"`
	PaceMarathonMinSecPerKm    float64 `json:"paceMarathonMinSecPerKm"`
	PaceMarathonMaxSecPerKm    float64 `json:"paceMarathonMaxSecPerKm"`
}

// DefaultProfile retourne le profil par défaut (genou gauche).
func DefaultProfile() Profile {
	return Profile{
		InjuredSide:                "left",
		CadenceTargetMin:           175,
		CadenceTargetMax:           182,
		GctBalanceAlertPct:         1.5,
		VerticalOscillationAlertCm: 9,
		DescentAlertM:              30,
		RhrAlertDeltaBpm:           5,
		Pace10kMinSecPerKm:         215,
		Pace10kMaxSecPerKm:         220,
		PaceMarathonMinSecPerKm:    230,
		PaceMarathonMaxSecPerKm:    240,
	}
}

// ProfilePatch est une mise à jour partielle du profil (PUT /api/settings).
type ProfilePatch struct {
	InjuredSide                *string  `json:"injuredSide"`
	CadenceTargetMin           *float64 `json:"cadenceTargetMin"`
	CadenceTargetMax           *float64 `json:"cadenceTargetMax"`
	GctBalanceAlertPct         *float64 `json:"gctBalanceAlertPct"`
	VerticalOscillationAlertCm *float64 `json:"verticalOscillationAlertCm"`
	DescentAlertM              *float64 `json:"descentAlertM"`
	RhrAlertDeltaBpm           *float64 `json:"rhrAlertDeltaBpm"`
	Pace10kMinSecPerKm         *float64 `json:"pace10kMinSecPerKm"`
	Pace10kMaxSecPerKm         *float64 `json:"pace10kMaxSecPerKm"`
	PaceMarathonMinSecPerKm    *float64 `json:"paceMarathonMinSecPerKm"`
	PaceMarathonMaxSecPerKm    *float64 `json:"paceMarathonMaxSecPerKm"`
}

// Apply applique le patch et valide le résultat.
func (p Profile) Apply(pp *ProfilePatch) (Profile, error) {
	if pp == nil {
		return p, nil
	}
	if pp.InjuredSide != nil {
		s := strings.ToLower(strings.TrimSpace(*pp.InjuredSide))
		if s != "left" && s != "right" {
			return p, errors.New("injuredSide doit valoir \"left\" ou \"right\"")
		}
		p.InjuredSide = s
	}
	set := func(dst *float64, src *float64, name string, min, max float64) error {
		if src == nil {
			return nil
		}
		if *src < min || *src > max {
			return fmt.Errorf("%s hors limites [%g, %g]", name, min, max)
		}
		*dst = *src
		return nil
	}
	checks := []error{
		set(&p.CadenceTargetMin, pp.CadenceTargetMin, "cadenceTargetMin", 100, 250),
		set(&p.CadenceTargetMax, pp.CadenceTargetMax, "cadenceTargetMax", 100, 250),
		set(&p.GctBalanceAlertPct, pp.GctBalanceAlertPct, "gctBalanceAlertPct", 0.1, 20),
		set(&p.VerticalOscillationAlertCm, pp.VerticalOscillationAlertCm, "verticalOscillationAlertCm", 3, 20),
		set(&p.DescentAlertM, pp.DescentAlertM, "descentAlertM", 1, 5000),
		set(&p.RhrAlertDeltaBpm, pp.RhrAlertDeltaBpm, "rhrAlertDeltaBpm", 1, 40),
		set(&p.Pace10kMinSecPerKm, pp.Pace10kMinSecPerKm, "pace10kMinSecPerKm", 120, 900),
		set(&p.Pace10kMaxSecPerKm, pp.Pace10kMaxSecPerKm, "pace10kMaxSecPerKm", 120, 900),
		set(&p.PaceMarathonMinSecPerKm, pp.PaceMarathonMinSecPerKm, "paceMarathonMinSecPerKm", 120, 900),
		set(&p.PaceMarathonMaxSecPerKm, pp.PaceMarathonMaxSecPerKm, "paceMarathonMaxSecPerKm", 120, 900),
	}
	if err := errors.Join(checks...); err != nil {
		return p, err
	}
	if p.CadenceTargetMin > p.CadenceTargetMax {
		return p, errors.New("cadenceTargetMin doit être ≤ cadenceTargetMax")
	}
	if p.Pace10kMinSecPerKm > p.Pace10kMaxSecPerKm || p.PaceMarathonMinSecPerKm > p.PaceMarathonMaxSecPerKm {
		return p, errors.New("les bornes d'allure min doivent être ≤ max (en s/km)")
	}
	return p, nil
}

// persisted est le contenu de DATA_DIR/settings.json.
type persisted struct {
	APIKey   string  `json:"apiKey,omitempty"`
	Timezone string  `json:"timezone,omitempty"`
	Profile  Profile `json:"profile"`
}

// Store gère les réglages persistés (thread-safe).
type Store struct {
	mu        sync.RWMutex
	path      string
	envKey    string
	envTZ     string
	data      persisted
	locCache  *time.Location
	locCached string
}

// NewStore charge DATA_DIR/settings.json (ou les valeurs par défaut).
func NewStore(dataDir string, cfg Config) (*Store, error) {
	s := &Store{
		path:   filepath.Join(dataDir, "settings.json"),
		envKey: cfg.EnvAPIKey,
		envTZ:  cfg.Timezone,
		data:   persisted{Profile: DefaultProfile()},
	}
	b, err := os.ReadFile(s.path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return s, nil
	case err != nil:
		return s, fmt.Errorf("lecture %s : %w", s.path, err)
	}
	var p persisted
	p.Profile = DefaultProfile() // champs absents = défauts
	if err := json.Unmarshal(b, &p); err != nil {
		return s, fmt.Errorf("settings.json illisible : %w", err)
	}
	if p.Profile.InjuredSide != "left" && p.Profile.InjuredSide != "right" {
		p.Profile.InjuredSide = "left"
	}
	s.data = p
	return s, nil
}

// APIKey retourne la clé effective et sa source ("ui" | "env" | "none").
func (s *Store) APIKey() (key, source string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.APIKey != "" {
		return s.data.APIKey, "ui"
	}
	if s.envKey != "" {
		return s.envKey, "env"
	}
	return "", "none"
}

// SetAPIKey enregistre (ou supprime si vide) la clé saisie dans l'UI.
func (s *Store) SetAPIKey(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.APIKey = strings.TrimSpace(key)
	return s.saveLocked()
}

// Timezone retourne le fuseau effectif.
func (s *Store) Timezone() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.Timezone != "" {
		return s.data.Timezone
	}
	if s.envTZ != "" {
		return s.envTZ
	}
	return DefaultTimezone
}

// Location retourne le *time.Location du fuseau effectif.
func (s *Store) Location() *time.Location {
	tz := s.Timezone()
	s.mu.RLock()
	if s.locCache != nil && s.locCached == tz {
		l := s.locCache
		s.mu.RUnlock()
		return l
	}
	s.mu.RUnlock()
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc, _ = time.LoadLocation(DefaultTimezone)
		if loc == nil {
			loc = time.UTC
		}
	}
	s.mu.Lock()
	s.locCache, s.locCached = loc, tz
	s.mu.Unlock()
	return loc
}

// SetTimezone valide et enregistre le fuseau ("" = retour à l'environnement).
func (s *Store) SetTimezone(tz string) error {
	tz = strings.TrimSpace(tz)
	if tz != "" {
		if _, err := time.LoadLocation(tz); err != nil {
			return fmt.Errorf("fuseau horaire inconnu : %q", tz)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Timezone = tz
	return s.saveLocked()
}

// Profile retourne une copie du profil.
func (s *Store) Profile() Profile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.Profile
}

// UpdateProfile applique un patch partiel et persiste.
func (s *Store) UpdateProfile(pp *ProfilePatch) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	np, err := s.data.Profile.Apply(pp)
	if err != nil {
		return err
	}
	s.data.Profile = np
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(s.path, b, 0o600)
}

// WriteFileAtomic écrit un fichier via un fichier temporaire + rename, avec les permissions données.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
