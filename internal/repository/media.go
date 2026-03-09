package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/farahty/hubflora-media/internal/model"
)

// MediaRepository provides CRUD for the media_files table.
type MediaRepository struct {
	pool *pgxpool.Pool
}

// NewMediaRepository creates a new repository.
func NewMediaRepository(pool *pgxpool.Pool) *MediaRepository {
	return &MediaRepository{pool: pool}
}

// Create inserts a new media_files row.
func (r *MediaRepository) Create(ctx context.Context, f *model.MediaFileRecord) error {
	metadataJSON, _ := json.Marshal(f.Metadata)

	_, err := r.pool.Exec(ctx, `
		INSERT INTO media_files (
			id, filename, original_filename, mime_type, file_size,
			width, height, duration, bucket_name, object_key, url,
			thumbnail_url, alt, caption, description, metadata,
			is_private, organization_id, uploaded_by, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9, $10, $11,
			$12, $13, $14, $15, $16,
			$17, $18, $19, $20, $21
		)`,
		f.ID, f.Filename, f.OriginalFilename, f.MimeType, f.FileSize,
		f.Width, f.Height, f.Duration, f.BucketName, f.ObjectKey, f.URL,
		f.ThumbnailURL, f.Alt, f.Caption, f.Description, metadataJSON,
		f.IsPrivate, f.OrganizationID, f.UploadedBy, f.CreatedAt, f.UpdatedAt,
	)
	return err
}

// GetByID fetches a single media file by ID, scoped to organization.
func (r *MediaRepository) GetByID(ctx context.Context, id string, orgID string) (*model.MediaFileRecord, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, filename, original_filename, mime_type, file_size,
			width, height, duration, bucket_name, object_key, url,
			thumbnail_url, alt, caption, description, metadata,
			is_private, organization_id, uploaded_by, created_at, updated_at
		FROM media_files
		WHERE id = $1 AND organization_id = $2`, id, orgID)
	return scanMediaFile(row)
}

// GetByIDs fetches multiple media files by IDs, scoped to organization.
func (r *MediaRepository) GetByIDs(ctx context.Context, ids []string, orgID string) ([]model.MediaFileRecord, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, filename, original_filename, mime_type, file_size,
			width, height, duration, bucket_name, object_key, url,
			thumbnail_url, alt, caption, description, metadata,
			is_private, organization_id, uploaded_by, created_at, updated_at
		FROM media_files
		WHERE id = ANY($1) AND organization_id = $2`, ids, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectMediaFiles(rows)
}

// ListOptions configures listing/pagination.
type ListOptions struct {
	Limit      int
	Offset     int
	Search     string
	MimePrefix string
	SortBy     string
	SortOrder  string
}

