package handler

import (
	"bytes"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/farahty/hubflora-media/internal/config"
	"github.com/farahty/hubflora-media/internal/model"
	"github.com/farahty/hubflora-media/internal/processing"
	"github.com/farahty/hubflora-media/internal/queue"
	"github.com/farahty/hubflora-media/internal/storage"
)

// CropInputRequest is the JSON body for cropping.
type CropInputRequest struct {
	ObjectKey          string  `json:"objectKey"`
	BucketName         string  `json:"bucketName"`
	X                  int     `json:"x"`
	Y                  int     `json:"y"`
	Width              int     `json:"width"`
	Height             int     `json:"height"`
	Rotate             float64 `json:"rotate"`             // rotation degrees (0, 90, 180, 270)
	Scale              float64 `json:"scale"`              // scale factor (1.0 = no scale)
	Quality            int     `json:"quality"`            // output quality (1-100, default 90)
	Format             string  `json:"format"`             // output format: "webp", "jpeg", "png" (default "webp")
	RegenerateVariants bool    `json:"regenerateVariants"`
	Async              bool    `json:"async"`
}

// Crop handles POST /api/v1/media/crop.
// Crops the image, replaces the original, and optionally regenerates all variants.
func Crop(cfg *config.Config, s3 *storage.S3Client, proc *processing.Processor, asynqClient *asynq.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CropInputRequest
		if err := decodeJSON(r, &req); err != nil {
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
		result, err := proc.CropImage(original, processing.CropOptions{
			X:       req.X,
			Y:       req.Y,
			Width:   req.Width,
			Height:  req.Height,
			Rotate:  req.Rotate,
			Scale:   req.Scale,
			Quality: req.Quality,
			Format:  req.Format,
		})
		if err != nil {
			slog.Error("crop failed", "error", err)
			writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: "image crop failed"})
			return
		}

		// Replace the original file with the cropped version
		if err := s3.Upload(r.Context(), bucket, req.ObjectKey, bytes.NewReader(result.Data), int64(len(result.Data)), result.MimeType); err != nil {
			slog.Error("crop upload failed", "error", err)
			writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: "failed to upload cropped image"})
			return
		}

		publicURL := s3.GetPublicURL(bucket, req.ObjectKey)
		w_ := result.Width
		h_ := result.Height

		mediaFile := &model.MediaFile{
			ID:         uuid.New().String(),
			BucketName: bucket,
			ObjectKey:  req.ObjectKey,
			URL:        publicURL,
			MimeType:   result.MimeType,
			FileSize:   int64(len(result.Data)),
			Width:      &w_,
			Height:     &h_,
			CreatedAt:  time.Now(),
		}

		// Regenerate variants from the cropped image
		if req.RegenerateVariants {
			folderPath := storage.ExtractFolderPath(req.ObjectKey)

			if req.Async && asynqClient != nil {
				task, err := queue.NewVariantTask(mediaFile.ID, bucket, folderPath, req.ObjectKey)
				if err == nil {
					info, err := asynqClient.Enqueue(task, asynq.MaxRetry(3))
					if err == nil {
						writeJSON(w, http.StatusAccepted, model.CropResponse{
							Success:   true,
							MediaFile: mediaFile,
							JobID:     info.ID,
						})
						return
					}
					slog.Warn("failed to enqueue variant regen after crop", "error", err)
				}
			}

			// Sync variant regeneration
			variants := queue.ProcessVariants(r.Context(), s3, proc, result.Data, bucket, folderPath)
			mediaFile.Variants = variants
			if len(variants) > 0 {
				for _, v := range variants {
					if v.Name == "thumbnail" {
						thumbURL := v.URL
						mediaFile.ThumbnailURL = &thumbURL
						break
					}
				}
			}
		}

		writeJSON(w, http.StatusOK, model.CropResponse{
			Success:   true,
			MediaFile: mediaFile,
		})
	}
}

// VariantsInfo handles GET /api/v1/media/variants/info?objectKey=...&bucket=...
// Lists existing variant files for a given folder path by scanning S3.
func VariantsInfo(cfg *config.Config, s3 *storage.S3Client) http.HandlerFunc {
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

		folderPath := storage.ExtractFolderPath(objectKey)
		if folderPath == "" {
			writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "invalid objectKey"})
			return
		}

		objects, err := s3.ListObjects(r.Context(), bucket, folderPath+"/")
		if err != nil {
			slog.Error("failed to list variants", "error", err)
			writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: "failed to list variants"})
			return
		}

		var variants []model.MediaVariant
		for _, obj := range objects {
			// Skip the original file
			if obj.Key == objectKey {
				continue
			}
			variants = append(variants, model.MediaVariant{
				ObjectKey: obj.Key,
				URL:       s3.GetPublicURL(bucket, obj.Key),
				FileSize:  obj.Size,
				MimeType:  obj.ContentType,
			})
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"objectKey": objectKey,
			"variants":  variants,
		})
	}
}
