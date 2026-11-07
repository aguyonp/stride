package admin

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"stride/internal/backup"
	"stride/internal/httpx"
)

// Backups & S3 (§15.4): admin-only, audited by the httpapi layer. {name} strictly matches the
// snapshot file pattern; the S3 secret key is never returned.

func (h *Handler) registerBackups(mux *http.ServeMux) {
	a := h.d.Auth.RequireAdmin
	mux.Handle("GET /api/admin/backups", a(http.HandlerFunc(h.listBackups)))
	mux.Handle("POST /api/admin/backups", a(http.HandlerFunc(h.createBackup)))
	mux.Handle("GET /api/admin/backups/s3", a(http.HandlerFunc(h.getS3)))
	mux.Handle("PUT /api/admin/backups/s3", a(http.HandlerFunc(h.putS3)))
	mux.Handle("POST /api/admin/backups/s3/test", a(http.HandlerFunc(h.testS3)))
	mux.Handle("GET /api/admin/backups/{name}/download", a(http.HandlerFunc(h.downloadBackup)))
	mux.Handle("POST /api/admin/backups/{name}/upload", a(http.HandlerFunc(h.uploadBackup)))
	mux.Handle("DELETE /api/admin/backups/{name}", a(http.HandlerFunc(h.deleteBackup)))
}

func (h *Handler) backupFail(w http.ResponseWriter, r *http.Request, err error) {
	var ce *backup.ConfigError
	var se *backup.S3Error
	switch {
	case errors.Is(err, backup.ErrInvalidName):
		httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Nom de sauvegarde invalide.")
	case errors.Is(err, backup.ErrNotFound):
		httpx.Fail(w, http.StatusNotFound, httpx.CodeNotFound, "Sauvegarde introuvable.")
	case errors.As(err, &ce):
		httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, ce.Msg)
	case errors.Is(err, backup.ErrS3NotConfigured):
		httpx.Fail(w, http.StatusPreconditionFailed, httpx.CodeNotConnected, "S3 non configuré : renseignez la cible dans « Backups & S3 ».")
	case errors.As(err, &se):
		httpx.Fail(w, http.StatusBadGateway, httpx.CodeProviderDown, backup.PublicError(err))
	default:
		h.internal(w, r, "Sauvegardes", err)
	}
}

func (h *Handler) backups(w http.ResponseWriter) (*backup.Service, bool) {
	if h.d.Backups == nil {
		h.unavailable(w)
		return nil, false
	}
	return h.d.Backups, true
}

func (h *Handler) listBackups(w http.ResponseWriter, r *http.Request) {
	b, ok := h.backups(w)
	if !ok {
		return
	}
	v, err := b.Overview(r.Context())
	if err != nil {
		h.backupFail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, v)
}

func (h *Handler) createBackup(w http.ResponseWriter, r *http.Request) {
	b, ok := h.backups(w)
	if !ok {
		return
	}
	var body struct {
		Upload bool `json:"upload"`
	}
	if r.ContentLength != 0 {
		if err := httpx.Decode(w, r, &body, 1<<10); err != nil {
			httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Corps JSON invalide ({upload} attendu).")
			return
		}
	}
	st, err := b.Enqueue(r.Context(), admin(r).ID, backup.TriggerManual, body.Upload)
	if err != nil {
		h.backupFail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusAccepted, st)
}

func (h *Handler) downloadBackup(w http.ResponseWriter, r *http.Request) {
	b, ok := h.backups(w)
	if !ok {
		return
	}
	path, it, err := b.File(r.PathValue("name"))
	if err != nil {
		h.backupFail(w, r, err)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		h.backupFail(w, r, backup.ErrNotFound)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", it.Name))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Checksum-Sha256", it.SHA256)
	created, _ := time.Parse(time.RFC3339, it.CreatedAt)
	http.ServeContent(w, r, it.Name, created, f)
}

func (h *Handler) uploadBackup(w http.ResponseWriter, r *http.Request) {
	b, ok := h.backups(w)
	if !ok {
		return
	}
	st, err := b.EnqueueUpload(r.Context(), admin(r).ID, r.PathValue("name"))
	if err != nil {
		h.backupFail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusAccepted, st)
}

func (h *Handler) deleteBackup(w http.ResponseWriter, r *http.Request) {
	b, ok := h.backups(w)
	if !ok {
		return
	}
	remote := r.URL.Query().Get("remote")
	if remote != "" && remote != "0" && remote != "1" {
		httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "remote doit valoir 0 ou 1.")
		return
	}
	if err := b.Delete(r.Context(), r.PathValue("name"), remote == "1"); err != nil {
		h.backupFail(w, r, err)
		return
	}
	h.d.Log.Info("💾 Sauvegarde supprimée par un administrateur", "adminId", admin(r).ID, "nom", r.PathValue("name"), "distante", remote == "1")
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) getS3(w http.ResponseWriter, r *http.Request) {
	b, ok := h.backups(w)
	if !ok {
		return
	}
	cfg, err := b.S3Config(r.Context())
	if err != nil {
		h.backupFail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, cfg)
}

func (h *Handler) putS3(w http.ResponseWriter, r *http.Request) {
	b, ok := h.backups(w)
	if !ok {
		return
	}
	var in backup.S3ConfigInput
	if err := httpx.Decode(w, r, &in, 4<<10); err != nil {
		httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Corps JSON invalide (configuration S3 attendue).")
		return
	}
	cfg, err := b.PutS3Config(r.Context(), in)
	if err != nil {
		h.backupFail(w, r, err)
		return
	}
	h.d.Log.Info("☁️ Cible S3 des sauvegardes modifiée", "adminId", admin(r).ID, "bucket", cfg.Bucket)
	httpx.JSON(w, http.StatusOK, cfg)
}

func (h *Handler) testS3(w http.ResponseWriter, r *http.Request) {
	b, ok := h.backups(w)
	if !ok {
		return
	}
	var body struct {
		Config *backup.S3ConfigInput `json:"config"`
	}
	if r.ContentLength != 0 {
		if err := httpx.Decode(w, r, &body, 4<<10); err != nil {
			httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, "Corps JSON invalide ({config?} attendu).")
			return
		}
	}
	httpx.JSON(w, http.StatusOK, b.TestS3(r.Context(), body.Config))
}
