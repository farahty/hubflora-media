package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/hibiken/asynq"

	"github.com/farahty/hubflora-media/internal/model"
	"github.com/farahty/hubflora-media/internal/processing"
	"github.com/farahty/hubflora-media/internal/repository"
	"github.com/farahty/hubflora-media/internal/storage"
)

const TypeVariantGenerate = "media:variant_generate"

// VariantPayload is the task payload for async variant generation.
// FileData is NOT embedded — the worker downloads from S3 using ObjectKey.
type VariantPayload struct {
	MediaID    string `json:"mediaId"`
	BucketName string `json:"bucketName"`
	FolderPath string `json:"folderPath"`
	ObjectKey  string `json:"objectKey"`
	MimeType   string `json:"mimeType"`
}

// NewVariantTask creates a new asynq task for variant generation.
// Only stores the S3 reference, not the file bytes.
func NewVariantTask(mediaID, bucket, folderPath, objectKey, mimeType string) (*asynq.Task, error) {
	payload, err := json.Marshal(VariantPayload{
		MediaID:    mediaID,
		BucketName: bucket,
		FolderPath: folderPath,
		ObjectKey:  objectKey,
		MimeType:   mimeType,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal variant payload: %w", err)
	}
	return asynq.NewTask(TypeVariantGenerate, payload), nil
}

// VariantHandler processes async variant generation tasks.
type VariantHandler struct {
	s3          *storage.S3Client
	proc        *processing.Processor
	mediaRepo   *repository.MediaRepository
	variantRepo *repository.VariantRepository
}

// NewVariantHandler creates a new handler for variant tasks.
func NewVariantHandler(s3 *storage.S3Client, proc *processing.Processor, mediaRepo *repository.MediaRepository, variantRepo *repository.VariantRepository) *VariantHandler {
	return &VariantHandler{s3: s3, proc: proc, mediaRepo: mediaRepo, variantRepo: variantRepo}
}

// ProcessTask implements the asynq.Handler interface.
func (h *VariantHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var payload VariantPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	slog.Info("processing variant generation", "mediaId", payload.MediaID, "folder", payload.FolderPath)

	// Download original file from S3
	data, err := h.s3.GetBuffer(ctx, payload.BucketName, payload.ObjectKey)
	if err != nil {
		return fmt.Errorf("failed to download original file %q: %w", payload.ObjectKey, err)
	}

	// Detect MIME type if not provided (backward compat with old payloads)
	mimeType := payload.MimeType
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}

	var variants []model.MediaVariant

	if processing.IsVideoMimeType(mimeType) {
		// Video: extract a single thumbnail frame
		variants = processVideoThumbnail(ctx, h.s3, h.proc, data, payload.BucketName, payload.FolderPath)
	} else {
		// Image (raster + SVG): generate all default variants
		variants = ProcessVariants(ctx, h.s3, h.proc, data, payload.BucketName, payload.FolderPath)
	}

	// Persist variants to DB
	if len(variants) > 0 {
		variantRecords := repository.ToRecords(payload.MediaID, variants)
		if err := h.variantRepo.CreateBatch(ctx, variantRecords); err != nil {
			slog.Warn("failed to persist async variant records", "error", err)
		}

		// Update thumbnail URL on the media file
		for _, v := range variants {
			if v.Name == "thumbnail" {
				thumbURL := v.URL
				h.mediaRepo.UpdateThumbnail(ctx, payload.MediaID, thumbURL)
				break
			}
		}
	}

	slog.Info("variant generation complete", "mediaId", payload.MediaID, "count", len(variants))
	return nil
}

// ProcessVariants generates all default variants from image data and uploads them to S3.
// Shared between sync upload, async worker, and crop re-generation.
func ProcessVariants(ctx context.Context, s3 *storage.S3Client, proc *processing.Processor, original []byte, bucket, folderPath string) []model.MediaVariant {
	var variants []model.MediaVariant

	for _, variantDef := range processing.DefaultVariants {
		result, err := proc.ProcessImage(original, variantDef)
		if err != nil {
			slog.Warn("variant processing failed", "variant", variantDef.Name, "error", err)
			continue
		}

		variantKey := storage.GenerateVariantKey(folderPath, variantDef.Name, result.Format.String())

		if err := s3.Upload(ctx, bucket, variantKey, bytes.NewReader(result.Data), int64(len(result.Data)), result.MimeType); err != nil {
			slog.Warn("variant upload failed", "variant", variantDef.Name, "error", err)
			continue
		}

		variants = append(variants, model.MediaVariant{
			Name:      variantDef.Name,
			Width:     result.Width,
			Height:    result.Height,
			FileSize:  int64(len(result.Data)),
			ObjectKey: variantKey,
			URL:       s3.GetPublicURL(bucket, variantKey),
			MimeType:  result.MimeType,
		})

		slog.Info("variant generated", "variant", variantDef.Name, "width", result.Width, "height", result.Height)
	}

	return variants
}

// processVideoThumbnail extracts a frame from video data and generates a WebP thumbnail.
func processVideoThumbnail(ctx context.Context, s3 *storage.S3Client, proc *processing.Processor, videoData []byte, bucket, folderPath string) []model.MediaVariant {
	frameData, err := processing.ExtractVideoThumbnail(videoData)
	if err != nil {
		slog.Warn("async video thumbnail extraction failed", "error", err)
		return nil
	}

	thumbVariant := processing.DefaultVariants[0] // "thumbnail"
	result, err := proc.ProcessImage(frameData, thumbVariant)
	if err != nil {
		slog.Warn("async video thumbnail processing failed", "error", err)
		return nil
	}

	thumbKey := storage.GenerateVariantKey(folderPath, "thumbnail", "webp")
	if err := s3.Upload(ctx, bucket, thumbKey, bytes.NewReader(result.Data), int64(len(result.Data)), result.MimeType); err != nil {
		slog.Warn("async video thumbnail upload failed", "error", err)
		return nil
	}

	return []model.MediaVariant{{
		Name:      "thumbnail",
		Width:     result.Width,
		Height:    result.Height,
		FileSize:  int64(len(result.Data)),
		ObjectKey: thumbKey,
		URL:       s3.GetPublicURL(bucket, thumbKey),
		MimeType:  result.MimeType,
	}}
}
