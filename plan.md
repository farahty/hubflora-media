# Plan: Decouple Media Module into `hubflora-media` (Go Microservice)

## Current State Analysis

### What the Media Module Does Today (in `traveler-aggregator`)

The media module is a **Next.js/TypeScript monolith component** embedded in `lib/media/` with these responsibilities:

| Layer | Files | Description |
|-------|-------|-------------|
| **DB Schema** | `db/schema/media.ts` | Two tables: `media_files` and `media_variants` (PostgreSQL via Drizzle ORM) |
| **Storage** | `lib/minio.ts` | S3-compatible storage client (MinIO / DigitalOcean Spaces) |
| **Folder Structure** | `lib/media/storage/folder-structure.ts` | Path generation: `/{org-slug}/{filename}/{variant}.{ext}` |
| **Image Processing** | `lib/media/processing/image-processor.ts` | Sharp-based resize/crop/format conversion with 5 default variants (thumbnail, small, medium, large, original_webp) |
| **Server Actions** | `lib/media/actions/media.ts` | Upload, delete, update, list, get-by-ids (Next.js server actions) |
| **Enhanced Upload** | `lib/media/actions/enhanced-media-upload.ts` | Client-side upload with XHR progress tracking |
| **API Routes** | `app/api/media/[id]`, `enhanced-upload`, `variant` | REST endpoints for upload, fetch, variant URLs |
| **Background Workers** | `lib/media/workers/media-processing.worker.ts` | BullMQ workers for async variant generation, chunked uploads, bulk ops |
| **React Components** | `lib/media/components/` | Library browser, selector dialog, image editor/cropper, media field |
| **React Hooks** | `lib/media/hooks/` | `useMediaProcessing`, `useMediaEditor`, `useMediaVariant` |

### Cross-Module Dependencies (What References Media)

| Module | How It Uses Media |
|--------|-------------------|
| **Pages** (`db/schema/pages.ts`) | `featuredImageId` FK → `media_files.id` |
| **Products** (`db/schema/products.ts`) | `mediaId` text field on product variants |
| **Opportunity Attachments** (`db/schema/opportunity-attachments.ts`) | `mediaFileId` FK → `media_files.id` with cascade delete |
| **Activity Attachments** (`db/schema/activities.ts`) | `mediaFileId` text field |
| **Farahty Builder** (image, video, hero-banner, carousel widgets) | Uses `MediaSelector` component + `getMediaFilesByIds` |
| **Funnel Steps** (step outcomes) | Uses `MediaSelector` + `getMediaFilesByIds` |
| **Form Preview** | Uses `getMediaFilesByIds` |
| **Media Admin Page** | Full media showcase with library, upload, editing |
| **BullMQ Queue** | `MEDIA_PROCESSING` queue in `lib/queue/queues.ts` |

---

## Proposed Architecture: `hubflora-media` Go Microservice

### Guiding Principles
1. **Media service owns storage + processing** — all file I/O, image transformations, variant generation
2. **Traveler-aggregator becomes a client** — calls media service via HTTP API, stores only media IDs
3. **DB tables stay in traveler-aggregator for now** — media metadata lives alongside the data that references it; the Go service manages the *files* not the *records*
4. **Incremental migration** — phase 1 extracts file operations; phase 2 (future) can move DB ownership

---

### Phase 1: Go Media Service (File Operations)

#### 1.1 Project Structure

```
hubflora-media/
├── cmd/
│   └── server/
│       └── main.go              # Entry point
├── internal/
│   ├── config/
│   │   └── config.go            # Env-based configuration
│   ├── handler/
│   │   ├── upload.go            # POST /api/v1/media/upload
│   │   ├── download.go          # GET  /api/v1/media/:id/download
│   │   ├── variant.go           # GET  /api/v1/media/:id/variant/:name
│   │   ├── delete.go            # DELETE /api/v1/media/:id
│   │   ├── crop.go              # POST /api/v1/media/:id/crop
│   │   ├── health.go            # GET  /healthz
│   │   └── presign.go           # GET  /api/v1/media/:id/presign
│   ├── middleware/
│   │   ├── auth.go              # JWT/API-key validation (shared secret with traveler-aggregator)
│   │   ├── ratelimit.go         # Upload rate limiting
│   │   └── cors.go              # CORS configuration
│   ├── processing/
│   │   ├── processor.go         # Image resize/crop/format (using libvips via bimg or imaging)
│   │   ├── variants.go          # Variant definitions (thumbnail, small, medium, large, original_webp)
│   │   └── metadata.go          # EXIF extraction, dimensions, color analysis
│   ├── storage/
│   │   ├── s3.go                # S3-compatible client (MinIO / DO Spaces)
│   │   ├── folder.go            # Folder structure: /{org-slug}/{filename}/{variant}.{ext}
│   │   └── presign.go           # Pre-signed URL generation
│   ├── queue/
│   │   ├── worker.go            # Background variant generation worker
│   │   └── redis.go             # Redis/BullMQ-compatible queue consumer
│   └── model/
│       ├── media.go             # MediaFile, MediaVariant structs
│       └── request.go           # Upload/crop request DTOs
├── pkg/
│   └── client/
│       └── client.go            # Go SDK client (for other Go services)
├── Dockerfile
├── docker-compose.yml
├── go.mod
├── go.sum
└── README.md
```

