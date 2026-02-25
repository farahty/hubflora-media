# Phase 2: hubflora-media Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add PostgreSQL persistence and JWT authentication to hubflora-media so it becomes a fully autonomous, public-facing media microservice.

**Architecture:** The Go service connects to the same PostgreSQL database as traveler-aggregator (schema managed by Drizzle ORM). A dual auth middleware validates either Better Auth JWTs (browser) or API keys (server-to-server). All handlers persist records to DB after S3 operations. New query endpoints expose media data via API.

**Tech Stack:** Go 1.23, chi/v5, pgx/v5 (PostgreSQL), go-jose/v4 (JWT/JWKS), bimg/libvips, asynq/Redis, minio-go/v7

---

### Task 1: Add Go Dependencies

**Files:**
- Modify: `go.mod`

**Step 1: Add pgx and go-jose dependencies**

Run:
```bash
cd /Users/nimer/Projects/hubflora-media
go get github.com/jackc/pgx/v5
go get github.com/go-jose/go-jose/v4
go mod tidy
```

Expected: `go.mod` now includes `github.com/jackc/pgx/v5` and `github.com/go-jose/go-jose/v4`.

**Step 2: Verify build still compiles**

Run: `go build ./...`
Expected: No errors.

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add pgx/v5 and go-jose/v4 dependencies for Phase 2"
```

---

### Task 2: Extend Config with Database and Auth URL

**Files:**
- Modify: `internal/config/config.go`
- Modify: `.env.example`

**Step 1: Add DatabaseURL and BetterAuthURL to Config struct**

In `internal/config/config.go`, add to the `Config` struct:

```go
// Database (shared with traveler-aggregator)
DatabaseURL string

// JWT Auth (JWKS endpoint: {BetterAuthURL}/api/auth/jwks)
BetterAuthURL string
```

In the `Load()` function, add after the `AllowedOrigins` line:

```go
DatabaseURL:   envStr("DATABASE_URL", ""),
BetterAuthURL: envStr("BETTER_AUTH_URL", ""),
```

Add validation at the end of `Load()`, before the return:

```go
if cfg.DatabaseURL == "" {
    return nil, fmt.Errorf("DATABASE_URL is required")
}

if cfg.BetterAuthURL == "" {
    return nil, fmt.Errorf("BETTER_AUTH_URL is required")
}
```

**Step 2: Update .env.example**

Append to `.env.example`:

```env

# Database (shared with traveler-aggregator)
DATABASE_URL=postgresql://postgres:postgres@localhost:5432/hubflora

# Better Auth (JWKS endpoint for JWT validation)
BETTER_AUTH_URL=http://auth.lvh.me:3000
```

**Step 3: Verify build**

Run: `go build ./...`
Expected: No errors.

**Step 4: Commit**

```bash
git add internal/config/config.go .env.example
git commit -m "feat: add DATABASE_URL and BETTER_AUTH_URL to config"
```

---

### Task 3: Create Auth Context and Dual Auth Middleware

**Files:**
- Create: `internal/middleware/context.go`
- Rewrite: `internal/middleware/auth.go`

**Step 1: Create auth context helpers**

Create `internal/middleware/context.go`:

```go
package middleware

import (
	"context"
	"net/http"
)

// AuthContext holds the authenticated identity extracted from JWT or API key.
type AuthContext struct {
	UserID         string
	OrganizationID string
	OrgSlug        string
	AuthMethod     string // "jwt" or "apikey"
}

type authContextKey struct{}

// WithAuthContext stores AuthContext in the request context.
func WithAuthContext(ctx context.Context, ac *AuthContext) context.Context {
	return context.WithValue(ctx, authContextKey{}, ac)
}

// GetAuthContext retrieves AuthContext from the request context.
// Returns nil if not present.
func GetAuthContext(ctx context.Context) *AuthContext {
	ac, _ := ctx.Value(authContextKey{}).(*AuthContext)
	return ac
}

// MustGetAuthContext retrieves AuthContext or panics.
// Use only in handlers behind the auth middleware.
func MustGetAuthContext(r *http.Request) *AuthContext {
	ac := GetAuthContext(r.Context())
	if ac == nil {
		panic("auth context missing — handler called without auth middleware")
	}
	return ac
}
```

**Step 2: Rewrite auth.go with dual auth (JWT + API key)**

Replace `internal/middleware/auth.go` with:

```go
package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
	josejwt "github.com/go-jose/go-jose/v4/jwt"
)

const apiKeyHeader = "X-Media-API-Key"

// JWKSCache caches the JWKS fetched from Better Auth.
type JWKSCache struct {
	mu        sync.RWMutex
	keys      *jose.JSONWebKeySet
	fetchedAt time.Time
	ttl       time.Duration
	jwksURL   string
	client    *http.Client
}

