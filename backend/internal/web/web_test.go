package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestSPAFallbackAndHeaders(t *testing.T) {
	h := New()
	for _, path := range []string{"/", "/activities/123", "/register"} {
		rec := get(h, path)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<html") {
			t.Errorf("%s: %d", path, rec.Code)
		}
		if rec.Header().Get("Cache-Control") != "no-cache" || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
			t.Errorf("%s: headers %v", path, rec.Header())
		}
	}
	if rec := get(h, "/assets/absent.js"); rec.Code != http.StatusNotFound {
		t.Errorf("missing asset: %d", rec.Code)
	}
	rec := get(h, "/")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("If-None-Match", rec.Header().Get("ETag"))
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusNotModified {
		t.Errorf("want 304, got %d", rec2.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: %d", rec.Code)
	}
}

func TestPWAFilesAndCaching(t *testing.T) {
	h := newHandler(fstest.MapFS{
		"index.html":             {Data: []byte("<html></html>")},
		"manifest.json":          {Data: []byte(`{"name":"Stride"}`)},
		"manifest.webmanifest":   {Data: []byte(`{"name":"Stride"}`)},
		"sw.js":                  {Data: []byte("self.addEventListener('fetch',()=>{})")},
		"assets/index-abc123.js": {Data: []byte("console.log(1)")},
		"icon-192.png":           {Data: []byte("\x89PNG\r\n\x1a\n")},
	})
	cases := []struct{ path, ctype, cache string }{
		{"/manifest.json", "application/manifest+json", "no-cache"},
		{"/manifest.webmanifest", "application/manifest+json", "no-cache"},
		{"/sw.js", "text/javascript", "no-cache"},
		{"/assets/index-abc123.js", "text/javascript", "public, max-age=31536000, immutable"},
		{"/icon-192.png", "image/png", "public, max-age=3600"},
	}
	for _, c := range cases {
		rec := get(h, c.path)
		if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), c.ctype) || rec.Header().Get("Cache-Control") != c.cache {
			t.Errorf("%s: %d %q %q", c.path, rec.Code, rec.Header().Get("Content-Type"), rec.Header().Get("Cache-Control"))
		}
	}
}
