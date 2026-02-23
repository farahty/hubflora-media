package handler

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/farahty/hubflora-media/internal/config"
	"github.com/farahty/hubflora-media/internal/model"
	"github.com/farahty/hubflora-media/internal/storage"
)

// Presign handles GET /api/v1/media/presign?objectKey=...&bucket=...&expiry=3600.
func Presign(cfg *config.Config, s3 *storage.S3Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		objectKey := r.URL.Query().Get("objectKey")
		if objectKey == "" {
			writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "objectKey is required"})
			return
		}

		bucket := r.URL.Query().Get("bucket")
		if bucket == "" {
			bucket = cfg.MinioDefaultBucket
		}

		expirySec := 3600
		if v := r.URL.Query().Get("expiry"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 86400 {
				expirySec = n
			}
		}

		expiry := time.Duration(expirySec) * time.Second

		url, err := s3.PresignedGetURL(r.Context(), bucket, objectKey, expiry)
		if err != nil {
			slog.Error("presign failed", "error", err)
			writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: "failed to generate presigned URL"})
			return
		}

		expiresAt := time.Now().Add(expiry).UTC().Format(time.RFC3339)

		writeJSON(w, http.StatusOK, model.PresignedDownloadResponse{
			URL:       url,
			ExpiresAt: expiresAt,
		})
	}
}
