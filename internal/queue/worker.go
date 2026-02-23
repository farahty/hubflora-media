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
type VariantPayload struct {
	MediaID    string `json:"mediaId"`
	BucketName string `json:"bucketName"`
	FolderPath string `json:"folderPath"`
	FileData   []byte `json:"fileData"`
}

// NewVariantTask creates a new asynq task for variant generation.
func NewVariantTask(mediaID, bucket, folderPath string, data []byte) (*asynq.Task, error) {
	payload, err := json.Marshal(VariantPayload{
		MediaID:    mediaID,
		BucketName: bucket,
		FolderPath: folderPath,
		FileData:   data,
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

	var variants []model.MediaVariant

	for _, variantDef := range processing.DefaultVariants {
		result, err := h.proc.ProcessImage(payload.FileData, variantDef)
		if err != nil {
			slog.Warn("variant processing failed", "variant", variantDef.Name, "error", err)
			continue
		}

		variantKey := storage.GenerateVariantKey(payload.FolderPath, variantDef.Name, result.Format.String())

		if err := h.s3.Upload(ctx, payload.BucketName, variantKey, bytes.NewReader(result.Data), int64(len(result.Data)), result.MimeType); err != nil {
			slog.Warn("variant upload failed", "variant", variantDef.Name, "error", err)
			continue
		}

		variants = append(variants, model.MediaVariant{
			Name:      variantDef.Name,
			Width:     result.Width,
			Height:    result.Height,
			FileSize:  int64(len(result.Data)),
			ObjectKey: variantKey,
			URL:       h.s3.GetPublicURL(payload.BucketName, variantKey),
			MimeType:  result.MimeType,
		})

		slog.Info("variant generated", "variant", variantDef.Name, "width", result.Width, "height", result.Height)
	}

	slog.Info("variant generation complete", "mediaId", payload.MediaID, "count", len(variants))
	return nil
}
