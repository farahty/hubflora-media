package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/farahty/hubflora-media/internal/middleware"
	"github.com/farahty/hubflora-media/internal/model"
	"github.com/farahty/hubflora-media/internal/repository"
)

// GetMedia handles GET /api/v1/media/{id}
func GetMedia(mediaRepo *repository.MediaRepository, variantRepo *repository.VariantRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authCtx := middleware.MustGetAuthContext(r)
		id := chi.URLParam(r, "id")
		if id == "" {
			writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "id is required"})
			return
		}

		record, err := mediaRepo.GetByID(r.Context(), id, authCtx.OrganizationID)
		if err != nil {
			if err == pgx.ErrNoRows {
				writeJSON(w, http.StatusNotFound, model.ErrorResponse{Error: "media file not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: "failed to fetch media file"})
			return
		}

		// Load variants
		variants, err := variantRepo.GetByMediaFileID(r.Context(), record.ID)
		if err == nil {
			record.Variants = variants
		}

		writeJSON(w, http.StatusOK, model.GetMediaResponse{MediaFile: *record})
	}
}

// ListMedia handles GET /api/v1/media/list
func ListMedia(mediaRepo *repository.MediaRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authCtx := middleware.MustGetAuthContext(r)

		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		search := r.URL.Query().Get("search")
		mimeType := r.URL.Query().Get("type")
		sortBy := r.URL.Query().Get("sort")
		order := r.URL.Query().Get("order")

		// Map "image" → "image/", "video" → "video/", etc.
		mimePrefix := ""
		if mimeType != "" {
			mimePrefix = mimeType + "/"
		}

		items, total, err := mediaRepo.List(r.Context(), authCtx.OrganizationID, repository.ListOptions{
			Limit:      limit,
			Offset:     offset,
			Search:     search,
			MimePrefix: mimePrefix,
			SortBy:     sortBy,
			SortOrder:  order,
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: "failed to list media"})
			return
		}

		if items == nil {
			items = []model.MediaFileRecord{}
		}

		writeJSON(w, http.StatusOK, model.ListMediaResponse{Items: items, Total: total})
	}
}

// BatchGetMedia handles POST /api/v1/media/batch
func BatchGetMedia(mediaRepo *repository.MediaRepository, variantRepo *repository.VariantRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authCtx := middleware.MustGetAuthContext(r)

		var req model.BatchGetRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "invalid JSON body"})
			return
		}

		if len(req.IDs) == 0 {
			writeJSON(w, http.StatusOK, model.BatchGetResponse{Items: []model.MediaFileRecord{}})
			return
		}
		if len(req.IDs) > 100 {
			writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "max 100 IDs per batch"})
			return
		}

		items, err := mediaRepo.GetByIDs(r.Context(), req.IDs, authCtx.OrganizationID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: "failed to batch fetch"})
			return
		}

		// Load variants for each item
		for i := range items {
			variants, err := variantRepo.GetByMediaFileID(r.Context(), items[i].ID)
			if err == nil {
				items[i].Variants = variants
			}
		}

		if items == nil {
			items = []model.MediaFileRecord{}
		}

		writeJSON(w, http.StatusOK, model.BatchGetResponse{Items: items})
	}
}

// UpdateMedia handles PATCH /api/v1/media/{id}
func UpdateMedia(mediaRepo *repository.MediaRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authCtx := middleware.MustGetAuthContext(r)
		id := chi.URLParam(r, "id")
		if id == "" {
			writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "id is required"})
			return
		}

		var req model.UpdateMediaRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "invalid JSON body"})
			return
		}

		fields := repository.UpdateFields{
			Alt:         req.Alt,
			Caption:     req.Caption,
			Description: req.Description,
			IsPrivate:   req.IsPrivate,
		}

		if err := mediaRepo.Update(r.Context(), id, authCtx.OrganizationID, fields); err != nil {
			writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: "failed to update"})
			return
		}

		// Return updated record
		record, err := mediaRepo.GetByID(r.Context(), id, authCtx.OrganizationID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, model.ErrorResponse{Error: "media file not found"})
			return
		}

		writeJSON(w, http.StatusOK, model.GetMediaResponse{MediaFile: *record})
	}
}

// GetMediaVariants handles GET /api/v1/media/{id}/variants
func GetMediaVariants(variantRepo *repository.VariantRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "id is required"})
			return
		}

		variants, err := variantRepo.GetByMediaFileID(r.Context(), id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: "failed to fetch variants"})
			return
		}

		if variants == nil {
			variants = []model.MediaVariantRecord{}
		}

		writeJSON(w, http.StatusOK, map[string]any{"variants": variants})
	}
}
