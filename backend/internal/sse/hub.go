// Package sse implements the native Go Server-Sent Events hub for real-time
// job progress, sync updates, biometric alerts and LLM token streaming, with optional Valkey
// Pub/Sub multi-instance fanout.
package sse

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/valkey-io/valkey-go"

	"stride/internal/auth"
	"stride/internal/httpx"
	"stride/internal/observability"
)

const (
	PubSubChannel            = "stride:sse:events"
	defaultHeartbeatInterval = 20 * time.Second
	clientBufferSize         = 128
)

// Event names defined by the Stride 4.0.0 SSE contract.
const (
	EventConnected      = "connected"
	EventJobUpdated     = "job.updated"
	EventSyncUpdated    = "sync.updated"
	EventAlertTriggered = "alert.triggered"
	EventLLMToken       = "llm.token"
	EventLLMDone        = "llm.done"
	// EventShareUpdated tells the owner that a share link wrote through the portal (4.2.0).
	EventShareUpdated = "share.updated"
	// EventNotificationCreated / EventNotificationUpdated feed the notification centre (4.7.0, #142).
	EventNotificationCreated = "notification.created"
	EventNotificationUpdated = "notification.updated"
)

// StreamNotificationEvent carries one new centre entry (created) or the new unread count (updated).
type StreamNotificationEvent struct {
	Entry  any `json:"entry,omitempty"`
	Unread int `json:"unread"`
}

// ShareUpdatedEvent names what a share link changed: directive | remark | plan | event.
type ShareUpdatedEvent struct {
	Kind   string `json:"kind"`
	LinkID int64  `json:"linkId"`
}

type ConnectedEvent struct {
	Status     string `json:"status"`
	UserID     int64  `json:"userId"`
	ServerTime string `json:"serverTime"`
}

type StreamAlertEvent struct {
	ID         string `json:"id"`
	ActivityID *int64 `json:"activityId"`
	Date       string `json:"date"`
	Level      string `json:"level"` // info | warn | critical
	Code       string `json:"code"`
	Title      string `json:"title"`
	Detail     string `json:"detail"`
	CreatedAt  string `json:"createdAt"`
}

type StreamLLMTokenEvent struct {
	JobID string `json:"jobId"`
	Kind  string `json:"kind"`
	Ref   string `json:"ref"`
	Delta string `json:"delta"`
}

type StreamLLMDoneEvent struct {
	JobID        string  `json:"jobId"`
	Kind         string  `json:"kind"`
	Ref          string  `json:"ref"`
	Error        *string `json:"error,omitempty"`
	Analysis     any     `json:"analysis,omitempty"`
	PlanDrafts   any     `json:"planDrafts,omitempty"`
	AdaptPreview any     `json:"adaptPreview,omitempty"`
	MorphPreview any     `json:"morphPreview,omitempty"`
}

type pubsubEnvelope struct {
	Origin string          `json:"origin"`
	UserID int64           `json:"userId,omitempty"`
	Event  string          `json:"event"`
	Data   json.RawMessage `json:"data"`
}

type client struct {
	userID int64
	ch     chan []byte
}

type Options struct {
	Metrics           *observability.Metrics
	Valkey            valkey.Client
	Log               *slog.Logger
	HeartbeatInterval time.Duration
}

