package handler

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/farahty/hubflora-media/internal/config"
	"github.com/farahty/hubflora-media/internal/storage"
)

// Download handles GET /api/v1/media/{bucket}/{objectKey...} — streams the file.
func Download(cfg *config.Config, s3 *storage.S3Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bucket := chi.URLParam(r, "bucket")
		if bucket == "" {
			bucket = cfg.MinioDefaultBucket
		}

		objectKey := chi.URLParam(r, "*")
		if objectKey == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "objectKey is required"})
			return
		}

		// Get object info
		info, err := s3.Stat(r.Context(), bucket, objectKey)
		if err != nil {
			slog.Error("stat failed", "error", err, "key", objectKey)
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
			return
		}

		reader, err := s3.Download(r.Context(), bucket, objectKey)
		if err != nil {
			slog.Error("download failed", "error", err, "key", objectKey)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to download file"})
			return
		}
		defer reader.Close()

		w.Header().Set("Content-Type", info.ContentType)
		w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size))
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")

		io.Copy(w, reader)
	}
}
