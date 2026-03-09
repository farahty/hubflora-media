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
	"github.com/farahty/hubflora-media/internal/middleware"
	"github.com/farahty/hubflora-media/internal/model"
	"github.com/farahty/hubflora-media/internal/processing"
	"github.com/farahty/hubflora-media/internal/queue"
	"github.com/farahty/hubflora-media/internal/repository"
	"github.com/farahty/hubflora-media/internal/storage"
)

// Upload handles POST /api/v1/media/upload.
// Accepts multipart/form-data with a "file" field and optional form values.
func Upload(cfg *config.Config, s3 *storage.S3Client, proc *processing.Processor, asynqClient *asynq.Client, mediaRepo *repository.MediaRepository, variantRepo *repository.VariantRepository) http.HandlerFunc {
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

		// Get auth context
		authCtx := middleware.MustGetAuthContext(r)

		// orgSlug from form takes priority, then from auth context (JWT)
		orgSlug := r.FormValue("orgSlug")
		if orgSlug == "" {
			orgSlug = authCtx.OrgSlug
		}
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

		// Detect MIME type from content (magic bytes)
		mimeType := http.DetectContentType(data)
		// http.DetectContentType returns "application/octet-stream" for unrecognized formats
		// (e.g. some WebP/AVIF). Trust the client Content-Type header in that case.
		if mimeType == "application/octet-stream" && header.Header.Get("Content-Type") != "" {
			mimeType = header.Header.Get("Content-Type")
		}
		// Normalize: strip params (e.g. "text/plain; charset=utf-8" → "text/plain")
		if idx := strings.Index(mimeType, ";"); idx > 0 {
			mimeType = strings.TrimSpace(mimeType[:idx])
		}

		// SVG detection: http.DetectContentType returns "text/xml" for SVGs
		if mimeType == "text/xml" || mimeType == "application/xml" {
			extLower := strings.ToLower(filepath.Ext(header.Filename))
			if extLower == ".svg" {
				mimeType = "image/svg+xml"
			} else {
				// Check file content for <svg tag
				snippet := string(data[:min(len(data), 1024)])
				if strings.Contains(strings.ToLower(snippet), "<svg") {
					mimeType = "image/svg+xml"
				}
			}
		}

		// Validate file type
		if !processing.IsAllowedMimeType(mimeType) {
			writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: fmt.Sprintf("file type %q is not allowed", mimeType)})
			return
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

		now := time.Now()
		record := &model.MediaFileRecord{
			ID:               uuid.New().String(),
			Filename:         storage.GenerateFileFolderName(originalFilename),
			OriginalFilename: originalFilename,
			MimeType:         mimeType,
			FileSize:         int64(len(data)),
			BucketName:       bucket,
			ObjectKey:        objectKey,
			URL:              publicURL,
			OrganizationID:   &authCtx.OrganizationID,
			UploadedBy:       authCtx.UserID,
			CreatedAt:        now,
			UpdatedAt:        now,
		}

		// Set alt/caption/description from form
		if alt := r.FormValue("alt"); alt != "" {
			record.Alt = &alt
		}
		if caption := r.FormValue("caption"); caption != "" {
			record.Caption = &caption
		}
		if description := r.FormValue("description"); description != "" {
			record.Description = &description
		}

		// Extract image metadata (raster images + SVGs via libvips)
		if processing.IsProcessableImageMimeType(mimeType) {
			meta, err := proc.GetMetadata(data)
			if err == nil {
				imgW := meta.Width
				imgH := meta.Height
				record.Width = &imgW
				record.Height = &imgH
				record.Metadata = map[string]any{
					"format":      meta.Format,
					"space":       meta.Space,
					"channels":    meta.Channels,
					"orientation": meta.Orientation,
				}
			}
		}

		// Extract video metadata (dimensions, duration, codec via ffprobe)
		if processing.IsVideoMimeType(mimeType) {
			meta, err := processing.ExtractVideoMetadata(data)
			if err == nil {
				record.Width = &meta.Width
				record.Height = &meta.Height
				record.Duration = &meta.Duration
				record.Metadata = map[string]any{
					"codec":  meta.Codec,
					"format": meta.Format,
				}
			} else {
				slog.Warn("video metadata extraction failed", "error", err)
			}
		}

		// Persist to DB
		if err := mediaRepo.Create(r.Context(), record); err != nil {
			slog.Error("failed to insert media_files record", "error", err)
			writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: "failed to persist media record"})
			return
		}

		// Generate variants (raster images + SVGs)
		if generateVariants && processing.IsProcessableImageMimeType(mimeType) {
			if async && asynqClient != nil {
				task, err := queue.NewVariantTask(record.ID, bucket, folderPath, objectKey, mimeType)
				if err == nil {
					info, err := asynqClient.Enqueue(task, asynq.MaxRetry(3), asynq.Timeout(5*time.Minute))
					if err != nil {
						slog.Warn("failed to enqueue variant task", "error", err)
					} else {
						writeJSON(w, http.StatusAccepted, model.UploadResponse{
							Success:   true,
							MediaFile: record.ToMediaFile(),
							JobID:     info.ID,
						})
						return
					}
				}
			}

			// Sync variant generation
			variants := queue.ProcessVariants(r.Context(), s3, proc, data, bucket, folderPath)
			if len(variants) > 0 {
				variantRecords := repository.ToRecords(record.ID, variants)
				if err := variantRepo.CreateBatch(r.Context(), variantRecords); err != nil {
					slog.Warn("failed to persist variant records", "error", err)
				}

				for _, v := range variants {
					if v.Name == "thumbnail" {
						thumbURL := v.URL
						record.ThumbnailURL = &thumbURL
						mediaRepo.Update(r.Context(), record.ID, authCtx.OrganizationID,
							repository.UpdateFields{ThumbnailURL: &thumbURL})
						break
					}
				}
				record.Variants = variantRecords
			}
		}

		// Generate video thumbnail (extract frame → resize → upload as WebP)
		if generateVariants && processing.IsVideoMimeType(mimeType) {
			frameData, err := processing.ExtractVideoThumbnail(data)
			if err != nil {
				slog.Warn("video thumbnail extraction failed", "error", err)
			} else {
				thumbVariant := processing.DefaultVariants[0] // "thumbnail"
				result, err := proc.ProcessImage(frameData, thumbVariant)
				if err != nil {
					slog.Warn("video thumbnail processing failed", "error", err)
				} else {
					thumbKey := storage.GenerateVariantKey(folderPath, "thumbnail", "webp")
					if err := s3.Upload(r.Context(), bucket, thumbKey, bytes.NewReader(result.Data),
						int64(len(result.Data)), result.MimeType); err != nil {
						slog.Warn("video thumbnail upload failed", "error", err)
					} else {
						thumbURL := s3.GetPublicURL(bucket, thumbKey)
						record.ThumbnailURL = &thumbURL

						variant := model.MediaVariant{
							Name:      "thumbnail",
							Width:     result.Width,
							Height:    result.Height,
							FileSize:  int64(len(result.Data)),
							ObjectKey: thumbKey,
							URL:       thumbURL,
							MimeType:  result.MimeType,
						}
						variantRecords := repository.ToRecords(record.ID, []model.MediaVariant{variant})
						if err := variantRepo.CreateBatch(r.Context(), variantRecords); err != nil {
							slog.Warn("failed to persist video thumbnail record", "error", err)
						}
						record.Variants = variantRecords

						mediaRepo.Update(r.Context(), record.ID, authCtx.OrganizationID,
							repository.UpdateFields{ThumbnailURL: &thumbURL})
					}
				}
			}
		}

		writeJSON(w, http.StatusOK, model.UploadResponse{
			Success:   true,
			MediaFile: record.ToMediaFile(),
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
			UploadURL:  uploadURL,
			ObjectKey:  objectKey,
			BucketName: bucket,
		})
	}
}
