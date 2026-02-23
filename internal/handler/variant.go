package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/farahty/hubflora-media/internal/config"
	"github.com/farahty/hubflora-media/internal/processing"
	"github.com/farahty/hubflora-media/internal/storage"
)

// VariantRedirect handles GET /api/v1/media/variant/{bucket}/{variantName}/{objectKeyPrefix...}.
// Constructs the variant object key and redirects to its public URL.
func VariantRedirect(cfg *config.Config, s3 *storage.S3Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bucket := chi.URLParam(r, "bucket")
		if bucket == "" {
			bucket = cfg.MinioDefaultBucket
		}

		variantName := chi.URLParam(r, "variantName")
		folderPath := chi.URLParam(r, "*")

		if variantName == "" || folderPath == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "variantName and folder path are required"})
			return
		}

		// Validate variant name
		validVariants := map[string]bool{
			"thumbnail": true, "small": true, "medium": true, "large": true, "original_webp": true,
		}
		if !validVariants[variantName] {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid variant name"})
			return
		}

		ext := processing.FindVariant(variantName).Format.String()
		variantKey := storage.GenerateVariantKey(folderPath, variantName, ext)

		// Check existence
		_, err := s3.Stat(r.Context(), bucket, variantKey)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "variant not found"})
			return
		}

		url := s3.GetPublicURL(bucket, variantKey)
		http.Redirect(w, r, url, http.StatusFound)
	}
}
