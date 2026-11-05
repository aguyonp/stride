package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// Credential is a stored passkey; Data is the go-webauthn Credential encoded as JSON.
type Credential struct {
	ID             []byte
	UserID         int64
	Name           string
	Data           []byte
	BackupEligible bool
	BackupState    bool
	CreatedAt      time.Time
	LastUsedAt     *time.Time
}

type NewCredential struct {
	ID             []byte
	Name           string
	Data           []byte
	BackupEligible bool
	BackupState    bool
}

func (s *Store) ListCredentials(ctx context.Context, userID int64) ([]Credential, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, user_id, name, credential, backup_eligible, backup_state, created_at, last_used_at
		FROM webauthn_credentials WHERE user_id = ? ORDER BY created_at, rowid`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Credential
	for rows.Next() {
		var (
			c       Credential
			be, bs  int
			created int64
			used    sql.NullInt64
		)
		if err := rows.Scan(&c.ID, &c.UserID, &c.Name, &c.Data, &be, &bs, &created, &used); err != nil {
			return nil, err
		}
		c.BackupEligible, c.BackupState = be != 0, bs != 0
		c.CreatedAt = time.Unix(created, 0).UTC()
		c.LastUsedAt = pfTimePtr(used)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) AddCredential(ctx context.Context, userID int64, c NewCredential, now time.Time) error {
	return addCredential(ctx, s.DB, userID, c, now)
}

func addCredential(ctx context.Context, q pfQuerier, userID int64, c NewCredential, now time.Time) error {
	_, err := q.ExecContext(ctx, `INSERT INTO webauthn_credentials (id, user_id, name, credential, backup_eligible, backup_state, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, c.ID, userID, c.Name, c.Data, pfBool(c.BackupEligible), pfBool(c.BackupState), now.Unix())
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return ErrDuplicateCredential
	}
	return err
}

// UpdateCredentialUse stores the refreshed credential (sign count, flags) after a successful login.
func (s *Store) UpdateCredentialUse(ctx context.Context, id, data []byte, backupState bool, at time.Time) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE webauthn_credentials SET credential = ?, backup_state = ?, last_used_at = ? WHERE id = ?`,
		data, pfBool(backupState), at.Unix(), id)
	return pfAffected(res, err, ErrUnknownCredential)
}

func (s *Store) RenameCredential(ctx context.Context, userID int64, id []byte, name string) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE webauthn_credentials SET name = ? WHERE id = ? AND user_id = ?`, name, id, userID)
	return pfAffected(res, err, ErrUnknownCredential)
}

// DeleteCredential refuses to remove the last passkey of a user.
func (s *Store) DeleteCredential(ctx context.Context, userID int64, id []byte) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		var owned, total int
		err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(id = ?), 0), COUNT(*) FROM webauthn_credentials WHERE user_id = ?`, id, userID).Scan(&owned, &total)
		if err != nil {
			return err
		}
		if owned == 0 {
			return ErrUnknownCredential
		}
		if total <= 1 {
			return ErrLastCredential
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM webauthn_credentials WHERE id = ? AND user_id = ?`, id, userID)
		return err
	})
}
