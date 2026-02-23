package handler

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/farahty/hubflora-media/internal/config"
	"github.com/farahty/hubflora-media/internal/model"
	"github.com/farahty/hubflora-media/internal/processing"
	"github.com/farahty/hubflora-media/internal/queue"
	"github.com/farahty/hubflora-media/internal/storage"
)

// Upload handles POST /api/v1/media/upload.
// Accepts multipart/form-data with a "file" field and optional form values.
func Upload(cfg *config.Config, s3 *storage.S3Client, proc *processing.Processor, asynqClient *asynq.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse multipart form with max upload size
		if err := r.ParseMultipartForm(cfg.MaxUploadSize); err != nil {
			writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "file too large or invalid form data"})
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "missing file field"})
			return
		}
		defer file.Close()

		orgSlug := r.FormValue("orgSlug")
		if orgSlug == "" {
			writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "orgSlug is required"})
			return
		}

		generateVariants := r.FormValue("generateVariants") == "true"
		async := r.FormValue("async") == "true"

		// Read file into buffer
		data, err := io.ReadAll(file)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: "failed to read file"})
			return
		}

		// Detect MIME type from content
		mimeType := http.DetectContentType(data)
		// Fallback to header content type if detection gives generic result
		if mimeType == "application/octet-stream" && header.Header.Get("Content-Type") != "" {
			mimeType = header.Header.Get("Content-Type")
		}

		// Generate object key
		originalFilename := header.Filename
		ext := strings.TrimPrefix(filepath.Ext(originalFilename), ".")
		if ext == "" {
			ext = storage.FileExtensionFromMimeType(mimeType)
		}

		folderName := storage.GenerateTimestampedFolderName(originalFilename)
		objectKey := fmt.Sprintf("%s/%s/original.%s", orgSlug, folderName, ext)
		folderPath := storage.ExtractFolderPath(objectKey)

		bucket := cfg.MinioDefaultBucket

		// Upload original file
		if err := s3.Upload(r.Context(), bucket, objectKey, bytes.NewReader(data), int64(len(data)), mimeType); err != nil {
			slog.Error("upload failed", "error", err)
			writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: "failed to upload file"})
			return
		}

		publicURL := s3.GetPublicURL(bucket, objectKey)

		mediaFile := &model.MediaFile{
			ID:               uuid.New().String(),
			Filename:         storage.GenerateFileFolderName(originalFilename),
			OriginalFilename: originalFilename,
			MimeType:         mimeType,
			FileSize:         int64(len(data)),
			BucketName:       bucket,
			ObjectKey:        objectKey,
			URL:              publicURL,
			CreatedAt:        time.Now(),
		}

		// Extract image metadata
		if processing.IsImageMimeType(mimeType) {
			meta, err := proc.GetMetadata(data)
			if err == nil {
				imgW := meta.Width
				imgH := meta.Height
				mediaFile.Width = &imgW
				mediaFile.Height = &imgH
				mediaFile.Metadata = map[string]any{
					"format":      meta.Format,
					"space":       meta.Space,
					"channels":    meta.Channels,
					"orientation": meta.Orientation,
				}
			}
		}

		// Generate variants
		if generateVariants && processing.IsImageMimeType(mimeType) {
			if async && asynqClient != nil {
				// Queue async variant generation — stores S3 reference, not file bytes
				task, err := queue.NewVariantTask(mediaFile.ID, bucket, folderPath, objectKey)
				if err == nil {
					info, err := asynqClient.Enqueue(task, asynq.MaxRetry(3), asynq.Timeout(5*time.Minute))
					if err != nil {
						slog.Warn("failed to enqueue variant task", "error", err)
					} else {
						writeJSON(w, http.StatusAccepted, model.UploadResponse{
							Success:   true,
							MediaFile: mediaFile,
							JobID:     info.ID,
						})
						return
					}
				}
			}

			// Sync variant generation
			variants := queue.ProcessVariants(r.Context(), s3, proc, data, bucket, folderPath)
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

		writeJSON(w, http.StatusOK, model.UploadResponse{
			Success:   true,
			MediaFile: mediaFile,
		})
	}
}

// PresignedUpload handles POST /api/v1/media/upload/presigned.
func PresignedUpload(cfg *config.Config, s3 *storage.S3Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req model.PresignedUploadRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "invalid JSON body"})
			return
		}

		if req.OrgSlug == "" || req.Filename == "" {
			writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "orgSlug and filename are required"})
			return
		}

		ext := strings.TrimPrefix(filepath.Ext(req.Filename), ".")
		if ext == "" {
			ext = storage.FileExtensionFromMimeType(req.MimeType)
		}

		folderName := storage.GenerateTimestampedFolderName(req.Filename)
		objectKey := fmt.Sprintf("%s/%s/original.%s", req.OrgSlug, folderName, ext)

		bucket := cfg.MinioDefaultBucket

		uploadURL, err := s3.PresignedPutURL(r.Context(), bucket, objectKey, 1*time.Hour)
		if err != nil {
			slog.Error("presigned URL failed", "error", err)
			writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: "failed to generate presigned URL"})
			return
		}

		writeJSON(w, http.StatusOK, model.PresignedUploadResponse{
			UploadURL: uploadURL,
			ObjectKey: objectKey,
		})
	}
}