#### 1.2 API Endpoints

| Method | Path | Description | Request | Response |
|--------|------|-------------|---------|----------|
| `POST` | `/api/v1/media/upload` | Upload file, optionally generate variants | `multipart/form-data` + `orgSlug`, `generateVariants`, `async` | `{ id, url, variants[] }` |
| `POST` | `/api/v1/media/upload/presigned` | Get a pre-signed upload URL | `{ orgSlug, filename, mimeType }` | `{ uploadUrl, objectKey }` |
| `GET` | `/api/v1/media/:id` | Get file metadata (fetched from DB or passed in) | — | `{ id, url, mimeType, width, height, variants[] }` |
| `GET` | `/api/v1/media/:id/variant/:name` | Redirect to variant URL | — | `302 redirect` or `{ url }` |
| `POST` | `/api/v1/media/:id/crop` | Crop image and save as new or replace | `{ x, y, width, height }` | `{ id, url }` |
| `POST` | `/api/v1/media/:id/variants` | Trigger variant regeneration | `{ variants: ["thumbnail","small",...] }` | `{ jobId }` |
| `DELETE` | `/api/v1/media/:id` | Delete file + all variants from storage | — | `{ success }` |
| `GET` | `/api/v1/media/:id/presign` | Get pre-signed download URL | `?expiry=3600` | `{ url, expiresAt }` |
| `GET` | `/healthz` | Health check | — | `200 OK` |

#### 1.3 Key Technology Choices

| Concern | Choice | Rationale |
|---------|--------|-----------|
| **HTTP Framework** | `net/http` + `chi` router | Lightweight, idiomatic Go |
| **Image Processing** | `h2non/bimg` (libvips bindings) | 4-8x faster than Sharp for resize, lower memory, C-level performance |
| **S3 Client** | `minio/minio-go` | Native Go, supports MinIO + DO Spaces + AWS S3 |
| **Queue** | Option A: Redis + `hibiken/asynq` | Go-native, Redis-backed, simpler than BullMQ compatibility. Or Option B: consume from the same BullMQ queues via raw Redis |
| **Config** | `kelseyhightower/envconfig` or `spf13/viper` | 12-factor env-based config |
| **Logging** | `log/slog` (stdlib) | Standard, structured, zero deps |
| **Auth** | Shared JWT secret or API key header | Traveler-aggregator signs requests; media service validates |

#### 1.4 Authentication Strategy

The media service is **internal** (not public-facing). Two options:

- **Option A: Shared API Key** — Traveler-aggregator sends `X-Media-API-Key` header; media service validates against env var. Simple, sufficient for internal service-to-service calls.
- **Option B: JWT passthrough** — Traveler-aggregator forwards the user's JWT; media service validates with the same secret. Provides user-level audit trail.

Recommendation: **Start with Option A**, add JWT passthrough later if needed.

#### 1.5 Database Ownership

**Phase 1: DB stays in traveler-aggregator.** The Go service:
- Receives `orgSlug` in upload requests (no need to query organization table)
- Returns `objectKey`, `url`, `width`, `height`, `fileSize`, `mimeType` — the caller stores in `media_files`
- Does NOT directly read/write `media_files` or `media_variants` tables

**Phase 2 (future): DB moves to media service.** The Go service:
- Owns `media_files` and `media_variants` tables
- Exposes full CRUD + query API
- Traveler-aggregator stores only `media_id` foreign keys

---

### Phase 2: Changes in `traveler-aggregator`

#### 2.1 New: Media Client SDK (`lib/media-client.ts`)

Create a thin HTTP client that replaces direct MinIO/Sharp calls:

```typescript
// lib/media-client.ts
class MediaServiceClient {
  constructor(private baseUrl: string, private apiKey: string) {}

  async upload(file: File | Buffer, opts: { orgSlug: string; generateVariants?: boolean; async?: boolean }): Promise<UploadResult>
  async delete(objectKey: string, bucketName: string): Promise<void>
  async getVariantUrl(objectKey: string, variant: string): Promise<string>
  async crop(objectKey: string, crop: CropParams): Promise<CropResult>
  async getPresignedUrl(objectKey: string, expiry?: number): Promise<string>
}
```

