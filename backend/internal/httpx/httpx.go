// Package httpx holds JSON helpers shared by platform and domain handlers.
package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

const (
	CodeBadRequest     = "BAD_REQUEST"
	CodeUnauthorized   = "UNAUTHORIZED"
	CodeForbidden      = "FORBIDDEN"
	CodeNotFound       = "NOT_FOUND"
	CodeConflict       = "CONFLICT"
	CodeRateLimited    = "RATE_LIMITED"
	CodeProviderAuth   = "PROVIDER_UNAUTHORIZED"
	CodeProviderDown   = "PROVIDER_UNAVAILABLE"
	CodeNotConnected   = "PROVIDER_NOT_CONNECTED"
	CodeInviteRequired = "INVITE_REQUIRED"
	CodeInternal       = "INTERNAL"
)

type Error struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if w.Header().Get("Cache-Control") == "" {
		w.Header().Set("Cache-Control", "no-store")
	}
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func Fail(w http.ResponseWriter, status int, code, msg string) {
	JSON(w, status, Error{Error: msg, Code: code})
}

func Text(w http.ResponseWriter, contentType, body string) {
	w.Header().Set("Content-Type", contentType)
	if w.Header().Get("Cache-Control") == "" {
		w.Header().Set("Cache-Control", "no-store")
	}
	_, _ = io.WriteString(w, body)
}

// Decode reads a JSON body (max 1 MiB unless limit > 0) and rejects unknown fields.
func Decode(w http.ResponseWriter, r *http.Request, v any, limit int64) error {
	if limit <= 0 {
		limit = 1 << 20
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("trailing data after JSON body")
	}
	return nil
}
