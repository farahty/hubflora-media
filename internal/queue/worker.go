package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"

	"github.com/farahty/hubflora-media/internal/model"
	"github.com/farahty/hubflora-media/internal/processing"
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
}

// NewVariantTask creates a new asynq task for variant generation.
// Only stores the S3 reference, not the file bytes.
func NewVariantTask(mediaID, bucket, folderPath, objectKey string) (*asynq.Task, error) {
	payload, err := json.Marshal(VariantPayload{
		MediaID:    mediaID,
		BucketName: bucket,
		FolderPath: folderPath,
		ObjectKey:  objectKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal variant payload: %w", err)
	}
	return asynq.NewTask(TypeVariantGenerate, payload), nil
}

// VariantHandler processes async variant generation tasks.
type VariantHandler struct {
	s3   *storage.S3Client
	proc *processing.Processor
}

// NewVariantHandler creates a new handler for variant tasks.
func NewVariantHandler(s3 *storage.S3Client, proc *processing.Processor) *VariantHandler {
	return &VariantHandler{s3: s3, proc: proc}
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

	variants := ProcessVariants(ctx, h.s3, h.proc, data, payload.BucketName, payload.FolderPath)

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
