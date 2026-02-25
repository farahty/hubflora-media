package handler

import (
	"net/http"

	"github.com/farahty/hubflora-media/internal/repository"
)

// GetMedia handles GET /api/v1/media/{id}
func GetMedia(mediaRepo *repository.MediaRepository, variantRepo *repository.VariantRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "not implemented"})
	}
}

// ListMedia handles GET /api/v1/media/list
func ListMedia(mediaRepo *repository.MediaRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "not implemented"})
	}
}

// BatchGetMedia handles POST /api/v1/media/batch
func BatchGetMedia(mediaRepo *repository.MediaRepository, variantRepo *repository.VariantRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "not implemented"})
	}
}

// UpdateMedia handles PATCH /api/v1/media/{id}
func UpdateMedia(mediaRepo *repository.MediaRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "not implemented"})
	}
}

// GetMediaVariants handles GET /api/v1/media/{id}/variants
func GetMediaVariants(variantRepo *repository.VariantRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "not implemented"})
	}
}