// NewJWKSCache creates a cache that fetches JWKS from the given URL.
func NewJWKSCache(jwksURL string, ttl time.Duration) *JWKSCache {
	return &JWKSCache{
		jwksURL: jwksURL,
		ttl:     ttl,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

// GetKeys returns cached keys or fetches fresh ones if expired.
func (c *JWKSCache) GetKeys(ctx context.Context) (*jose.JSONWebKeySet, error) {
	c.mu.RLock()
	if c.keys != nil && time.Since(c.fetchedAt) < c.ttl {
		keys := c.keys
		c.mu.RUnlock()
		return keys, nil
	}
	c.mu.RUnlock()
	return c.refresh(ctx)
}

// RefreshForKID re-fetches JWKS if the given kid is not found in the cache.
func (c *JWKSCache) RefreshForKID(ctx context.Context, kid string) (*jose.JSONWebKeySet, error) {
	c.mu.RLock()
	if c.keys != nil {
		if keys := c.keys.Key(kid); len(keys) > 0 {
			c.mu.RUnlock()
			return c.keys, nil
		}
	}
	c.mu.RUnlock()
	return c.refresh(ctx)
}

func (c *JWKSCache) refresh(ctx context.Context) (*jose.JSONWebKeySet, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, "GET", c.jwksURL, nil)
	if err != nil {
		return c.keys, fmt.Errorf("jwks request build: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		// Return stale cache if available
		if c.keys != nil {
			slog.Warn("jwks fetch failed, using stale cache", "error", err)
			return c.keys, nil
		}
		return nil, fmt.Errorf("jwks fetch failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		if c.keys != nil {
			return c.keys, nil
		}
		return nil, fmt.Errorf("jwks read body: %w", err)
	}

	var jwks jose.JSONWebKeySet
	if err := json.Unmarshal(body, &jwks); err != nil {
		if c.keys != nil {
			return c.keys, nil
		}
		return nil, fmt.Errorf("jwks unmarshal: %w", err)
	}

	c.keys = &jwks
	c.fetchedAt = time.Now()
	slog.Info("jwks refreshed", "keys", len(jwks.Keys))
	return c.keys, nil
}

// MediaClaims are the custom JWT claims from Better Auth's definePayload.
type MediaClaims struct {
	Sub     string `json:"sub"`
	OrgID   string `json:"orgId"`
	OrgSlug string `json:"orgSlug"`
}

// DualAuth validates either a Bearer JWT or an API key.
// JWT is tried first. If no Authorization header, falls back to API key.
func DualAuth(apiKey string, jwksCache *JWKSCache, betterAuthURL string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Path 1: Try JWT
			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				token := strings.TrimPrefix(authHeader, "Bearer ")
				ac, err := validateJWT(r.Context(), token, jwksCache, betterAuthURL)
				if err != nil {
					slog.Debug("jwt validation failed", "error", err)
					http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
					return
				}
				ctx := WithAuthContext(r.Context(), ac)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Path 2: Try API key
			key := r.Header.Get(apiKeyHeader)
			if key != "" {
				if key != apiKey {
					http.Error(w, `{"error":"invalid API key"}`, http.StatusUnauthorized)
					return
				}
				ac := &AuthContext{
					UserID:         r.Header.Get("X-Media-User-Id"),
					OrganizationID: r.Header.Get("X-Media-Org-Id"),
					OrgSlug:        r.Header.Get("X-Media-Org-Slug"),
					AuthMethod:     "apikey",
				}
				ctx := WithAuthContext(r.Context(), ac)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Path 3: Neither
			http.Error(w, `{"error":"missing authentication"}`, http.StatusUnauthorized)
		})
	}
}

// validateJWT parses and validates a Better Auth JWT using JWKS.
func validateJWT(ctx context.Context, rawToken string, jwksCache *JWKSCache, issuer string) (*AuthContext, error) {
	// Parse the JWT without verification first to get the kid
	parsed, err := josejwt.ParseSigned(rawToken, []jose.SignatureAlgorithm{
		jose.EdDSA, jose.RS256, jose.ES256, jose.ES384, jose.ES512,
	})
	if err != nil {
		return nil, fmt.Errorf("parse jwt: %w", err)
	}

	// Get kid from header
	if len(parsed.Headers) == 0 {
		return nil, fmt.Errorf("jwt has no headers")
	}
	kid := parsed.Headers[0].KeyID

	// Fetch JWKS (cached, refreshes for unknown kid)
	jwks, err := jwksCache.RefreshForKID(ctx, kid)
	if err != nil {
		return nil, fmt.Errorf("jwks fetch: %w", err)
	}

	keys := jwks.Key(kid)
	if len(keys) == 0 {
		return nil, fmt.Errorf("no key found for kid %q", kid)
	}

	// Verify signature and extract claims
	var standard josejwt.Claims
	var custom MediaClaims

	if err := parsed.Claims(keys[0].Key, &standard, &custom); err != nil {
		return nil, fmt.Errorf("verify claims: %w", err)
	}

	// Validate standard claims
	expected := josejwt.Expected{
		Issuer:   issuer,
		Audience: josejwt.Audience{issuer},
		Time:     time.Now(),
	}
	if err := standard.Validate(expected); err != nil {
		return nil, fmt.Errorf("claim validation: %w", err)
	}

	return &AuthContext{
		UserID:         custom.Sub,
		OrganizationID: custom.OrgID,
		OrgSlug:        custom.OrgSlug,
		AuthMethod:     "jwt",
	}, nil
}

