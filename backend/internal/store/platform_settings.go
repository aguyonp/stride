package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

const (
	SettingRegistrationMode = "registration_mode"
	RegistrationOpen        = "open"
	RegistrationInvite      = "invite"
)

func (s *Store) GetSetting(ctx context.Context, key string) (string, bool, error) {
	var v string
	err := s.DB.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return v, err == nil, err
}

func (s *Store) SetSetting(ctx context.Context, key, value string, now time.Time) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO app_settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, key, value, now.Unix())
	return err
}

// EnsureSetting stores value only when key is not set yet (env-provided initial values).
func (s *Store) EnsureSetting(ctx context.Context, key, value string, now time.Time) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO app_settings (key, value, updated_at) VALUES (?, ?, ?) ON CONFLICT(key) DO NOTHING`, key, value, now.Unix())
	return err
}

func (s *Store) RegistrationMode(ctx context.Context) (string, error) {
	return registrationMode(ctx, s.DB)
}

func registrationMode(ctx context.Context, q pfQuerier) (string, error) {
	var v string
	err := q.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key = ?`, SettingRegistrationMode).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && v != RegistrationOpen) {
		return RegistrationInvite, nil
	}
	return v, err
}