#### 2.2 Files to Modify

| File | Change |
|------|--------|
| `lib/media/actions/media.ts` | Replace `minioService.uploadFile()` → `mediaClient.upload()` |
| `lib/media/actions/media.ts` | Replace `minioService.deleteFile()` → `mediaClient.delete()` |
| `lib/media/actions/media.ts` | Replace `ImageProcessor.processImage()` → handled by media service |
| `lib/media/processing/image-processor.ts` | Remove (processing moves to Go service) |
| `lib/minio.ts` | Remove or keep only for non-media uses if any |
| `lib/media/storage/folder-structure.ts` | Remove (folder logic moves to Go service) |
| `lib/media/workers/media-processing.worker.ts` | Remove (workers move to Go service) |
| `app/api/media/enhanced-upload/route.ts` | Proxy to Go service or call `mediaClient.upload()` |
| `app/api/media/variant/route.ts` | Proxy to Go service |
| `lib/actions/image-crop.ts` | Use `mediaClient.crop()` |

#### 2.3 Files That Stay Unchanged

| File | Why |
|------|-----|
| `db/schema/media.ts` | DB ownership stays in traveler-aggregator (Phase 1) |
| All React components in `lib/media/components/` | They call server actions, which internally switch to the Go client |
| All hooks in `lib/media/hooks/` | Consumer-facing interface stays the same |
| `lib/media/types/types.ts` | Shared types stay |
| `lib/media/actions/enhanced-media-upload.ts` | Client-side upload helper stays (changes endpoint URL) |

---

### Phase 3: Deployment & Infrastructure

#### 3.1 Docker Setup

```yaml
# docker-compose addition
hubflora-media:
  build: ./hubflora-media
  ports:
    - "8090:8090"
  environment:
    - MINIO_ENDPOINT=minio:9000
    - MINIO_ACCESS_KEY=${MINIO_ACCESS_KEY}
    - MINIO_SECRET_KEY=${MINIO_SECRET_KEY}
    - MINIO_DEFAULT_BUCKET=media
    - MINIO_USE_SSL=false
    - REDIS_URL=redis://redis:6379
    - API_KEY=${MEDIA_SERVICE_API_KEY}
    - PORT=8090
  depends_on:
    - minio
    - redis
```

#### 3.2 Traveler-Aggregator Env Additions

```env
MEDIA_SERVICE_URL=http://hubflora-media:8090
MEDIA_SERVICE_API_KEY=<shared-secret>
```

---

## Migration Strategy

### Step-by-step Order

1. **Build the Go service** with upload, delete, variant generation, crop endpoints
2. **Add health check + integration tests** against MinIO
3. **Create `lib/media-client.ts`** in traveler-aggregator with a feature flag
4. **Dual-write phase**: Both old (direct MinIO) and new (Go service) paths active; feature flag toggles
5. **Test in staging** with the Go service handling all uploads
6. **Cut over**: Remove direct MinIO/Sharp dependencies from traveler-aggregator
7. **Remove dead code**: `lib/minio.ts`, `lib/media/processing/`, `lib/media/workers/`, `lib/media/storage/`

### Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| **Latency increase** from extra network hop | Go service runs in same Docker network; image processing in Go (bimg/libvips) is faster than Sharp |
| **Breaking existing uploads** | Feature flag + dual-write phase; rollback by toggling flag |
| **BullMQ compatibility** | Use `asynq` (Go-native Redis queue) for media jobs; traveler-aggregator enqueues via media service API instead of direct BullMQ |
| **Large file uploads** | Support streaming upload / pre-signed URL flow to avoid double-buffering |
| **CORS for direct browser uploads** | Add pre-signed upload endpoint; browser uploads directly to S3, then notifies media service |
| **Organization slug resolution** | Traveler-aggregator resolves slug before calling media service; passes `orgSlug` as parameter |

---

## Summary

| What | Where Today | Where After |
|------|-------------|-------------|
| File upload to S3 | `lib/minio.ts` (TS) | `hubflora-media` Go service |
| Image resize/crop | `sharp` in `lib/media/processing/` (TS) | `bimg/libvips` in Go service |
| Variant generation | BullMQ worker (TS) | `asynq` worker in Go service |
| Folder path generation | `lib/media/storage/` (TS) | Go service `internal/storage/folder.go` |
| DB records (`media_files`, `media_variants`) | Drizzle ORM in traveler-aggregator | **Stays** in traveler-aggregator (Phase 1) |
| React components | `lib/media/components/` | **Stays** in traveler-aggregator (unchanged) |
| API routes | Next.js `app/api/media/` | Proxy to Go service, or direct client calls |
