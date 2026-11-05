package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Invitation never exposes the code; only its SHA-256 and a display prefix are stored.
type Invitation struct {
	ID         int64
	Label      string
	CodePrefix string
	MaxUses    int
	Uses       int
	ExpiresAt  *time.Time
	Revoked    bool
	CreatedBy  *int64
	CreatedAt  time.Time
}

func (s *Store) CreateInvitation(ctx context.Context, inv Invitation, codeHash []byte) (*Invitation, error) {
	var createdBy any
	if inv.CreatedBy != nil {
		createdBy = *inv.CreatedBy
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO invitations (code_hash, code_prefix, label, max_uses, expires_at, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, codeHash, inv.CodePrefix, inv.Label, inv.MaxUses, pfNullTime(inv.ExpiresAt), createdBy, inv.CreatedAt.Unix())
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetInvitation(ctx, id)
}

const invitationColumns = `id, label, code_prefix, max_uses, uses, expires_at, revoked, created_by, created_at`

func scanInvitation(sc interface{ Scan(...any) error }) (*Invitation, error) {
	var (
		inv       Invitation
		expires   sql.NullInt64
		revoked   int
		createdBy sql.NullInt64
		created   int64
	)
	if err := sc.Scan(&inv.ID, &inv.Label, &inv.CodePrefix, &inv.MaxUses, &inv.Uses, &expires, &revoked, &createdBy, &created); err != nil {
		return nil, err
	}
	inv.ExpiresAt = pfTimePtr(expires)
	inv.Revoked = revoked != 0
	if createdBy.Valid {
		inv.CreatedBy = &createdBy.Int64
	}
	inv.CreatedAt = time.Unix(created, 0).UTC()
	return &inv, nil
}

func (s *Store) GetInvitation(ctx context.Context, id int64) (*Invitation, error) {
	inv, err := scanInvitation(s.DB.QueryRowContext(ctx, `SELECT `+invitationColumns+` FROM invitations WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUnknownInvitation
	}
	return inv, err
}

func (s *Store) ListInvitations(ctx context.Context) ([]Invitation, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+invitationColumns+` FROM invitations ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Invitation
	for rows.Next() {
		inv, err := scanInvitation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *inv)
	}
	return out, rows.Err()
}

func (s *Store) RevokeInvitation(ctx context.Context, id int64) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE invitations SET revoked = 1 WHERE id = ?`, id)
	return pfAffected(res, err, ErrUnknownInvitation)
}

// CheckInvitation reports ErrInviteInvalid unless the invitation can still be used.
func (s *Store) CheckInvitation(ctx context.Context, codeHash []byte, now time.Time) error {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM invitations WHERE code_hash = ? AND `+usableInvitation, codeHash, now.Unix()).Scan(&n)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrInviteInvalid
	}
	return nil
}

const usableInvitation = `revoked = 0 AND uses < max_uses AND (expires_at IS NULL OR expires_at > ?)`

func consumeInvitation(ctx context.Context, q pfQuerier, codeHash []byte, now time.Time) error {
	res, err := q.ExecContext(ctx, `UPDATE invitations SET uses = uses + 1 WHERE code_hash = ? AND `+usableInvitation, codeHash, now.Unix())
	return pfAffected(res, err, ErrInviteInvalid)
}

// PurgeInvitations deletes invitations that are revoked, used up or expired and older than keep.
func (s *Store) PurgeInvitations(ctx context.Context, now time.Time, keep time.Duration) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM invitations
		WHERE (revoked = 1 OR uses >= max_uses OR (expires_at IS NOT NULL AND expires_at <= ?)) AND created_at < ?`,
		now.Unix(), now.Add(-keep).Unix())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