// APIKeyAuth validates requests against a shared API key (legacy, kept for backward compat).
func APIKeyAuth(apiKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get(apiKeyHeader)
			if key == "" {
				http.Error(w, `{"error":"missing API key"}`, http.StatusUnauthorized)
				return
			}
			if key != apiKey {
				http.Error(w, `{"error":"invalid API key"}`, http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
```

**Step 3: Verify build**

Run: `go build ./...`
Expected: No errors.

**Step 4: Commit**

```bash
git add internal/middleware/context.go internal/middleware/auth.go
git commit -m "feat: add dual auth middleware (JWT via JWKS + API key)"
```

---

### Task 4: Add DB Record Structs to Model

**Files:**
- Modify: `internal/model/media.go`
- Modify: `internal/model/request.go`

**Step 1: Add DB record structs to media.go**

Append to `internal/model/media.go`:

```go
// MediaFileRecord represents a row in the media_files table.
type MediaFileRecord struct {
	ID               string         `json:"id"`
	Filename         string         `json:"filename"`
	OriginalFilename string         `json:"originalFilename"`
	MimeType         string         `json:"mimeType"`
	FileSize         int64          `json:"fileSize"`
	Width            *int           `json:"width,omitempty"`
	Height           *int           `json:"height,omitempty"`
	Duration         *int           `json:"duration,omitempty"`
	BucketName       string         `json:"bucketName"`
	ObjectKey        string         `json:"objectKey"`
	URL              string         `json:"url"`
	ThumbnailURL     *string        `json:"thumbnailUrl,omitempty"`
	Alt              *string        `json:"alt,omitempty"`
	Caption          *string        `json:"caption,omitempty"`
	Description      *string        `json:"description,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	IsPrivate        bool           `json:"isPrivate"`
	OrganizationID   *string        `json:"organizationId,omitempty"`
	UploadedBy       string         `json:"uploadedBy"`
	Variants         []MediaVariantRecord `json:"variants,omitempty"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
}

// MediaVariantRecord represents a row in the media_variants table.
type MediaVariantRecord struct {
	ID          string    `json:"id"`
	MediaFileID string    `json:"mediaFileId"`
	Variant     string    `json:"variant"`
	Width       int       `json:"width"`
	Height      int       `json:"height"`
	FileSize    int64     `json:"fileSize"`
	ObjectKey   string    `json:"objectKey"`
	URL         string    `json:"url"`
	MimeType    string    `json:"mimeType"`
	CreatedAt   time.Time `json:"createdAt"`
}

// ToMediaFile converts a DB record to the API response MediaFile.
func (r *MediaFileRecord) ToMediaFile() *MediaFile {
	mf := &MediaFile{
		ID:               r.ID,
		Filename:         r.Filename,
		OriginalFilename: r.OriginalFilename,
		MimeType:         r.MimeType,
		FileSize:         r.FileSize,
		Width:            r.Width,
		Height:           r.Height,
		BucketName:       r.BucketName,
		ObjectKey:        r.ObjectKey,
		URL:              r.URL,
		ThumbnailURL:     r.ThumbnailURL,
		Metadata:         r.Metadata,
		CreatedAt:        r.CreatedAt,
	}
	for _, vr := range r.Variants {
		mf.Variants = append(mf.Variants, MediaVariant{
			Name:     vr.Variant,
			Width:    vr.Width,
			Height:   vr.Height,
			FileSize: vr.FileSize,
			ObjectKey: vr.ObjectKey,
			URL:      vr.URL,
			MimeType: vr.MimeType,
		})
	}
	return mf
}
```

**Step 2: Add new request/response types to request.go**

Append to `internal/model/request.go`:

```go
// ListMediaRequest represents query params for GET /api/v1/media.
type ListMediaRequest struct {
	Limit      int    `json:"limit"`
	Offset     int    `json:"offset"`
	Search     string `json:"search"`
	MimePrefix string `json:"type"` // "image", "video", etc. → "image/"
	SortBy     string `json:"sort"`
	SortOrder  string `json:"order"`
}

// ListMediaResponse is the response for listing media files.
type ListMediaResponse struct {
	Items []MediaFileRecord `json:"items"`
	Total int               `json:"total"`
}

// BatchGetRequest is the body for POST /api/v1/media/batch.
type BatchGetRequest struct {
	IDs []string `json:"ids"`
}

// BatchGetResponse is the response for batch get.
type BatchGetResponse struct {
	Items []MediaFileRecord `json:"items"`
}

// UpdateMediaRequest is the body for PATCH /api/v1/media/:id.
type UpdateMediaRequest struct {
	Alt         *string `json:"alt"`
	Caption     *string `json:"caption"`
	Description *string `json:"description"`
	IsPrivate   *bool   `json:"isPrivate"`
}

// GetMediaResponse is the response for GET /api/v1/media/:id.
type GetMediaResponse struct {
	MediaFile MediaFileRecord `json:"mediaFile"`
}
```

**Step 3: Verify build**

Run: `go build ./...`
Expected: No errors.

**Step 4: Commit**

```bash
git add internal/model/media.go internal/model/request.go
git commit -m "feat: add DB record structs and new request/response types"
```

---

### Task 5: Create Repository Layer

**Files:**
- Create: `internal/repository/media.go`
- Create: `internal/repository/variant.go`

**Step 1: Create media repository**

Create `internal/repository/media.go`:

```go
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
	Alt         *string
	Caption     *string
	Description *string
	IsPrivate   *bool
	ThumbnailURL *string
	Width       *int
	Height      *int
	FileSize    *int64
	MimeType    *string
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
```

**Step 2: Create variant repository**

Create `internal/repository/variant.go`:

```go
package repository

import (
	"context"
	"time"

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
			ID:          generateID(),
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

func generateID() string {
	// Using google/uuid which is already a dependency
	return mustNewUUID()
}

func mustNewUUID() string {
	// Import will be resolved — google/uuid is already in go.mod
	id, _ := newUUID()
	return id
}
```

Wait — we need a clean UUID import. Let me fix that.

Replace the bottom of `internal/repository/variant.go` (the `generateID`, `mustNewUUID` functions) with:

```go
import "github.com/google/uuid"

// in the import block at top, add "github.com/google/uuid"
// then replace the helper functions with:

func generateID() string {
	return uuid.New().String()
}
```

The complete import block for `variant.go` should be:

```go
import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/farahty/hubflora-media/internal/model"
)
```

**Step 3: Verify build**

Run: `go build ./...`
Expected: No errors.

**Step 4: Commit**

```bash
git add internal/repository/media.go internal/repository/variant.go
git commit -m "feat: add media and variant repository layer (pgx/v5)"
```

---

### Task 6: Initialize DB Pool in main.go and Wire to Router

**Files:**
- Modify: `cmd/server/main.go`

**Step 1: Add DB pool initialization and update middleware**

In `cmd/server/main.go`, update the imports to include:

```go
"github.com/jackc/pgx/v5/pgxpool"
"github.com/farahty/hubflora-media/internal/repository"
```

After the `proc := processing.NewProcessor()` line (~line 48), add:

```go
// Initialize PostgreSQL connection pool
dbPool, err := pgxpool.New(ctx, cfg.DatabaseURL)
if err != nil {
    slog.Error("failed to connect to database", "error", err)
    os.Exit(1)
}
defer dbPool.Close()

// Verify DB connection
if err := dbPool.Ping(ctx); err != nil {
    slog.Error("failed to ping database", "error", err)
    os.Exit(1)
}
slog.Info("database connected")

// Initialize repositories
mediaRepo := repository.NewMediaRepository(dbPool)
variantRepo := repository.NewVariantRepository(dbPool)
```

After the Redis/asynq initialization block, add:

```go
// Initialize JWKS cache for JWT validation
jwksURL := cfg.BetterAuthURL + "/api/auth/jwks"
jwksCache := middleware.NewJWKSCache(jwksURL, 1*time.Hour)
```

Replace the middleware line in the API route group:

```go
// BEFORE:
// r.Use(middleware.APIKeyAuth(cfg.APIKey))

// AFTER:
r.Use(middleware.DualAuth(cfg.APIKey, jwksCache, cfg.BetterAuthURL))
```

Update handler function calls to pass `mediaRepo` and `variantRepo`. For now, just add the parameters — we'll update the handler signatures in subsequent tasks. The handler calls will be updated to:

```go
r.With(uploadLimiter.Middleware).Post("/upload", handler.Upload(cfg, s3Client, proc, asynqClient, mediaRepo, variantRepo))
r.Post("/crop", handler.Crop(cfg, s3Client, proc, asynqClient, mediaRepo, variantRepo))
r.Delete("/", handler.Delete(cfg, s3Client, mediaRepo, variantRepo))
```

Also add the new query endpoints inside the route group:

```go
// New Phase 2 query endpoints
r.Get("/{id}", handler.GetMedia(mediaRepo, variantRepo))
r.Get("/", handler.ListMedia(mediaRepo))  // Note: existing DELETE / stays
r.Post("/batch", handler.BatchGetMedia(mediaRepo, variantRepo))
r.Patch("/{id}", handler.UpdateMedia(mediaRepo))
r.Get("/{id}/variants", handler.GetMediaVariants(variantRepo))
```

**Note:** The route ordering matters with chi. The `GET /` for listing needs to be registered
such that it doesn't conflict with existing paths. We'll need to restructure slightly. The
list endpoint should be on the group root and the delete should move to `DELETE /{id}` for
consistency. This is a minor breaking change — document it.

Actually, to avoid breaking changes now, let's keep the existing delete endpoint as-is and add new endpoints. Add a `Route` sub-group for the new ID-based endpoints:

```go
// New Phase 2 endpoints
r.Get("/list", handler.ListMedia(mediaRepo))
r.Post("/batch", handler.BatchGetMedia(mediaRepo, variantRepo))
r.Route("/{id}", func(r chi.Router) {
    r.Get("/", handler.GetMedia(mediaRepo, variantRepo))
    r.Patch("/", handler.UpdateMedia(mediaRepo))
    r.Get("/variants", handler.GetMediaVariants(variantRepo))
})
```

Also pass `dbPool` to the health check for DB health:

```go
r.Get("/healthz", handler.Health(dbPool))
```

**Step 2: The upload, crop, delete, health handlers won't compile yet — that's expected. Create stub handlers so the build passes.**

Create `internal/handler/media_queries.go` with stubs:

```go
package handler

import (
	"net/http"

	"github.com/farahty/hubflora-media/internal/repository"
)

// GetMedia handles GET /api/v1/media/{id}
func GetMedia(mediaRepo *repository.MediaRepository, variantRepo *repository.VariantRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// TODO: implement in Task 9
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "not implemented"})
	}
}

