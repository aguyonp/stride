package store

import (
	"context"
	"strconv"
)

// Release check (SPEC §22, issue #143): one admin switch, off by default is not an option — the
// feature is useless disabled — but an air-gapped instance must be able to silence it.
const (
	SettingUpdateCheckEnabled = "update_check_enabled"
	DefaultUpdateCheckEnabled = true
)

// UpdateCheckEnabled reads the admin switch; a missing or unreadable value keeps the default.
func (s *Store) UpdateCheckEnabled(ctx context.Context) bool {
	v, ok, err := s.Setting(ctx, SettingUpdateCheckEnabled)
	if err != nil || !ok {
		return DefaultUpdateCheckEnabled
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return DefaultUpdateCheckEnabled
	}
	return b
}

// AdminUserIDs lists the active administrators: the release notice is written once per admin and
// per version (issue #143), so every admin finds it on the next load whatever device they use.
func (s *Store) AdminUserIDs(ctx context.Context) ([]int64, error) {
	rows, err := s.R.QueryContext(ctx, `SELECT id FROM users WHERE role = 'admin' AND status = 'active' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
