package domain

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"stride/internal/apimeter"
	"stride/internal/athlete"
	"stride/internal/garmin"
	"stride/internal/provider/strava"
)

var (
	ErrNotFound       = errors.New("ressource introuvable")
	ErrNotConnected   = errors.New("fournisseur non connecté")
	ErrStravaDisabled = errors.New("Strava n'est pas configuré sur ce serveur")
)

// BadRequestError carries a French message for a 400 answer.
type BadRequestError struct{ Msg string }

func (e *BadRequestError) Error() string { return e.Msg }

func badRequest(format string, args ...any) error {
	return &BadRequestError{Msg: fmt.Sprintf(format, args...)}
}

type ConflictError struct{ Msg string }

func (e *ConflictError) Error() string { return e.Msg }

// ProviderError describes a failed provider call for the HTTP layer.
type ProviderError struct {
	Provider      string
	Unauthorized  bool
	RateLimited   bool
	Timeout       bool
	CooldownUntil time.Time
	Status        int
	Err           error
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("%s: %v", e.Provider, e.Err)
}

func (e *ProviderError) Unwrap() error { return e.Err }

// Message is the French text returned to the client.
func (e *ProviderError) Message() string {
	name := providerName(e.Provider)
	switch {
	case e.Unauthorized:
		return name + " a refusé les identifiants (clé ou autorisation révoquée)."
	case e.RateLimited:
		return name + " limite le nombre de requêtes, nouvelle tentative possible plus tard."
	case e.Timeout:
		return name + " ne répond pas (délai dépassé)."
	case e.Status > 0:
		return fmt.Sprintf("%s est indisponible (HTTP %d).", name, e.Status)
	}
	return name + " est injoignable (erreur réseau)."
}

func providerName(p string) string {
	switch p {
	case apimeter.Strava:
		return "Strava"
	case apimeter.OpenMeteo:
		return "Le service météo"
	}
	return "Garmin"
}

// providerErr normalizes provider client errors; other errors pass through.
func providerErr(provider string, err error) error {
	if err == nil {
		return nil
	}
	var pe *ProviderError
	if errors.As(err, &pe) || errors.Is(err, ErrNotConnected) || errors.Is(err, context.Canceled) {
		return err
	}
	if errors.Is(err, garmin.ErrNoAPIKey) {
		return ErrNotConnected
	}
	out := &ProviderError{Provider: provider, Err: err}
	var cd *apimeter.CooldownError
	var ae *garmin.APIError
	var su *strava.ErrUnauthorized
	switch {
	case errors.As(err, &cd):
		out.RateLimited, out.CooldownUntil = true, cd.Until
	case errors.As(err, &su):
		out.Unauthorized = true
	case errors.As(err, &ae):
		out.Status = ae.Status
		out.Unauthorized = garmin.IsUnauthorized(err)
		out.RateLimited = ae.Status == 429
		out.Timeout = ae.Status == 0 && garmin.IsTimeout(err)
	case garmin.IsTimeout(err):
		out.Timeout = true
	}
	return out
}

// errText is a short log-safe description (never contains credentials).
func errText(err error) string {
	var pe *ProviderError
	if errors.As(err, &pe) {
		return pe.Message()
	}
	return err.Error()
}

func isNotFound(err error) bool {
	return errors.Is(err, ErrNotFound) || errors.Is(err, athlete.ErrNotFound) || garmin.IsNotFound(err)
}

func gz(b []byte) []byte {
	var buf bytes.Buffer
	w, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	_, _ = w.Write(b)
	_ = w.Close()
	return buf.Bytes()
}

func gunzip(b []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(io.LimitReader(r, 256<<20))
}

func rfc3339(ts int64) *string {
	if ts <= 0 {
		return nil
	}
	s := time.Unix(ts, 0).UTC().Format(time.RFC3339)
	return &s
}

func ptrStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func f64(v float64) *float64 { return &v }
