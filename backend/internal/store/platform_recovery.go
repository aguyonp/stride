package store

import (
	"context"
	"database/sql"
	"time"
)

// Recovery codes are one-time login credentials (issue #56). Only the SHA-256 of the
// normalized code is stored; the plaintext exists for the user at generation only.

// CountUnusedRecoveryCodes returns how many of the user's codes are still usable.
func (s *Store) CountUnusedRecoveryCodes(ctx context.Context, userID int64) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM recovery_codes WHERE user_id = ? AND used_at IS NULL`, userID).Scan(&n)
	return n, err
}

// ReplaceRecoveryCodes atomically invalidates the user's remaining codes and stores a new set.
func (s *Store) ReplaceRecoveryCodes(ctx context.Context, userID int64, hashes [][]byte, now time.Time) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM recovery_codes WHERE user_id = ?`, userID); err != nil {
			return err
		}
		for _, h := range hashes {
			if _, err := tx.ExecContext(ctx, `INSERT INTO recovery_codes (user_id, code_hash, created_at) VALUES (?, ?, ?)`,
				userID, h, now.Unix()); err != nil {
				return err
			}
		}
		return nil
	})
}

// ConsumeRecoveryCode marks the unused code with the given hash as used (single use enforced
// by the affected-rows guard) and returns its owner.
func (s *Store) ConsumeRecoveryCode(ctx context.Context, hash []byte, now time.Time) (int64, error) {
	var userID int64
	err := s.Tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE recovery_codes SET used_at = ? WHERE code_hash = ? AND used_at IS NULL`, now.Unix(), hash)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrRecoveryCodeInvalid
		}
		return tx.QueryRowContext(ctx, `SELECT user_id FROM recovery_codes WHERE code_hash = ?`, hash).Scan(&userID)
	})
	if err != nil {
		return 0, err
	}
	return userID, nil
}
