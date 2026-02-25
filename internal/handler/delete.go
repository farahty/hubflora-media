package handler

import (
	"log/slog"
	"net/http"

	"github.com/farahty/hubflora-media/internal/config"
	"github.com/farahty/hubflora-media/internal/middleware"
	"github.com/farahty/hubflora-media/internal/model"
	"github.com/farahty/hubflora-media/internal/repository"
	"github.com/farahty/hubflora-media/internal/storage"
)

// Delete handles DELETE /api/v1/media.
// Deletes the media file from DB and all associated S3 objects.
func Delete(cfg *config.Config, s3 *storage.S3Client, mediaRepo *repository.MediaRepository, variantRepo *repository.VariantRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authCtx := middleware.MustGetAuthContext(r)

		var req struct {
			ID         string `json:"id"`
			ObjectKey  string `json:"objectKey"`
			BucketName string `json:"bucketName"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "invalid JSON body"})
			return
		}

		bucket := req.BucketName
		if bucket == "" {
			bucket = cfg.MinioDefaultBucket
		}

		var objectKey string

		if req.ID != "" {
			// Look up by ID
			record, err := mediaRepo.GetByID(r.Context(), req.ID, authCtx.OrganizationID)
			if err != nil {
				writeJSON(w, http.StatusNotFound, model.ErrorResponse{Error: "media file not found"})
				return
			}
			objectKey = record.ObjectKey

			// Delete from DB
			variantRepo.DeleteByMediaFileID(r.Context(), req.ID)
			mediaRepo.Delete(r.Context(), req.ID, authCtx.OrganizationID)
		} else if req.ObjectKey != "" {
			objectKey = req.ObjectKey

			// Try to find and delete from DB
			record, err := mediaRepo.GetByObjectKey(r.Context(), req.ObjectKey, authCtx.OrganizationID)
			if err == nil && record != nil {
				variantRepo.DeleteByMediaFileID(r.Context(), record.ID)
				mediaRepo.Delete(r.Context(), record.ID, authCtx.OrganizationID)
			}
		} else {
			writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "id or objectKey is required"})
			return
		}

		// Delete from S3
		folderPath := storage.ExtractFolderPath(objectKey)
		if folderPath != "" {
			if err := s3.DeletePrefix(r.Context(), bucket, folderPath+"/"); err != nil {
				slog.Error("failed to delete S3 folder", "path", folderPath, "error", err)
			}
		} else {
			if err := s3.Delete(r.Context(), bucket, objectKey); err != nil {
				slog.Error("failed to delete S3 object", "key", objectKey, "error", err)
			}
		}

		writeJSON(w, http.StatusOK, model.DeleteResponse{Success: true})
	}
}
