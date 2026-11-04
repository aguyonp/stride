package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSPAFallbackAndHeaders(t *testing.T) {
	h := New()
	for _, path := range []string{"/", "/activities/123", "/day/2026-11-03"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<html") {
			t.Errorf("%s : %d", path, rec.Code)
		}
		if rec.Header().Get("Cache-Control") != "no-cache" || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
			t.Errorf("%s : en-têtes %v", path, rec.Header())
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/absent.js", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("asset absent : %d", rec.Code)
	}
	// ETag / 304
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("If-None-Match", rec.Header().Get("ETag"))
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusNotModified {
		t.Errorf("304 attendu, obtenu %d", rec2.Code)
	}
}

func TestContentTypes(t *testing.T) {
	for ext, want := range map[string]string{".webmanifest": "application/manifest+json", ".js": "text/javascript", ".svg": "image/svg+xml", ".woff2": "font/woff2"} {
		if !strings.HasPrefix(contentTypes[ext], want) {
			t.Errorf("%s → %s", ext, contentTypes[ext])
		}
	}
}
