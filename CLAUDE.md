# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

hubflora-media is a Go microservice for file storage, image processing, and variant generation. It's decoupled from the traveler-aggregator monolith and uses S3-compatible storage (MinIO), Redis (asynq) for async task queues, and PostgreSQL for metadata persistence. It also ships a TypeScript SDK (`@hubflora/media-client`).

## Build & Run

```bash
# Build (requires libvips installed: brew install vips on macOS)
go build -o hubflora-media ./cmd/server

# Run locally (needs .env — copy from .env.example)
cp .env.example .env
./hubflora-media

# Run with Docker Compose (includes MinIO + Redis)
docker compose up --build
```

CGO is required (`CGO_ENABLED=1`) because of the bimg (libvips) dependency for image processing.

### TypeScript SDK

```bash
cd sdk/js
npm install
npm run build    # uses tsup
```

## Architecture

### Go Service (`cmd/server/main.go` is the entrypoint)

```
cmd/server/         Single main.go — wires everything, creates chi router, starts HTTP + asynq worker
internal/
  config/           Environment-based config (no config files, all from env vars)
  handler/          HTTP handlers — one file per concern (upload, crop, delete, presign, download, variant, job, media_queries, showcase)
  middleware/       DualAuth (JWT via JWKS + API key), rate limiting, AuthContext propagation
  model/            Domain types: MediaFile, MediaVariant, request/response structs, DB record types
  processing/       Image processing via bimg/libvips — variants.go defines DefaultVariants, processor.go does resize/crop
  queue/            Asynq task definitions (redis.go) and worker handler (worker.go) for async variant generation
  repository/       PostgreSQL repositories (media.go, variant.go) using pgx — raw SQL, no ORM
  storage/          S3Client wrapping minio-go — upload, download, presign, delete, public URL generation
pkg/client/         Go SDK client for service-to-service calls
sdk/js/             TypeScript SDK (@hubflora/media-client) with React hooks support
web/                Static HTML (showcase page)
```

### Key Design Patterns

- **DualAuth middleware** (`internal/middleware/auth.go`): All `/api/v1/*` routes authenticate via JWT (Bearer token validated against Better Auth JWKS) or API key (`X-Media-API-Key` header). Auth context (UserID, OrgID, OrgSlug) propagated via `middleware.AuthContext` in request context.
- **Sync/async variant generation**: Uploads can generate image variants synchronously or queue them via asynq (Redis). The `queue.ProcessVariants()` function is shared between sync upload, async worker, and crop regeneration paths.
- **Organization-scoped data**: All DB queries are scoped by `organization_id`. Files are stored under `{orgSlug}/{timestamp}/{filename}` paths in S3.
- **No ORM**: Repositories use raw SQL with pgx. The `media_files` and `media_variants` tables store metadata; actual files live in S3.

### Database

Shares the PostgreSQL database with traveler-aggregator. Tables: `media_files`, `media_variants`. No migrations in this repo — schema is managed by the parent app (Better Auth + Drizzle).

### Image Variants

Five default variants (thumbnail, small, medium, large, original_webp) defined in `internal/processing/variants.go`. All convert to WebP format.

## Dependencies

- **chi** — HTTP router
- **bimg/libvips** — image processing (CGO required)
- **asynq** — Redis-based async task queue
- **pgx** — PostgreSQL driver (connection pool)
- **minio-go** — S3-compatible storage client
- **go-jose** — JWT/JWKS validation

## No Tests

There are currently no test files in this repository.
