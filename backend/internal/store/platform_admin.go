package store

import (
	"context"
	"database/sql"
	"errors"
)

// ErrLastAdmin protects the instance from losing its last active administrator.
var ErrLastAdmin = errors.New("store: last active administrator")

// ensureOtherAdmin fails with ErrLastAdmin when id is an active administrator and no other one exists.
func ensureOtherAdmin(ctx context.Context, q pfQuerier, id int64) error {
	var role, status string
	err := q.QueryRowContext(ctx, `SELECT role, status FROM users WHERE id = ?`, id).Scan(&role, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrUnknownUser
	}
	if err != nil {
		return err
	}
	if role != RoleAdmin || status != StatusActive {
		return nil
	}
	var others int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role = 'admin' AND status = 'active' AND id <> ?`, id).Scan(&others); err != nil {
		return err
	}
	if others == 0 {
		return ErrLastAdmin
	}
	return nil
}

// EnsureNotLastAdmin returns ErrLastAdmin when removing id would leave no active administrator.
func (s *Store) EnsureNotLastAdmin(ctx context.Context, id int64) error {
	return ensureOtherAdmin(ctx, s.DB, id)
}

// DeleteUserSessions revokes every session of a user and returns how many were removed.
func (s *Store) DeleteUserSessions(ctx context.Context, id int64) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, id)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