// ListMedia handles GET /api/v1/media/list
func ListMedia(mediaRepo *repository.MediaRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "not implemented"})
	}
}

// BatchGetMedia handles POST /api/v1/media/batch
func BatchGetMedia(mediaRepo *repository.MediaRepository, variantRepo *repository.VariantRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "not implemented"})
	}
}

// UpdateMedia handles PATCH /api/v1/media/{id}
func UpdateMedia(mediaRepo *repository.MediaRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "not implemented"})
	}
}

// GetMediaVariants handles GET /api/v1/media/{id}/variants
func GetMediaVariants(variantRepo *repository.VariantRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "not implemented"})
	}
}
```

**Step 3: Update existing handler signatures to accept repos**

Update `handler.Upload` signature in `internal/handler/upload.go`:

```go
func Upload(cfg *config.Config, s3 *storage.S3Client, proc *processing.Processor, asynqClient *asynq.Client, mediaRepo *repository.MediaRepository, variantRepo *repository.VariantRepository) http.HandlerFunc {
```

Add import: `"github.com/farahty/hubflora-media/internal/repository"`

Update `handler.Delete` signature in `internal/handler/delete.go`:

```go
func Delete(cfg *config.Config, s3 *storage.S3Client, mediaRepo *repository.MediaRepository, variantRepo *repository.VariantRepository) http.HandlerFunc {
```

Update `handler.Crop` signature in `internal/handler/crop.go`:

```go
func Crop(cfg *config.Config, s3 *storage.S3Client, proc *processing.Processor, asynqClient *asynq.Client, mediaRepo *repository.MediaRepository, variantRepo *repository.VariantRepository) http.HandlerFunc {
```

Update `handler.Health` signature in `internal/handler/health.go`:

```go
import "github.com/jackc/pgx/v5/pgxpool"

func Health(dbPool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := "ok"
		if err := dbPool.Ping(r.Context()); err != nil {
			status = "db_unhealthy"
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": status})
	}
}
```

**Step 4: Verify build**

Run: `go build ./...`
Expected: No errors. (Repos are passed but not yet used in handler bodies — that's next.)

**Step 5: Commit**

```bash
git add cmd/server/main.go internal/handler/media_queries.go internal/handler/upload.go internal/handler/delete.go internal/handler/crop.go internal/handler/health.go
git commit -m "feat: wire DB pool, repos, and dual auth into router"
```

---

### Task 7: Update Upload Handler to Persist to DB

**Files:**
- Modify: `internal/handler/upload.go`

**Step 1: Update upload handler body to persist records**

The handler now:
1. Extracts auth context (orgSlug from JWT or form field)
2. After S3 upload, inserts into `media_files`
3. After variant generation, inserts into `media_variants`
4. Updates `thumbnail_url` on the media file record

Key changes inside the handler function body:

After the `orgSlug` check, add fallback to auth context:

```go
// Get auth context
authCtx := middleware.MustGetAuthContext(r)

// orgSlug from form takes priority, then from auth context (JWT)
if orgSlug == "" {
    orgSlug = authCtx.OrgSlug
}
if orgSlug == "" {
    writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "orgSlug is required"})
    return
}
```

After constructing the `mediaFile` struct, replace it with a `MediaFileRecord` and INSERT:

```go
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

// Set image dimensions
if processing.IsImageMimeType(mimeType) {
    meta, err := proc.GetMetadata(data)
    if err == nil {
        w := meta.Width
        h := meta.Height
        record.Width = &w
        record.Height = &h
        record.Metadata = map[string]any{
            "format":      meta.Format,
            "space":       meta.Space,
            "channels":    meta.Channels,
            "orientation": meta.Orientation,
        }
    }
}

// Persist to DB
if err := mediaRepo.Create(r.Context(), record); err != nil {
    slog.Error("failed to insert media_files record", "error", err)
    writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: "failed to persist media record"})
    return
}
```

After variant generation (sync path), persist variants:

```go
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
```

Convert the response to use `record.ToMediaFile()`:

```go
writeJSON(w, http.StatusOK, model.UploadResponse{
    Success:   true,
    MediaFile: record.ToMediaFile(),
})
```

**Step 2: Verify build**

Run: `go build ./...`
Expected: No errors.

**Step 3: Commit**

```bash
git add internal/handler/upload.go
git commit -m "feat: upload handler persists to media_files and media_variants"
```

---

### Task 8: Update Delete, Crop Handlers and Async Worker

**Files:**
- Modify: `internal/handler/delete.go`
- Modify: `internal/handler/crop.go`
- Modify: `internal/queue/worker.go`

**Step 1: Update delete handler to remove from DB first**

Rewrite the delete handler body to:
1. Accept either `id` or `objectKey`
2. Look up the record in DB (org-scoped)
3. Delete variants from DB
4. Delete media file from DB
5. Delete from S3

```go
func Delete(cfg *config.Config, s3 *storage.S3Client, mediaRepo *repository.MediaRepository, variantRepo *repository.VariantRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authCtx := middleware.MustGetAuthContext(r)

		var req struct {
			ID         string `json:"id"`
			ObjectKey  string `json:"objectKey"`
			BucketName string `json:"bucketName"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "invalid JSON body"})
			return
		}

		bucket := req.BucketName
		if bucket == "" {
			bucket = cfg.MinioDefaultBucket
		}

		var objectKey string

		if req.ID != "" {
			// Look up by ID
			record, err := mediaRepo.GetByID(r.Context(), req.ID, authCtx.OrganizationID)
			if err != nil {
				writeJSON(w, http.StatusNotFound, model.ErrorResponse{Error: "media file not found"})
				return
			}
			objectKey = record.ObjectKey

			// Delete from DB
			variantRepo.DeleteByMediaFileID(r.Context(), req.ID)
			mediaRepo.Delete(r.Context(), req.ID, authCtx.OrganizationID)
		} else if req.ObjectKey != "" {
			objectKey = req.ObjectKey

			// Try to find and delete from DB
			record, err := mediaRepo.GetByObjectKey(r.Context(), req.ObjectKey, authCtx.OrganizationID)
			if err == nil && record != nil {
				variantRepo.DeleteByMediaFileID(r.Context(), record.ID)
				mediaRepo.Delete(r.Context(), record.ID, authCtx.OrganizationID)
			}
		} else {
			writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "id or objectKey is required"})
			return
		}

		// Delete from S3
		folderPath := storage.ExtractFolderPath(objectKey)
		if folderPath != "" {
			s3.DeletePrefix(r.Context(), bucket, folderPath+"/")
		} else {
			s3.Delete(r.Context(), bucket, objectKey)
		}

		writeJSON(w, http.StatusOK, model.DeleteResponse{Success: true})
	}
}
```

Add imports for `middleware` package.

**Step 2: Update crop handler to update DB after crop**

In the crop handler, after the cropped image is uploaded to S3, update the DB record:

```go
authCtx := middleware.MustGetAuthContext(r)

// After S3 upload of cropped image, update DB record if it exists
record, dbErr := mediaRepo.GetByObjectKey(r.Context(), req.ObjectKey, authCtx.OrganizationID)
if dbErr == nil && record != nil {
    w_ := result.Width
    h_ := result.Height
    fileSize := int64(len(result.Data))
    mediaRepo.Update(r.Context(), record.ID, authCtx.OrganizationID, repository.UpdateFields{
        Width:    &w_,
        Height:   &h_,
        FileSize: &fileSize,
        MimeType: &result.MimeType,
    })

    // Use existing record ID for response
    mediaFile.ID = record.ID
}
```

And for variant regeneration after crop, persist variants to DB:

```go
variants := queue.ProcessVariants(r.Context(), s3, proc, result.Data, bucket, folderPath)
if len(variants) > 0 && record != nil {
    // Delete old variants and insert new ones
    variantRepo.DeleteByMediaFileID(r.Context(), record.ID)
    variantRecords := repository.ToRecords(record.ID, variants)
    variantRepo.CreateBatch(r.Context(), variantRecords)

    for _, v := range variants {
        if v.Name == "thumbnail" {
            thumbURL := v.URL
            mediaRepo.Update(r.Context(), record.ID, authCtx.OrganizationID,
                repository.UpdateFields{ThumbnailURL: &thumbURL})
            break
        }
    }
}
```

**Step 3: Update async worker to persist variants to DB**

The worker needs access to repos. Update `VariantHandler` in `internal/queue/worker.go`:

```go
type VariantHandler struct {
	s3          *storage.S3Client
	proc        *processing.Processor
	mediaRepo   *repository.MediaRepository
	variantRepo *repository.VariantRepository
}

func NewVariantHandler(s3 *storage.S3Client, proc *processing.Processor, mediaRepo *repository.MediaRepository, variantRepo *repository.VariantRepository) *VariantHandler {
	return &VariantHandler{s3: s3, proc: proc, mediaRepo: mediaRepo, variantRepo: variantRepo}
}
```

In `ProcessTask`, after `ProcessVariants()`, add:

```go
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
            // We don't have orgID in the payload — update without org scope
            // This is safe because the worker is trusted (internal)
            h.mediaRepo.UpdateThumbnail(ctx, payload.MediaID, thumbURL)
            break
        }
    }
}
```

Add an `UpdateThumbnail` method to `MediaRepository` that doesn't require orgID (for internal worker use):

```go
// UpdateThumbnail sets the thumbnail URL without org scoping (for internal worker use).
func (r *MediaRepository) UpdateThumbnail(ctx context.Context, id string, thumbnailURL string) error {
	_, err := r.pool.Exec(ctx, `UPDATE media_files SET thumbnail_url = $1, updated_at = $2 WHERE id = $3`,
		thumbnailURL, time.Now(), id)
	return err
}
```

Update `main.go` where `NewVariantHandler` is called to pass repos:

```go
mux.Handle(queue.TypeVariantGenerate, queue.NewVariantHandler(s3Client, proc, mediaRepo, variantRepo))
```

**Step 4: Verify build**

Run: `go build ./...`
Expected: No errors.

**Step 5: Commit**

```bash
git add internal/handler/delete.go internal/handler/crop.go internal/queue/worker.go internal/repository/media.go cmd/server/main.go
git commit -m "feat: delete, crop, and async worker now persist to DB"
```

---

### Task 9: Implement New Query Endpoints

**Files:**
- Modify: `internal/handler/media_queries.go`

**Step 1: Implement all five query handlers**

Replace the stubs in `internal/handler/media_queries.go` with full implementations:

```go
package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/farahty/hubflora-media/internal/middleware"
	"github.com/farahty/hubflora-media/internal/model"
	"github.com/farahty/hubflora-media/internal/repository"
)

// GetMedia handles GET /api/v1/media/{id}
func GetMedia(mediaRepo *repository.MediaRepository, variantRepo *repository.VariantRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authCtx := middleware.MustGetAuthContext(r)
		id := chi.URLParam(r, "id")
		if id == "" {
			writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "id is required"})
			return
		}

		record, err := mediaRepo.GetByID(r.Context(), id, authCtx.OrganizationID)
		if err != nil {
			if err == pgx.ErrNoRows {
				writeJSON(w, http.StatusNotFound, model.ErrorResponse{Error: "media file not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: "failed to fetch media file"})
			return
		}

		// Load variants
		variants, err := variantRepo.GetByMediaFileID(r.Context(), record.ID)
		if err == nil {
			record.Variants = variants
		}

		writeJSON(w, http.StatusOK, model.GetMediaResponse{MediaFile: *record})
	}
}

// ListMedia handles GET /api/v1/media/list
func ListMedia(mediaRepo *repository.MediaRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authCtx := middleware.MustGetAuthContext(r)

		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		search := r.URL.Query().Get("search")
		mimeType := r.URL.Query().Get("type")
		sortBy := r.URL.Query().Get("sort")
		order := r.URL.Query().Get("order")

		// Map "image" → "image/", "video" → "video/", etc.
		mimePrefix := ""
		if mimeType != "" {
			mimePrefix = mimeType + "/"
		}

		items, total, err := mediaRepo.List(r.Context(), authCtx.OrganizationID, repository.ListOptions{
			Limit:      limit,
			Offset:     offset,
			Search:     search,
			MimePrefix: mimePrefix,
			SortBy:     sortBy,
			SortOrder:  order,
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: "failed to list media"})
			return
		}

		if items == nil {
			items = []model.MediaFileRecord{}
		}

		writeJSON(w, http.StatusOK, model.ListMediaResponse{Items: items, Total: total})
	}
}

// BatchGetMedia handles POST /api/v1/media/batch
func BatchGetMedia(mediaRepo *repository.MediaRepository, variantRepo *repository.VariantRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authCtx := middleware.MustGetAuthContext(r)

		var req model.BatchGetRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "invalid JSON body"})
			return
		}

		if len(req.IDs) == 0 {
			writeJSON(w, http.StatusOK, model.BatchGetResponse{Items: []model.MediaFileRecord{}})
			return
		}
		if len(req.IDs) > 100 {
			writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "max 100 IDs per batch"})
			return
		}

		items, err := mediaRepo.GetByIDs(r.Context(), req.IDs, authCtx.OrganizationID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: "failed to batch fetch"})
			return
		}

		// Load variants for each item
		for i := range items {
			variants, err := variantRepo.GetByMediaFileID(r.Context(), items[i].ID)
			if err == nil {
				items[i].Variants = variants
			}
		}

		if items == nil {
			items = []model.MediaFileRecord{}
		}

		writeJSON(w, http.StatusOK, model.BatchGetResponse{Items: items})
	}
}

// UpdateMedia handles PATCH /api/v1/media/{id}
func UpdateMedia(mediaRepo *repository.MediaRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authCtx := middleware.MustGetAuthContext(r)
		id := chi.URLParam(r, "id")
		if id == "" {
			writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "id is required"})
			return
		}

		var req model.UpdateMediaRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "invalid JSON body"})
			return
		}

		fields := repository.UpdateFields{
			Alt:         req.Alt,
			Caption:     req.Caption,
			Description: req.Description,
			IsPrivate:   req.IsPrivate,
		}

		if err := mediaRepo.Update(r.Context(), id, authCtx.OrganizationID, fields); err != nil {
			writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: "failed to update"})
			return
		}

		// Return updated record
		record, err := mediaRepo.GetByID(r.Context(), id, authCtx.OrganizationID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, model.ErrorResponse{Error: "media file not found"})
			return
		}

		writeJSON(w, http.StatusOK, model.GetMediaResponse{MediaFile: *record})
	}
}

// GetMediaVariants handles GET /api/v1/media/{id}/variants
func GetMediaVariants(variantRepo *repository.VariantRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "id is required"})
			return
		}

		variants, err := variantRepo.GetByMediaFileID(r.Context(), id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: "failed to fetch variants"})
			return
		}

		if variants == nil {
			variants = []model.MediaVariantRecord{}
		}

		writeJSON(w, http.StatusOK, map[string]any{"variants": variants})
	}
}
```

**Step 2: Verify build**

Run: `go build ./...`
Expected: No errors.

**Step 3: Commit**

```bash
git add internal/handler/media_queries.go
git commit -m "feat: implement get, list, batch, update, and variants query endpoints"
```

---

### Task 10: Update CORS and .env for Public-Facing Access

**Files:**
- Modify: `cmd/server/main.go` (CORS section)
- Modify: `.env.example`

**Step 1: Update CORS configuration**

In `cmd/server/main.go`, update the CORS handler:

```go
r.Use(cors.Handler(cors.Options{
    AllowedOrigins:   cfg.AllowedOrigins,
    AllowedMethods:   []string{"GET", "POST", "PATCH", "PUT", "DELETE", "OPTIONS"},
    AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Media-API-Key", "X-Media-User-Id", "X-Media-Org-Id", "X-Media-Org-Slug"},
    ExposedHeaders:   []string{"Link"},
    AllowCredentials: false,
    MaxAge:           300,
}))
```

Note: `AllowCredentials` changed to `false` — we use Bearer tokens, not cookies.
`PATCH` added to allowed methods. New `X-Media-*` headers added.

**Step 2: Update .env.example CORS value**

```env
ALLOWED_CORS_ORIGINS=https://*.hubflora.com,http://*.lvh.me:3000
```

**Step 3: Verify build**

Run: `go build ./...`
Expected: No errors.

**Step 4: Commit**

```bash
git add cmd/server/main.go .env.example
git commit -m "feat: update CORS for public-facing access with Bearer auth"
```

---

### Task 11: Update SDK — Dual Auth Support

**Files:**
- Modify: `sdk/js/src/types.ts`
- Modify: `sdk/js/src/client.ts`

**Step 1: Update types.ts**

Add/modify in `sdk/js/src/types.ts`:

In `HubfloraMediaConfig`, make `apiKey` optional and add `tokenProvider`:

```typescript
export interface HubfloraMediaConfig {
  baseUrl: string;
  /** API key for server-to-server auth (optional if using tokenProvider) */
  apiKey?: string;
  /** Token provider for browser auth — called before each request */
  tokenProvider?: () => Promise<string> | string;
  /** Custom fetch implementation */
  fetch?: typeof globalThis.fetch;
}
```

Make `orgSlug` optional in `UploadOptions`:

```typescript
export interface UploadOptions {
  file: File | Blob;
  orgSlug?: string;  // Optional when using JWT (org from token)
  // ... rest stays the same
}
```

Add new types for Phase 2 endpoints:

```typescript
export interface ListMediaOptions {
  limit?: number;
  offset?: number;
  search?: string;
  type?: "image" | "video" | "audio" | "document";
  sort?: "created_at" | "file_size" | "filename";
  order?: "asc" | "desc";
}

export interface ListMediaResponse {
  items: MediaFile[];
  total: number;
}

export interface BatchGetResponse {
  items: MediaFile[];
}

export interface UpdateMediaFields {
  alt?: string;
  caption?: string;
  description?: string;
  isPrivate?: boolean;
}

export interface GetMediaResponse {
  mediaFile: MediaFile;
}
```

Update `MediaFile` to include all DB fields:

```typescript
export interface MediaFile {
  id: string;
  filename: string;
  originalFilename: string;
  mimeType: string;
  fileSize: number;
  width?: number;
  height?: number;
  duration?: number;
  bucketName: string;
  objectKey: string;
  url: string;
  thumbnailUrl?: string;
  alt?: string;
  caption?: string;
  description?: string;
  metadata?: Record<string, unknown>;
  isPrivate?: boolean;
  organizationId?: string;
  uploadedBy?: string;
  variants?: MediaVariant[];
  createdAt: string;
  updatedAt?: string;
}
```

**Step 2: Update client.ts for dual auth**

Change the constructor and `headers()` method in `sdk/js/src/client.ts`:

```typescript
export class HubfloraMedia {
  private baseUrl: string;
  private apiKey?: string;
  private tokenProvider?: () => Promise<string> | string;
  private _fetch: typeof globalThis.fetch;

  constructor(config: HubfloraMediaConfig) {
    this.baseUrl = config.baseUrl.replace(/\/+$/, "");
    this.apiKey = config.apiKey;
    this.tokenProvider = config.tokenProvider;
    this._fetch = config.fetch ?? globalThis.fetch.bind(globalThis);

    if (!this.apiKey && !this.tokenProvider) {
      throw new Error("HubfloraMedia: either apiKey or tokenProvider is required");
    }
  }

  private async authHeaders(): Promise<Record<string, string>> {
    if (this.tokenProvider) {
      const token = await this.tokenProvider();
      return { Authorization: `Bearer ${token}` };
    }
    if (this.apiKey) {
      return { "X-Media-API-Key": this.apiKey };
    }
    throw new Error("HubfloraMedia: no auth configured");
  }
```

Update the `headers()` helper and `request()` method to be async:

```typescript
  private async request<T>(
    method: string,
    path: string,
    opts?: { headers?: Record<string, string>; body?: BodyInit },
  ): Promise<T> {
    const auth = await this.authHeaders();
    const res = await this._fetch(`${this.baseUrl}${path}`, {
      method,
      headers: { ...auth, ...opts?.headers },
      body: opts?.body,
    });
    // ... rest stays the same
  }
```

Update `uploadWithProgress` to use async auth:

```typescript
  async uploadWithProgress(opts: UploadWithProgressOptions): Promise<UploadResponse> {
    const auth = await this.authHeaders();
    return new Promise((resolve, reject) => {
      const xhr = new XMLHttpRequest();
      // ...
      for (const [key, value] of Object.entries(auth)) {
        xhr.setRequestHeader(key, value);
      }
      // ... rest stays the same but remove the old x-media-api-key line
    });
  }
```

Add new methods:

```typescript
  /** Get a single media file by ID */
  async get(id: string): Promise<GetMediaResponse> {
    return this.request<GetMediaResponse>("GET", `/api/v1/media/${id}`);
  }

  /** List media files for the authenticated org */
  async list(opts?: ListMediaOptions): Promise<ListMediaResponse> {
    const query = this.qs({
      limit: opts?.limit,
      offset: opts?.offset,
      search: opts?.search,
      type: opts?.type,
      sort: opts?.sort,
      order: opts?.order,
    });
    return this.request<ListMediaResponse>("GET", `/api/v1/media/list${query}`);
  }

  /** Get multiple media files by IDs */
  async batchGet(ids: string[]): Promise<BatchGetResponse> {
    return this.request<BatchGetResponse>("POST", "/api/v1/media/batch", {
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ ids }),
    });
  }

  /** Update media metadata */
  async update(id: string, fields: UpdateMediaFields): Promise<GetMediaResponse> {
    return this.request<GetMediaResponse>("PATCH", `/api/v1/media/${id}`, {
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(fields),
    });
  }
```

**Step 3: Build SDK**

Run:
```bash
cd /Users/nimer/Projects/hubflora-media/sdk/js && npm run build
```
Expected: Build succeeds.

**Step 4: Commit**

```bash
cd /Users/nimer/Projects/hubflora-media
git add sdk/js/src/types.ts sdk/js/src/client.ts sdk/js/dist/
git commit -m "feat: SDK dual auth (tokenProvider + apiKey) and new query methods"
```

---

### Task 12: Update SDK React Provider with Session Sync

**Files:**
- Modify: `sdk/js/src/react.ts`

**Step 1: Add session-synced provider**

Add new provider component and update exports in `sdk/js/src/react.ts`.
Keep the existing simple `HubfloraMediaProvider` (context provider) for backward
compatibility, and add a new `HubfloraMediaSessionProvider`:

```typescript
import {
  createContext,
  useContext,
  useState,
  useCallback,
  useRef,
  useMemo,
  useEffect,
  type ReactNode,
} from "react";

// ... existing code stays ...

// ── Session-Synced Provider ──

export interface HubfloraMediaSessionProviderProps {
  children: ReactNode;
  /** Media service base URL */
  baseUrl: string;
  /** Function that syncs the org session and returns a fresh JWT access token */
  getToken: () => Promise<string>;
  /** Current organization ID — triggers re-sync when it changes */
  organizationId: string;
  /** Fallback UI while initializing (default: null) */
  fallback?: ReactNode;
}

/**
 * Provider that syncs the org session before any media operation.
 * Ensures the JWT always has the correct org context.
 *
 * ```tsx
 * <HubfloraMediaSessionProvider
 *   baseUrl="https://media.hubflora.com"
 *   organizationId={org.id}
 *   getToken={async () => {
 *     await authClient.organization.setActive({ organizationId: org.id });
 *     const res = await authClient.$fetch("/token");
 *     return res.data.token;
 *   }}
 * >
 *   {children}
 * </HubfloraMediaSessionProvider>
 * ```
 */
export function HubfloraMediaSessionProvider({
  children,
  baseUrl,
  getToken,
  organizationId,
  fallback = null,
}: HubfloraMediaSessionProviderProps) {
  const [ready, setReady] = useState(false);
  const tokenRef = useRef<string | null>(null);
  const refreshTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const client = useMemo(
    () =>
      new HubfloraMedia({
        baseUrl,
        tokenProvider: async () => {
          if (tokenRef.current) return tokenRef.current;
          const token = await getToken();
          tokenRef.current = token;
          return token;
        },
      }),
    [baseUrl],
  );

  useEffect(() => {
    let cancelled = false;

    async function init() {
      try {
        const token = await getToken();
        if (cancelled) return;
        tokenRef.current = token;
        setReady(true);

        // Refresh token every 12 minutes (tokens expire in 15m)
        refreshTimerRef.current = setInterval(async () => {
          try {
            const freshToken = await getToken();
            tokenRef.current = freshToken;
          } catch {
            // Silent refresh failure — next request will retry
          }
        }, 12 * 60 * 1000);
      } catch (err) {
        console.error("HubfloraMediaSessionProvider: failed to get token", err);
      }
    }

    setReady(false);
    tokenRef.current = null;
    init();

    return () => {
      cancelled = true;
      if (refreshTimerRef.current) {
        clearInterval(refreshTimerRef.current);
      }
    };
  }, [organizationId, getToken]);

  if (!ready) return fallback as any;

  return <HubfloraMediaProvider value={client}>{children}</HubfloraMediaProvider>;
}
```

**Step 2: Update sdk/js/src/index.ts exports**

Ensure the new provider is exported:

```typescript
export { HubfloraMediaSessionProvider } from "./react.js";
export type { HubfloraMediaSessionProviderProps } from "./react.js";
```

**Step 3: Build SDK**

Run:
```bash
cd /Users/nimer/Projects/hubflora-media/sdk/js && npm run build
```
Expected: Build succeeds.

**Step 4: Commit**

```bash
cd /Users/nimer/Projects/hubflora-media
git add sdk/js/src/react.ts sdk/js/src/index.ts sdk/js/dist/
git commit -m "feat: add HubfloraMediaSessionProvider with org sync and token refresh"
```

---

### Task 13: Update docker-compose and Dockerfile

**Files:**
- Modify: `docker-compose.yml`

**Step 1: Add DATABASE_URL and BETTER_AUTH_URL to docker-compose**

In `docker-compose.yml`, add the new env vars to the `hubflora-media` service:

```yaml
environment:
  # ... existing vars ...
  - DATABASE_URL=${DATABASE_URL}
  - BETTER_AUTH_URL=${BETTER_AUTH_URL}
```

Add a `depends_on` for the database if it's in the same compose file, or document
that the shared PostgreSQL must be reachable.

**Step 2: Commit**

```bash
git add docker-compose.yml
git commit -m "feat: add DATABASE_URL and BETTER_AUTH_URL to docker-compose"
```

---

### Task 14: Final Build Verification and Version Bump

**Step 1: Full Go build**

Run: `go build ./...`
Expected: No errors.

**Step 2: Full SDK build**

Run:
```bash
cd /Users/nimer/Projects/hubflora-media/sdk/js && npm run build
```
Expected: No errors.

**Step 3: Bump SDK version**

In `sdk/js/package.json`, bump version from `0.1.0` to `0.2.0`.

**Step 4: Commit**

```bash
cd /Users/nimer/Projects/hubflora-media
git add sdk/js/package.json
git commit -m "chore: bump SDK version to 0.2.0 for Phase 2"
```

---

## Summary of All Tasks

| # | Task | Scope |
|---|------|-------|
| 1 | Add Go dependencies (pgx, go-jose) | go.mod |
| 2 | Extend config (DATABASE_URL, BETTER_AUTH_URL) | config, .env |
| 3 | Dual auth middleware (JWT + API key) | middleware |
| 4 | DB record structs and new request types | model |
| 5 | Repository layer (media + variant CRUD) | repository |
| 6 | Wire DB pool, repos, and auth into router | main.go, stubs |
| 7 | Upload handler persists to DB | upload handler |
| 8 | Delete, crop, async worker persist to DB | delete, crop, worker |
| 9 | New query endpoints (get, list, batch, update) | media_queries |
| 10 | CORS for public-facing access | main.go, .env |
| 11 | SDK dual auth + new query methods | SDK client, types |
| 12 | SDK React provider with session sync | SDK react |
| 13 | Docker-compose env vars | docker-compose |
| 14 | Final build verification + version bump | all |
