// Package web serves the Vite SPA embedded in the binary. The Docker build copies frontend/dist
// into internal/web/dist; otherwise the committed placeholder index.html is served.
package web

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"
)

//go:embed all:dist
var distFS embed.FS

var contentTypes = map[string]string{
	".html":        "text/html; charset=utf-8",
	".js":          "text/javascript; charset=utf-8",
	".mjs":         "text/javascript; charset=utf-8",
	".css":         "text/css; charset=utf-8",
	".json":        "application/json; charset=utf-8",
	".map":         "application/json; charset=utf-8",
	".webmanifest": "application/manifest+json; charset=utf-8",
	".svg":         "image/svg+xml",
	".png":         "image/png",
	".jpg":         "image/jpeg",
	".jpeg":        "image/jpeg",
	".webp":        "image/webp",
	".avif":        "image/avif",
	".gif":         "image/gif",
	".ico":         "image/x-icon",
	".woff":        "font/woff",
	".woff2":       "font/woff2",
	".ttf":         "font/ttf",
	".txt":         "text/plain; charset=utf-8",
	".xml":         "application/xml; charset=utf-8",
	".wasm":        "application/wasm",
}

// noCacheFiles must always be revalidated (PWA manifest and service worker).
var noCacheFiles = map[string]bool{
	"sw.js": true, "service-worker.js": true, "registerSW.js": true,
	"manifest.json": true, "manifest.webmanifest": true,
}

type file struct {
	data []byte
	etag string
}

// Handler serves static files with an index.html fallback for SPA routes.
type Handler struct {
	fsys  fs.FS
	mu    sync.RWMutex
	cache map[string]*file
}

func New() *Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	return newHandler(sub)
}

func newHandler(fsys fs.FS) *Handler { return &Handler{fsys: fsys, cache: map[string]*file{}} }

// HasFrontend reports whether a real build (not the placeholder) is embedded.
func HasFrontend() bool {
	_, err := fs.Stat(distFS, "dist/assets")
	return err == nil
}

func (h *Handler) load(name string) (*file, bool) {
	h.mu.RLock()
	f, ok := h.cache[name]
	h.mu.RUnlock()
	if ok {
		return f, f != nil
	}
	b, err := fs.ReadFile(h.fsys, name)
	if err != nil {
		h.mu.Lock()
		h.cache[name] = nil
		h.mu.Unlock()
		return nil, false
	}
	sum := sha256.Sum256(b)
	f = &file{data: b, etag: `"` + hex.EncodeToString(sum[:8]) + `"`}
	h.mu.Lock()
	h.cache[name] = f
	h.mu.Unlock()
	return f, true
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Méthode non autorisée", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "" {
		name = "index.html"
	}
	if f, ok := h.load(name); ok && name != "index.html" {
		h.serve(w, r, name, f)
		return
	}
	// a missing file with an extension is a 404, never HTML in place of an asset
	if ext := path.Ext(name); ext != "" && name != "index.html" {
		http.NotFound(w, r)
		return
	}
	f, ok := h.load("index.html")
	if !ok {
		http.Error(w, "index.html absent du build", http.StatusInternalServerError)
		return
	}
	h.serve(w, r, "index.html", f)
}

func (h *Handler) serve(w http.ResponseWriter, r *http.Request, name string, f *file) {
	hdr := w.Header()
	ct, ok := contentTypes[strings.ToLower(path.Ext(name))]
	if name == "manifest.json" {
		ct, ok = contentTypes[".webmanifest"], true
	}
	if !ok {
		ct = http.DetectContentType(f.data)
	}
	hdr.Set("Content-Type", ct)
	switch {
	case strings.HasPrefix(name, "assets/"):
		hdr.Set("Cache-Control", "public, max-age=31536000, immutable")
	case name == "index.html", noCacheFiles[name]:
		hdr.Set("Cache-Control", "no-cache")
	default:
		hdr.Set("Cache-Control", "public, max-age=3600")
	}
	hdr.Set("ETag", f.etag)
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(f.data))
}
