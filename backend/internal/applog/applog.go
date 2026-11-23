// Package applog keeps a bounded audit trail of HTTP requests, outbound provider calls and user actions
// for the admin back office. Writes are asynchronous and batched; old rows are purged.
package applog

import (
	"context"
	"database/sql"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	CategoryRequest = "request"
	CategoryAction  = "user_action"
	LevelInfo       = "info"
	LevelWarn       = "warn"
	LevelError      = "error"

	Retention = 14 * 24 * time.Hour
	MaxRows   = 100_000
)

type Entry struct {
	ID         int64     `json:"id"`
	At         time.Time `json:"at"`
	Category   string    `json:"category"`
	Level      string    `json:"level"`
	UserID     *int64    `json:"userId"`
	UserName   *string   `json:"userName"`
	Method     string    `json:"method"`
	Target     string    `json:"target"`
	Provider   string    `json:"provider"`
	Status     int       `json:"status"`
	DurationMs int64     `json:"durationMs"`
	Action     string    `json:"action"`
	Message    string    `json:"message"`
}

type Log struct {
	db      *sql.DB
	log     *slog.Logger
	ch      chan Entry
	dropped atomic.Int64
	done    chan struct{}
	once    sync.Once
}

func New(db *sql.DB, log *slog.Logger) *Log {
	l := &Log{db: db, log: log, ch: make(chan Entry, 2048), done: make(chan struct{})}
	go l.loop()
	return l
}

// Add enqueues an entry without blocking; entries are dropped when the buffer is full.
func (l *Log) Add(e Entry) {
	if l == nil {
		return
	}
	if e.At.IsZero() {
		e.At = time.Now()
	}
	if e.Level == "" {
		e.Level = LevelInfo
	}
	select {
	case l.ch <- e:
	default:
		l.dropped.Add(1)
	}
}

// Close flushes pending entries.
func (l *Log) Close() {
	if l == nil {
		return
	}
	l.once.Do(func() { close(l.ch) })
	<-l.done
}

func (l *Log) loop() {
	defer close(l.done)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	batch := make([]Entry, 0, 256)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := l.insert(batch); err != nil && l.log != nil {
			l.log.Warn("⚠️ Écriture des journaux impossible", "err", err.Error(), "entrées", len(batch))
		}
		batch = batch[:0]
	}
	for {
		select {
		case e, ok := <-l.ch:
			if !ok {
				flush()
				return
			}
			batch = append(batch, e)
			if len(batch) >= 256 {
				flush()
			}
		case <-tick.C:
			flush()
		}
	}
}

func (l *Log) insert(batch []Entry) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO app_logs (at_ms, category, level, user_id, method, target, provider, status, duration_ms, action, message) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, e := range batch {
		if _, err := stmt.ExecContext(ctx, e.At.UnixMilli(), e.Category, e.Level, e.UserID, e.Method, trunc(e.Target, 300), e.Provider, e.Status, e.DurationMs, e.Action, trunc(e.Message, 500)); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// Purge drops rows older than Retention and keeps at most MaxRows.
func (l *Log) Purge(ctx context.Context, now time.Time) (int64, error) {
	res, err := l.db.ExecContext(ctx, `DELETE FROM app_logs WHERE at_ms < ?`, now.Add(-Retention).UnixMilli())
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	res, err = l.db.ExecContext(ctx, `DELETE FROM app_logs WHERE id <= (SELECT id FROM app_logs ORDER BY id DESC LIMIT 1 OFFSET ?)`, MaxRows)
	if err != nil {
		return n, err
	}
	m, _ := res.RowsAffected()
	return n + m, nil
}

// CountErrorsSince counts error-level entries recorded after since (pending batch excluded).
func (l *Log) CountErrorsSince(ctx context.Context, since time.Time) (int, error) {
	var n int
	err := l.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM app_logs WHERE level = ? AND at_ms >= ?`, LevelError, since.UnixMilli()).Scan(&n)
	return n, err
}

// Dropped is the number of entries lost because the write buffer was full.
func (l *Log) Dropped() int64 { return l.dropped.Load() }

type Filter struct {
	Category string
	Level    string
	UserID   int64
	Before   int64
	Limit    int
}

type Page struct {
	Items      []Entry `json:"items"`
	NextBefore *int64  `json:"nextBefore"`
	Dropped    int64   `json:"dropped"`
}

func (l *Log) Query(ctx context.Context, f Filter) (Page, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 100
	}
	var where []string
	var args []any
	if f.Category != "" {
		where = append(where, "l.category = ?")
		args = append(args, f.Category)
	}
	if f.Level != "" {
		where = append(where, "l.level = ?")
		args = append(args, f.Level)
	}
	if f.UserID > 0 {
		where = append(where, "l.user_id = ?")
		args = append(args, f.UserID)
	}
	if f.Before > 0 {
		where = append(where, "l.id < ?")
		args = append(args, f.Before)
	}
	q := `SELECT l.id, l.at_ms, l.category, l.level, l.user_id, u.display_name, l.method, l.target, l.provider, l.status, l.duration_ms, l.action, l.message
		FROM app_logs l LEFT JOIN users u ON u.id = l.user_id`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY l.id DESC LIMIT " + strconv.Itoa(f.Limit+1)
	rows, err := l.db.QueryContext(ctx, q, args...)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	p := Page{Items: []Entry{}, Dropped: l.dropped.Load()}
	for rows.Next() {
		var e Entry
		var at int64
		var uid sql.NullInt64
		var name sql.NullString
		if err := rows.Scan(&e.ID, &at, &e.Category, &e.Level, &uid, &name, &e.Method, &e.Target, &e.Provider, &e.Status, &e.DurationMs, &e.Action, &e.Message); err != nil {
			return Page{}, err
		}
		e.At = time.UnixMilli(at).UTC()
		if uid.Valid {
			e.UserID = &uid.Int64
		}
		if name.Valid {
			e.UserName = &name.String
		}
		p.Items = append(p.Items, e)
	}
	if len(p.Items) > f.Limit {
		p.Items = p.Items[:f.Limit]
		next := p.Items[len(p.Items)-1].ID
		p.NextBefore = &next
	}
	return p, rows.Err()
}

type metaKey struct{}

// Meta carries per-request facts discovered deeper in the handler chain (the authenticated user).
type Meta struct{ userID atomic.Int64 }

func WithMeta(ctx context.Context) (context.Context, *Meta) {
	m := &Meta{}
	return context.WithValue(ctx, metaKey{}, m), m
}

// SetUser records the authenticated user of the current request, if the request is tracked.
func SetUser(ctx context.Context, id int64) {
	if m, ok := ctx.Value(metaKey{}).(*Meta); ok {
		m.userID.Store(id)
	}
}

func (m *Meta) UserID() *int64 {
	if id := m.userID.Load(); id > 0 {
		return &id
	}
	return nil
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
