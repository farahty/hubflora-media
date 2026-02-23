package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/farahty/hubflora-media/internal/config"
	"github.com/farahty/hubflora-media/internal/model"
	"github.com/farahty/hubflora-media/internal/processing"
	"github.com/farahty/hubflora-media/internal/storage"
)

// CropInputRequest is the JSON body for cropping.
type CropInputRequest struct {
	ObjectKey  string `json:"objectKey"`
	BucketName string `json:"bucketName"`
	X          int    `json:"x"`
	Y          int    `json:"y"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
}

// Crop handles POST /api/v1/media/crop.
func Crop(cfg *config.Config, s3 *storage.S3Client, proc *processing.Processor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CropInputRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "invalid JSON body"})
			return
		}

		if req.ObjectKey == "" || req.Width <= 0 || req.Height <= 0 {
			writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "objectKey, width, and height are required"})
			return
		}

		bucket := req.BucketName
		if bucket == "" {
			bucket = cfg.MinioDefaultBucket
		}

		// Download original
		original, err := s3.GetBuffer(r.Context(), bucket, req.ObjectKey)
		if err != nil {
			slog.Error("failed to download for crop", "error", err)
			writeJSON(w, http.StatusNotFound, model.ErrorResponse{Error: "source file not found"})
			return
		}

		// Crop
		result, err := proc.CropImage(original, req.X, req.Y, req.Width, req.Height)
		if err != nil {
			slog.Error("crop failed", "error", err)
			writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: "image crop failed"})
			return
		}

		// Upload cropped image alongside the original
		folderPath := storage.ExtractFolderPath(req.ObjectKey)
		croppedKey := fmt.Sprintf("%s/cropped.%s", folderPath, result.Format.String())

		if err := s3.Upload(r.Context(), bucket, croppedKey, bytes.NewReader(result.Data), int64(len(result.Data)), result.MimeType); err != nil {
			slog.Error("crop upload failed", "error", err)
			writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: "failed to upload cropped image"})
			return
		}

		croppedURL := s3.GetPublicURL(bucket, croppedKey)
		w_ := result.Width
		h_ := result.Height

		writeJSON(w, http.StatusOK, model.CropResponse{
			Success: true,
			MediaFile: &model.MediaFile{
				ID:        uuid.New().String(),
				ObjectKey: croppedKey,
				URL:       croppedURL,
				MimeType:  result.MimeType,
				FileSize:  int64(len(result.Data)),
				Width:     &w_,
				Height:    &h_,
				CreatedAt: time.Now(),
			},
		})
	}
}
