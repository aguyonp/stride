package store

import (
	"context"
	"database/sql"
	"time"
)

// APIEvent is an outbound provider call worth keeping (errors, rate limiting) for the admin meter.
type APIEvent struct {
	At       time.Time
	Provider string
	UserID   int64
	Endpoint string
	Status   int
	Message  string
}

func (s *Store) InsertAPIEvents(ctx context.Context, events []APIEvent) error {
	if len(events) == 0 {
		return nil
	}
	return s.Tx(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, `INSERT INTO api_events (at_ms, provider, user_id, endpoint, status, message) VALUES (?, ?, ?, ?, ?, ?)`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, e := range events {
			var uid any
			if e.UserID != 0 {
				uid = e.UserID
			}
			if _, err := stmt.ExecContext(ctx, e.At.UnixMilli(), e.Provider, uid, e.Endpoint, e.Status, e.Message); err != nil {
				return err
			}
		}
		return nil
	})
}

// RecentAPIEvents returns the newest events first.
func (s *Store) RecentAPIEvents(ctx context.Context, limit int) ([]APIEvent, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT at_ms, provider, COALESCE(user_id, 0), endpoint, status, message
		FROM api_events ORDER BY at_ms DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIEvent
	for rows.Next() {
		var (
			e  APIEvent
			at int64
		)
		if err := rows.Scan(&at, &e.Provider, &e.UserID, &e.Endpoint, &e.Status, &e.Message); err != nil {
			return nil, err
		}
		e.At = time.UnixMilli(at).UTC()
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) PurgeAPIEvents(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM api_events WHERE at_ms < ?`, before.UnixMilli())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
