package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Garmin Connect bridge tokens (§23, issue #147): their own provider row, `garmin_web`, so they never
// share a field with the Personal API credentials. The bytes are sealed by the caller (AAD
// `garmin_web:<uid>:token`); this layer stores an opaque blob.

// BridgeTokens returns the sealed blob, nil when the user never connected the bridge.
func (s *Store) BridgeTokens(ctx context.Context, userID int64) ([]byte, error) {
	var blob []byte
	err := s.R.QueryRowContext(ctx, `SELECT token_enc FROM provider_connections WHERE user_id = ? AND provider = 'garmin_web'`, userID).Scan(&blob)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return blob, nil
}

// SaveBridgeTokens upserts the sealed blob; the presence of the row is the user switch.
func (s *Store) SaveBridgeTokens(ctx context.Context, userID int64, sealed []byte, displayName string, now time.Time) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO provider_connections (user_id, provider, status, display_name, token_enc, created_at, updated_at)
		VALUES (?, 'garmin_web', 'ok', ?, ?, ?, ?)
		ON CONFLICT(user_id, provider) DO UPDATE SET status = 'ok', display_name = excluded.display_name,
			token_enc = excluded.token_enc, last_error = NULL, last_error_at = NULL, updated_at = excluded.updated_at`,
		userID, displayName, sealed, now.Unix(), now.Unix())
	return err
}

// DeleteBridgeTokens removes the row: the bridge is disconnected and its tokens are gone.
func (s *Store) DeleteBridgeTokens(ctx context.Context, userID int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM provider_connections WHERE user_id = ? AND provider = 'garmin_web'`, userID)
	return err
}

// BridgeConnected reports whether the bridge has stored tokens (used by the settings and the plan).
func (s *Store) BridgeConnected(ctx context.Context, userID int64) bool {
	var one int
	return s.R.QueryRowContext(ctx, `SELECT 1 FROM provider_connections WHERE user_id = ? AND provider = 'garmin_web'`, userID).Scan(&one) == nil
}