// List fetches paginated media files for an organization.
func (r *MediaRepository) List(ctx context.Context, orgID string, opts ListOptions) ([]model.MediaFileRecord, int, error) {
	if opts.Limit <= 0 || opts.Limit > 100 {
		opts.Limit = 50
	}

	// Build WHERE clause
	where := "WHERE organization_id = $1"
	args := []any{orgID}
	argIdx := 2

	if opts.Search != "" {
		where += fmt.Sprintf(` AND (filename ILIKE $%d OR original_filename ILIKE $%d OR alt ILIKE $%d OR caption ILIKE $%d)`,
			argIdx, argIdx, argIdx, argIdx)
		args = append(args, "%"+opts.Search+"%")
		argIdx++
	}

	if opts.MimePrefix != "" {
		where += fmt.Sprintf(` AND mime_type LIKE $%d`, argIdx)
		args = append(args, opts.MimePrefix+"%")
		argIdx++
	}

	// Count total
	var total int
	countQuery := "SELECT COUNT(*) FROM media_files " + where
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Sort
	sortCol := "created_at"
	switch opts.SortBy {
	case "file_size", "filename", "created_at":
		sortCol = opts.SortBy
	}
	sortDir := "DESC"
	if strings.EqualFold(opts.SortOrder, "asc") {
		sortDir = "ASC"
	}

	// Query
	query := fmt.Sprintf(`
		SELECT id, filename, original_filename, mime_type, file_size,
			width, height, duration, bucket_name, object_key, url,
			thumbnail_url, alt, caption, description, metadata,
			is_private, organization_id, uploaded_by, created_at, updated_at
		FROM media_files %s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d`, where, sortCol, sortDir, argIdx, argIdx+1)

	args = append(args, opts.Limit, opts.Offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	files, err := collectMediaFiles(rows)
	return files, total, err
}

// UpdateFields holds optional fields for partial updates.
type UpdateFields struct {
	Alt          *string
	Caption      *string
	Description  *string
	IsPrivate    *bool
	ThumbnailURL *string
	Width        *int
	Height       *int
	FileSize     *int64
	MimeType     *string
	Duration     *int
}

// Update partially updates a media file by ID, scoped to organization.
func (r *MediaRepository) Update(ctx context.Context, id string, orgID string, fields UpdateFields) error {
	sets := []string{}
	args := []any{}
	argIdx := 1

	addField := func(col string, val any) {
		sets = append(sets, fmt.Sprintf("%s = $%d", col, argIdx))
		args = append(args, val)
		argIdx++
	}

	if fields.Alt != nil {
		addField("alt", *fields.Alt)
	}
	if fields.Caption != nil {
		addField("caption", *fields.Caption)
	}
	if fields.Description != nil {
		addField("description", *fields.Description)
	}
	if fields.IsPrivate != nil {
		addField("is_private", *fields.IsPrivate)
	}
	if fields.ThumbnailURL != nil {
		addField("thumbnail_url", *fields.ThumbnailURL)
	}
	if fields.Width != nil {
		addField("width", *fields.Width)
	}
	if fields.Height != nil {
		addField("height", *fields.Height)
	}
	if fields.FileSize != nil {
		addField("file_size", *fields.FileSize)
	}
	if fields.MimeType != nil {
		addField("mime_type", *fields.MimeType)
	}
	if fields.Duration != nil {
		addField("duration", *fields.Duration)
	}

	if len(sets) == 0 {
		return nil
	}

	addField("updated_at", time.Now())

	query := fmt.Sprintf("UPDATE media_files SET %s WHERE id = $%d AND organization_id = $%d",
		strings.Join(sets, ", "), argIdx, argIdx+1)
	args = append(args, id, orgID)

	_, err := r.pool.Exec(ctx, query, args...)
	return err
}

// Delete removes a media file by ID, scoped to organization.
func (r *MediaRepository) Delete(ctx context.Context, id string, orgID string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM media_files WHERE id = $1 AND organization_id = $2`, id, orgID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// GetByObjectKey fetches a media file by object key, scoped to organization.
func (r *MediaRepository) GetByObjectKey(ctx context.Context, objectKey string, orgID string) (*model.MediaFileRecord, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, filename, original_filename, mime_type, file_size,
			width, height, duration, bucket_name, object_key, url,
			thumbnail_url, alt, caption, description, metadata,
			is_private, organization_id, uploaded_by, created_at, updated_at
		FROM media_files
		WHERE object_key = $1 AND organization_id = $2`, objectKey, orgID)
	return scanMediaFile(row)
}

// scanMediaFile scans a single row into a MediaFileRecord.
func scanMediaFile(row pgx.Row) (*model.MediaFileRecord, error) {
	var f model.MediaFileRecord
	var metadataJSON []byte

	err := row.Scan(
		&f.ID, &f.Filename, &f.OriginalFilename, &f.MimeType, &f.FileSize,
		&f.Width, &f.Height, &f.Duration, &f.BucketName, &f.ObjectKey, &f.URL,
		&f.ThumbnailURL, &f.Alt, &f.Caption, &f.Description, &metadataJSON,
		&f.IsPrivate, &f.OrganizationID, &f.UploadedBy, &f.CreatedAt, &f.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if metadataJSON != nil {
		json.Unmarshal(metadataJSON, &f.Metadata)
	}
	return &f, nil
}

// collectMediaFiles scans all rows from a query.
func collectMediaFiles(rows pgx.Rows) ([]model.MediaFileRecord, error) {
	var files []model.MediaFileRecord
	for rows.Next() {
		var f model.MediaFileRecord
		var metadataJSON []byte

		err := rows.Scan(
			&f.ID, &f.Filename, &f.OriginalFilename, &f.MimeType, &f.FileSize,
			&f.Width, &f.Height, &f.Duration, &f.BucketName, &f.ObjectKey, &f.URL,
			&f.ThumbnailURL, &f.Alt, &f.Caption, &f.Description, &metadataJSON,
			&f.IsPrivate, &f.OrganizationID, &f.UploadedBy, &f.CreatedAt, &f.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		if metadataJSON != nil {
			json.Unmarshal(metadataJSON, &f.Metadata)
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

// UpdateThumbnail sets the thumbnail URL without org scoping (for internal worker use).
func (r *MediaRepository) UpdateThumbnail(ctx context.Context, id string, thumbnailURL string) error {
	_, err := r.pool.Exec(ctx, `UPDATE media_files SET thumbnail_url = $1, updated_at = $2 WHERE id = $3`,
		thumbnailURL, time.Now(), id)
	return err
}
