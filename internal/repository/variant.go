package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/farahty/hubflora-media/internal/model"
)

// VariantRepository provides CRUD for the media_variants table.
type VariantRepository struct {
	pool *pgxpool.Pool
}

// NewVariantRepository creates a new repository.
func NewVariantRepository(pool *pgxpool.Pool) *VariantRepository {
	return &VariantRepository{pool: pool}
}

// CreateBatch inserts multiple variant rows.
func (r *VariantRepository) CreateBatch(ctx context.Context, variants []model.MediaVariantRecord) error {
	if len(variants) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, v := range variants {
		batch.Queue(`
			INSERT INTO media_variants (
				id, media_file_id, variant, width, height, file_size,
				object_key, url, mime_type, created_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (id) DO NOTHING`,
			v.ID, v.MediaFileID, v.Variant, v.Width, v.Height, v.FileSize,
			v.ObjectKey, v.URL, v.MimeType, v.CreatedAt,
		)
	}

	br := r.pool.SendBatch(ctx, batch)
	defer br.Close()

	for range variants {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

// GetByMediaFileID fetches all variants for a given media file.
func (r *VariantRepository) GetByMediaFileID(ctx context.Context, mediaFileID string) ([]model.MediaVariantRecord, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, media_file_id, variant, width, height, file_size,
			object_key, url, mime_type, created_at
		FROM media_variants
		WHERE media_file_id = $1
		ORDER BY created_at`, mediaFileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var variants []model.MediaVariantRecord
	for rows.Next() {
		var v model.MediaVariantRecord
		err := rows.Scan(
			&v.ID, &v.MediaFileID, &v.Variant, &v.Width, &v.Height, &v.FileSize,
			&v.ObjectKey, &v.URL, &v.MimeType, &v.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		variants = append(variants, v)
	}
	return variants, rows.Err()
}

// DeleteByMediaFileID removes all variants for a given media file.
func (r *VariantRepository) DeleteByMediaFileID(ctx context.Context, mediaFileID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM media_variants WHERE media_file_id = $1`, mediaFileID)
	return err
}

// ToRecords converts model.MediaVariant slice (from S3 processing) to DB records.
func ToRecords(mediaFileID string, variants []model.MediaVariant) []model.MediaVariantRecord {
	now := time.Now()
	records := make([]model.MediaVariantRecord, len(variants))
	for i, v := range variants {
		records[i] = model.MediaVariantRecord{
			ID:          uuid.New().String(),
			MediaFileID: mediaFileID,
			Variant:     v.Name,
			Width:       v.Width,
			Height:      v.Height,
			FileSize:    v.FileSize,
			ObjectKey:   v.ObjectKey,
			URL:         v.URL,
			MimeType:    v.MimeType,
			CreatedAt:   now,
		}
	}
	return records
}