type Hub struct {
	metrics    *observability.Metrics
	valkey     valkey.Client
	log        *slog.Logger
	heartbeat  time.Duration
	instanceID string

	mu       sync.RWMutex
	byUser   map[int64]map[*client]struct{}
	closed   chan struct{}
	stopOnce sync.Once
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

func NewHub(opts Options) *Hub {
	hb := opts.HeartbeatInterval
	if hb <= 0 {
		hb = defaultHeartbeatInterval
	}
	log := opts.Log
	if log == nil {
		log = slog.Default()
	}
	var idBytes [8]byte
	_, _ = rand.Read(idBytes[:])
	ctx, cancel := context.WithCancel(context.Background())
	h := &Hub{
		metrics:    opts.Metrics,
		valkey:     opts.Valkey,
		log:        log,
		heartbeat:  hb,
		instanceID: hex.EncodeToString(idBytes[:]),
		byUser:     make(map[int64]map[*client]struct{}),
		closed:     make(chan struct{}),
		cancel:     cancel,
	}
	if h.valkey != nil {
		h.wg.Add(1)
		go h.runPubSub(ctx)
	}
	return h
}

func (h *Hub) runPubSub(ctx context.Context) {
	defer h.wg.Done()
	for ctx.Err() == nil {
		err := h.valkey.Receive(ctx, h.valkey.B().Subscribe().Channel(PubSubChannel).Build(), func(msg valkey.PubSubMessage) {
			var env pubsubEnvelope
			if json.Unmarshal([]byte(msg.Message), &env) != nil || env.Origin == h.instanceID {
				return
			}
			if env.UserID > 0 {
				h.deliverUser(env.UserID, formatFrame(env.Event, env.Data))
			}
		})
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			h.log.Warn("📡 Relais Pub/Sub SSE interrompu, reconnexion…", "err", err.Error())
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}

// Close disconnects active SSE streams and stops any Valkey Pub/Sub subscriber.
func (h *Hub) Close() {
	if h == nil {
		return
	}
	h.stopOnce.Do(func() {
		h.cancel()
		close(h.closed)
		h.wg.Wait()
	})
}

func formatFrame(event string, rawJSON []byte) []byte {
	out := make([]byte, 0, len(event)+len(rawJSON)+16)
	out = append(out, "event: "...)
	out = append(out, event...)
	out = append(out, "\ndata: "...)
	out = append(out, rawJSON...)
	out = append(out, "\n\n"...)
	return out
}

// HasSubscribers reports whether at least one SSE client is currently connected for userID.
func (h *Hub) HasSubscribers(userID int64) bool {
	if h == nil || userID <= 0 {
		return false
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.byUser[userID]) > 0
}

// PublishToUser sends a JSON SSE event exclusively to streams authenticated as userID.
func (h *Hub) PublishToUser(userID int64, event string, payload any) {
	if h == nil || userID <= 0 {
		return
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	frame := formatFrame(event, raw)
	h.deliverUser(userID, frame)
	if h.valkey != nil {
		env, _ := json.Marshal(pubsubEnvelope{
			Origin: h.instanceID,
			UserID: userID,
			Event:  event,
			Data:   raw,
		})
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_ = h.valkey.Do(ctx, h.valkey.B().Publish().Channel(PubSubChannel).Message(string(env)).Build()).Error()
		cancel()
	}
}

func (h *Hub) deliverUser(userID int64, frame []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.byUser[userID] {
		select {
		case c.ch <- frame:
		default:
		}
	}
}

// Stats returns the open streams and distinct users connected to this instance.
func (h *Hub) Stats() (clients, users int) {
	if h == nil {
		return 0, 0
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, m := range h.byUser {
		clients += len(m)
	}
	return clients, len(h.byUser)
}

func (h *Hub) addClient(c *client) {
	h.mu.Lock()
	if h.byUser[c.userID] == nil {
		h.byUser[c.userID] = make(map[*client]struct{})
	}
	h.byUser[c.userID][c] = struct{}{}
	h.mu.Unlock()
	h.metrics.AddSSEClient(1)
}

func (h *Hub) removeClient(c *client) {
	h.mu.Lock()
	if m := h.byUser[c.userID]; m != nil {
		delete(m, c)
		if len(m) == 0 {
			delete(h.byUser, c.userID)
		}
	}
	h.mu.Unlock()
	h.metrics.AddSSEClient(-1)
}

// ServeHTTP handles GET /api/events/stream (expects RequireUser middleware upstream).
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFrom(r.Context())
	if !ok || u.ID <= 0 {
		httpx.Fail(w, http.StatusUnauthorized, httpx.CodeUnauthorized, "Authentification requise.")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpx.Fail(w, http.StatusInternalServerError, httpx.CodeInternal, "Flux SSE non pris en charge.")
		return
	}

	// Clear http.Server.WriteTimeout for this long-lived SSE connection.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})

	hdr := w.Header()
	hdr.Set("Content-Type", "text/event-stream; charset=utf-8")
	hdr.Set("Cache-Control", "no-cache, no-transform")
	hdr.Set("Connection", "keep-alive")
	hdr.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	c := &client{userID: u.ID, ch: make(chan []byte, clientBufferSize)}
	h.addClient(c)
	defer h.removeClient(c)

	initPayload, _ := json.Marshal(ConnectedEvent{
		Status:     "ok",
		UserID:     u.ID,
		ServerTime: time.Now().UTC().Format(time.RFC3339),
	})
	if _, err := w.Write(formatFrame(EventConnected, initPayload)); err != nil {
		return
	}
	flusher.Flush()

	ticker := time.NewTicker(h.heartbeat)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-h.closed:
			return
		case frame := <-c.ch:
			if _, err := w.Write(frame); err != nil {
				return
			}
			flusher.Flush()
		case t := <-ticker.C:
			if _, err := fmt.Fprintf(w, ": heartbeat %d\n\n", t.Unix()); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
