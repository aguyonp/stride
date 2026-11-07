package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var (
	ErrUnknownUser         = errors.New("store: unknown user")
	ErrUnknownCredential   = errors.New("store: unknown credential")
	ErrUnknownInvitation   = errors.New("store: unknown invitation")
	ErrDuplicateCredential = errors.New("store: credential already registered")
	ErrLastCredential      = errors.New("store: cannot delete the last credential")
	ErrInviteRequired      = errors.New("store: invitation required")
	ErrInviteInvalid       = errors.New("store: invitation invalid, expired or used up")
)

const (
	RoleUser       = "user"
	RoleAdmin      = "admin"
	StatusActive   = "active"
	StatusDisabled = "disabled"
)

type pfQuerier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type User struct {
	ID                  int64
	WebAuthnID          []byte
	DisplayName         string
	Role                string
	Status              string
	OnboardingCompleted bool
	CreatedAt           time.Time
	LastLoginAt         *time.Time
	Passkeys            int
}

// NewUser is a registration request; the first user becomes admin, later ones obey the registration mode.
type NewUser struct {
	WebAuthnID  []byte
	DisplayName string
	InviteHash  []byte
	Credential  NewCredential
}

const userColumns = `id, webauthn_user_id, display_name, role, status, onboarding_completed, created_at, last_login_at`

// scanUser reads userColumns, optionally surrounded by extra columns.
func scanUser(sc interface{ Scan(...any) error }, pre []any, post ...any) (*User, error) {
	var (
		u         User
		onboarded int
		created   int64
		lastLogin sql.NullInt64
	)
	dest := append(append([]any{}, pre...), &u.ID, &u.WebAuthnID, &u.DisplayName, &u.Role, &u.Status, &onboarded, &created, &lastLogin)
	dest = append(dest, post...)
	if err := sc.Scan(dest...); err != nil {
		return nil, err
	}
	u.OnboardingCompleted = onboarded != 0
	u.CreatedAt = time.Unix(created, 0).UTC()
	u.LastLoginAt = pfTimePtr(lastLogin)
	return &u, nil
}

func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func (s *Store) GetUser(ctx context.Context, id int64) (*User, error) {
	u, err := scanUser(s.DB.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE id = ?`, id), nil)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUnknownUser
	}
	return u, err
}

func (s *Store) GetUserByWebAuthnID(ctx context.Context, handle []byte) (*User, error) {
	u, err := scanUser(s.DB.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE webauthn_user_id = ?`, handle), nil)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUnknownUser
	}
	return u, err
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+userColumns+`,
		(SELECT COUNT(*) FROM webauthn_credentials c WHERE c.user_id = users.id)
		FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var n int
		u, err := scanUser(rows, nil, &n)
		if err != nil {
			return nil, err
		}
		u.Passkeys = n
		out = append(out, *u)
	}
	return out, rows.Err()
}

func (s *Store) CountAdmins(ctx context.Context) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role = 'admin' AND status = 'active'`).Scan(&n)
	return n, err
}

// RegisterUser creates the user and its first passkey atomically, consuming the invitation when required.
func (s *Store) RegisterUser(ctx context.Context, nu NewUser, now time.Time) (*User, error) {
	var id int64
	role := RoleUser
	err := s.Tx(ctx, func(tx *sql.Tx) error {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			role = RoleAdmin
		} else {
			mode, err := registrationMode(ctx, tx)
			if err != nil {
				return err
			}
			if mode == RegistrationInvite {
				if len(nu.InviteHash) == 0 {
					return ErrInviteRequired
				}
				if err := consumeInvitation(ctx, tx, nu.InviteHash, now); err != nil {
					return err
				}
			}
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO users (webauthn_user_id, display_name, role, status, created_at, last_login_at)
			VALUES (?, ?, ?, 'active', ?, ?)`, nu.WebAuthnID, nu.DisplayName, role, now.Unix(), now.Unix())
		if err != nil {
			return err
		}
		if id, err = res.LastInsertId(); err != nil {
			return err
		}
		return addCredential(ctx, tx, id, nu.Credential, now)
	})
	if err != nil {
		return nil, err
	}
	return s.GetUser(ctx, id)
}

func (s *Store) UpdateUserProfile(ctx context.Context, id int64, displayName *string, onboarding *bool) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE users SET
		display_name = COALESCE(?, display_name),
		onboarding_completed = COALESCE(?, onboarding_completed)
		WHERE id = ?`, pfNullString(displayName), pfNullBool(onboarding), id)
	return pfAffected(res, err, ErrUnknownUser)
}

// UpdateUserAccess changes role and/or status; disabling revokes every session of the user.
// Demoting or disabling the last active administrator fails with ErrLastAdmin.
func (s *Store) UpdateUserAccess(ctx context.Context, id int64, role, status *string) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		if (role != nil && *role != RoleAdmin) || (status != nil && *status != StatusActive) {
			if err := ensureOtherAdmin(ctx, tx, id); err != nil {
				return err
			}
		}
		res, err := tx.ExecContext(ctx, `UPDATE users SET role = COALESCE(?, role), status = COALESCE(?, status) WHERE id = ?`,
			pfNullString(role), pfNullString(status), id)
		if err := pfAffected(res, err, ErrUnknownUser); err != nil {
			return err
		}
		if status != nil && *status == StatusDisabled {
			_, err = tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, id)
		}
		return err
	})
}

func (s *Store) TouchUserLogin(ctx context.Context, id int64, at time.Time) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE users SET last_login_at = ? WHERE id = ?`, at.Unix(), id)
	return err
}

// DeleteUser removes the user; credentials, sessions and domain rows cascade.
func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	return pfAffected(res, err, ErrUnknownUser)
}

func pfAffected(res sql.Result, err error, notFound error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return notFound
	}
	return nil
}

func pfTimePtr(v sql.NullInt64) *time.Time {
	if !v.Valid {
		return nil
	}
	t := time.Unix(v.Int64, 0).UTC()
	return &t
}

func pfNullString(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

func pfNullBool(p *bool) any {
	if p == nil {
		return nil
	}
	return pfBool(*p)
}

func pfBool(b bool) int {
	if b {
		return 1
	}
	return 0
}

func pfNullTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.Unix()
}
