// Package web sert le SPA (build Vite) embarqué dans le binaire.
// Au build Docker, frontend/dist est copié dans internal/web/dist ; sinon un
// placeholder index.html (« build frontend absent ») est servi.
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

// noCacheFiles : fichiers à revalider systématiquement (service worker PWA éventuel).
var noCacheFiles = map[string]bool{"sw.js": true, "service-worker.js": true, "registerSW.js": true}

type file struct {
	data []byte
	etag string
}

// Handler sert les fichiers statiques avec repli sur index.html (routes SPA).
type Handler struct {
	fsys  fs.FS
	mu    sync.RWMutex
	cache map[string]*file
}

// New construit le handler sur le dist embarqué.
func New() *Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	return &Handler{fsys: sub, cache: map[string]*file{}}
}

// HasFrontend indique si un vrai build (et non le placeholder) est embarqué.
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
	// fichier avec extension introuvable → 404 (ne pas renvoyer du HTML à la place d'un asset)
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
	if !ok {
		ct = http.DetectContentType(f.data)
	}
	hdr.Set("Content-Type", ct)
	switch {
	case strings.HasPrefix(name, "assets/"):
		hdr.Set("Cache-Control", "public, max-age=31536000, immutable")
	case name == "index.html", path.Ext(name) == ".webmanifest", noCacheFiles[name]:
		hdr.Set("Cache-Control", "no-cache")
	default:
		hdr.Set("Cache-Control", "public, max-age=3600")
	}
	hdr.Set("ETag", f.etag)
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(f.data))
}
