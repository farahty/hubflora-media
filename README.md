# hubflora-media

Go microservice for file storage, image processing, and variant generation. Decoupled from the traveler-aggregator monolith.

## Quick Start

```bash
# Copy env and edit
cp .env.example .env

# Run with Docker Compose
docker compose up --build
```

The service starts on `http://localhost:8090`.

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/media/upload` | Upload file (multipart) with optional variant generation |
| `POST` | `/api/v1/media/upload/presigned` | Get pre-signed upload URL |
| `POST` | `/api/v1/media/crop` | Crop an image |
| `DELETE` | `/api/v1/media` | Delete file + variants |
| `GET` | `/api/v1/media/presign?objectKey=...` | Get pre-signed download URL |
| `GET` | `/api/v1/media/download/{bucket}/*` | Stream file download |
| `GET` | `/api/v1/media/variant/{bucket}/{name}/*` | Redirect to variant URL |
| `GET` | `/healthz` | Health check |

## Authentication

All `/api/v1/*` routes require `X-Media-API-Key` header matching the `MEDIA_SERVICE_API_KEY` env var.

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8090` | HTTP server port |
| `MINIO_ENDPOINT` | `localhost` | S3-compatible storage host |
| `MINIO_PORT` | `9000` | Storage port |
| `MINIO_USE_SSL` | `false` | Use HTTPS for storage |
| `MINIO_ACCESS_KEY` | — | Storage access key (required) |
| `MINIO_SECRET_KEY` | — | Storage secret key (required) |
| `MINIO_DEFAULT_BUCKET` | `media` | Default bucket name |
| `MINIO_CDN_DOMAIN` | — | CDN domain for public URLs |
| `MINIO_USE_CDN` | `false` | Use CDN for public URLs |
| `REDIS_URL` | `redis://localhost:6379` | Redis for async task queue |
| `MEDIA_SERVICE_API_KEY` | — | Shared API key (required) |
| `ALLOWED_CORS_ORIGINS` | `*` | Comma-separated CORS origins |
| `MAX_UPLOAD_SIZE` | `52428800` | Max upload size in bytes (50MB) |

## API Examples

### Health Check

```bash
curl http://localhost:8090/healthz
```

Response:
```json
{ "status": "ok" }
```

### Upload File

```bash
curl -X POST http://localhost:8090/api/v1/media/upload \
  -H "X-Media-API-Key: your-api-key" \
  -F "file=@photo.jpg" \
  -F "orgSlug=my-org" \
  -F "generateVariants=true" \
  -F "async=false" \
  -F "alt=A sunset photo" \
  -F "caption=Sunset" \
  -F "description=Beautiful sunset over the ocean"
```

Response (`200` sync / `202` async):
```json
{
  "success": true,
  "mediaFile": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "filename": "20240101-120000",
    "originalFilename": "photo.jpg",
    "mimeType": "image/jpeg",
    "fileSize": 204800,
    "width": 1920,
    "height": 1080,
    "bucketName": "media",
    "objectKey": "my-org/20240101-120000/photo.jpg",
    "url": "https://cdn.example.com/my-org/20240101-120000/photo.jpg",
    "thumbnailUrl": "https://cdn.example.com/my-org/20240101-120000/thumbnail.webp",
    "metadata": {
      "format": "jpeg",
      "space": "srgb",
      "channels": 3,
      "orientation": 1
    },
    "variants": [],
    "createdAt": "2024-01-01T12:00:00Z"
  },
  "jobId": "job-uuid-here"
}
```

### Presigned Upload

```bash
curl -X POST http://localhost:8090/api/v1/media/upload/presigned \
  -H "X-Media-API-Key: your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "orgSlug": "my-org",
    "filename": "photo.jpg",
    "mimeType": "image/jpeg"
  }'
```

Response:
```json
{
  "uploadUrl": "https://s3.example.com/media/my-org/20240101-120000/photo.jpg?X-Amz-...",
  "objectKey": "my-org/20240101-120000/photo.jpg",
  "bucketName": "media"
}
```

### Crop Image

```bash
curl -X POST http://localhost:8090/api/v1/media/crop \
  -H "X-Media-API-Key: your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "objectKey": "my-org/20240101-120000/photo.jpg",
    "x": 100,
    "y": 50,
    "width": 800,
    "height": 600,
    "rotate": 0,
    "scale": 1.0,
    "quality": 90,
    "format": "webp",
    "regenerateVariants": true,
    "async": false
  }'
```

Response (`200` sync / `202` async):
```json
{
  "success": true,
  "mediaFile": { "..." : "..." },
  "jobId": "job-uuid-here"
}
```

### Regenerate Variants

```bash
curl -X POST http://localhost:8090/api/v1/media/variants \
  -H "X-Media-API-Key: your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "objectKey": "my-org/20240101-120000/photo.jpg"
  }'
```

Response (`202`):
```json
{
  "jobId": "550e8400-e29b-41d4-a716-446655440000"
}
```

### Get Variants Info

```bash
curl "http://localhost:8090/api/v1/media/variants/info?objectKey=my-org/20240101-120000/photo.jpg" \
  -H "X-Media-API-Key: your-api-key"
```

Response:
```json
{
  "objectKey": "my-org/20240101-120000/photo.jpg",
  "variants": [
    {
      "objectKey": "my-org/20240101-120000/thumbnail.webp",
      "url": "https://cdn.example.com/my-org/20240101-120000/thumbnail.webp",
      "fileSize": 5120,
      "mimeType": "image/webp"
    }
  ]
}
```

### Delete Media

```bash
curl -X DELETE http://localhost:8090/api/v1/media/ \
  -H "X-Media-API-Key: your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "objectKey": "my-org/20240101-120000/photo.jpg"
  }'
```

Response:
```json
{ "success": true }
```

### Presign Download URL

```bash
curl "http://localhost:8090/api/v1/media/presign?objectKey=my-org/20240101-120000/photo.jpg&expiry=3600" \
  -H "X-Media-API-Key: your-api-key"
```

Response:
```json
{
  "url": "https://s3.example.com/media/my-org/20240101-120000/photo.jpg?X-Amz-...",
  "expiresAt": "2024-01-01T13:00:00Z"
}
```

### Download File

```bash
curl http://localhost:8090/api/v1/media/download/media/my-org/20240101-120000/photo.jpg \
  -H "X-Media-API-Key: your-api-key" \
  -o photo.jpg
```

Returns the binary file stream with `Content-Type` and `Cache-Control: public, max-age=31536000, immutable` headers.

### Variant Redirect

```bash
curl -L http://localhost:8090/api/v1/media/variant/media/thumbnail/my-org/20240101-120000/ \
  -H "X-Media-API-Key: your-api-key"
```

Returns a `302` redirect to the public variant URL. Valid variant names: `thumbnail`, `small`, `medium`, `large`, `original_webp`.

### Job Status

```bash
curl http://localhost:8090/api/v1/media/job/550e8400-e29b-41d4-a716-446655440000 \
  -H "X-Media-API-Key: your-api-key"
```

Response:
```json
{
  "jobId": "550e8400-e29b-41d4-a716-446655440000",
  "state": "completed",
  "progress": 100,
  "processedOn": 1704106800000,
  "finishedOn": 1704106801000,
  "failedReason": ""
}
```

## Image Variants

Uploaded images can auto-generate these variants:

| Name | Max Size | Quality | Format | Fit |
|------|----------|---------|--------|-----|
| `thumbnail` | 400×400 | 80 | WebP | Cover (crop) |
| `small` | 600×600 | 85 | WebP | Inside |
| `medium` | 1024×1024 | 85 | WebP | Inside |
| `large` | 1440×1440 | 90 | WebP | Inside |
| `original_webp` | 2048×2048 | 95 | WebP | Inside |

## Development

```bash
# Build locally (requires libvips)
go build -o hubflora-media ./cmd/server

# Run
./hubflora-media
```
