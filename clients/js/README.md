# @hubflora/media-client

TypeScript/JavaScript client for the [hubflora-media](../../README.md) service.

## Installation

```bash
npm install @hubflora/media-client
```

Or link locally during development:

```bash
cd clients/js && npm install && npm run build
npm link
# In your app:
npm link @hubflora/media-client
```

## Usage

```typescript
import { HubfloraMediaClient } from "@hubflora/media-client";

const media = new HubfloraMediaClient({
  baseUrl: process.env.MEDIA_SERVICE_URL!,  // e.g. http://localhost:8090
  apiKey: process.env.MEDIA_SERVICE_API_KEY!,
});
```

### Upload

```typescript
const result = await media.upload(file, "photo.jpg", {
  orgSlug: "my-org",
  generateVariants: true,
  async: true,
});

console.log(result.mediaFile?.url);
console.log(result.mediaFile?.variants);

// If async, poll for completion:
if (result.jobId) {
  const status = await media.pollJobStatus(result.jobId);
  console.log(status.state); // "completed" | "failed"
}
```

### Upload from Buffer (Node.js)

```typescript
import { readFileSync } from "fs";

const buffer = readFileSync("./photo.jpg");
const result = await media.upload(buffer, "photo.jpg", {
  orgSlug: "my-org",
  generateVariants: true,
});
```

### Crop

```typescript
const cropped = await media.crop({
  objectKey: "my-org/photo-1234/original.jpg",
  x: 100,
  y: 50,
  width: 800,
  height: 600,
  rotate: 90,
  quality: 85,
  format: "webp",
  regenerateVariants: true,
});

console.log(cropped.mediaFile?.url);
```

### Delete

```typescript
await media.delete("my-org/photo-1234/original.jpg");
```

### Presigned URLs

```typescript
// Download URL (for private files)
const { url } = await media.getPresignedDownloadUrl({
  objectKey: "my-org/photo-1234/original.jpg",
  expiry: 3600, // 1 hour
});

// Upload URL (for direct browser-to-S3 uploads)
const { uploadUrl, objectKey } = await media.getPresignedUploadUrl({
  orgSlug: "my-org",
  filename: "photo.jpg",
  mimeType: "image/jpeg",
});
```

### Regenerate Variants

```typescript
const { jobId } = await media.regenerateVariants(
  "my-org/photo-1234/original.jpg",
);

const status = await media.pollJobStatus(jobId);
```

### Download

```typescript
const { data, contentType } = await media.download(
  "media",
  "my-org/photo-1234/original.jpg",
);
```

### Health Check

```typescript
const healthy = await media.health();
```

## Error Handling

```typescript
import { MediaServiceError } from "@hubflora/media-client";

try {
  await media.upload(file, "photo.jpg", { orgSlug: "my-org" });
} catch (err) {
  if (err instanceof MediaServiceError) {
    console.error(err.status);     // HTTP status code
    console.error(err.body?.error); // Error message from service
  }
}
```

## Types

All request/response types are exported:

```typescript
import type {
  MediaFile,
  MediaVariant,
  UploadResponse,
  CropResponse,
  CropOptions,
  JobStatusResponse,
} from "@hubflora/media-client";
```
