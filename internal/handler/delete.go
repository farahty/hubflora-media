package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/farahty/hubflora-media/internal/config"
	"github.com/farahty/hubflora-media/internal/model"
	"github.com/farahty/hubflora-media/internal/storage"
)

// DeleteRequest specifies what to delete.
type DeleteRequest struct {
	ObjectKey  string `json:"objectKey"`
	BucketName string `json:"bucketName"`
}

// Delete handles DELETE /api/v1/media.
// Deletes the original file and all variants under the same folder prefix.
func Delete(cfg *config.Config, s3 *storage.S3Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req DeleteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "invalid JSON body"})
			return
		}

		if req.ObjectKey == "" {
			writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "objectKey is required"})
			return
		}

		bucket := req.BucketName
		if bucket == "" {
			bucket = cfg.MinioDefaultBucket
		}

		// Delete the entire folder (all variants + original)
		folderPath := storage.ExtractFolderPath(req.ObjectKey)
		if folderPath != "" {
			if err := s3.DeletePrefix(r.Context(), bucket, folderPath+"/"); err != nil {
				slog.Error("failed to delete folder", "path", folderPath, "error", err)
				writeJSON(w, http.StatusInternalServerError, model.DeleteResponse{
					Success: false,
					Error:   "failed to delete files",
				})
				return
			}
		} else {
			// Single file delete
			if err := s3.Delete(r.Context(), bucket, req.ObjectKey); err != nil {
				slog.Error("failed to delete file", "key", req.ObjectKey, "error", err)
				writeJSON(w, http.StatusInternalServerError, model.DeleteResponse{
					Success: false,
					Error:   "failed to delete file",
				})
				return
			}
		}

		writeJSON(w, http.StatusOK, model.DeleteResponse{Success: true})
	}
}
