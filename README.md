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

All `/api/v1/*` routes require `X-Media-API-Key` header matching the `API_KEY` env var.

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
| `API_KEY` | — | Shared API key (required) |
| `ALLOWED_CORS_ORIGINS` | `*` | Comma-separated CORS origins |
| `MAX_UPLOAD_SIZE` | `52428800` | Max upload size in bytes (50MB) |

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
