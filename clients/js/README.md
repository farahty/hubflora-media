# @hubflora/media-client

TypeScript/JavaScript client for the [hubflora-media](../../README.md) service.

Two entrypoints:
- `@hubflora/media-client` — Core client, works in Node.js and browsers
- `@hubflora/media-client/react` — React hooks with upload progress, multi-file, and job polling

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

---

## Core Client

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

### Upload with Progress (Browser)

```typescript
import { uploadWithProgress } from "@hubflora/media-client";

const result = await uploadWithProgress(baseUrl, apiKey, file, file.name, {
  orgSlug: "my-org",
  generateVariants: true,
  onProgress: ({ loaded, total, percent }) => {
    console.log(`${percent}% (${loaded}/${total} bytes)`);
  },
  signal: abortController.signal, // optional cancellation
});
```

### Multi-file Upload

```typescript
import { MultiUploader } from "@hubflora/media-client";

const uploader = new MultiUploader(client, baseUrl, apiKey, {
  orgSlug: "my-org",
  generateVariants: true,
  async: true,
  concurrency: 3,
  maxFileSize: 50 * 1024 * 1024, // 50MB
  allowedTypes: ["image/", "application/pdf"],
  onFileStateChange: (file, allFiles) => {
    console.log(file.id, file.status, file.progress.percent);
  },
  onTotalProgress: ({ percent }) => {
    console.log(`Total: ${percent}%`);
  },
});

uploader.addFiles(selectedFiles);
const result = await uploader.start();
// result.completed, result.failed, result.cancelled
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
  expiry: 3600,
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
const { jobId } = await media.regenerateVariants("my-org/photo-1234/original.jpg");
const status = await media.pollJobStatus(jobId);
```

### Download

```typescript
const { data, contentType } = await media.download("media", "my-org/photo-1234/original.jpg");
```

### Health Check

```typescript
const healthy = await media.health();
```

---

## React / Next.js

```bash
# React is a peer dependency (optional for core client)
npm install react @hubflora/media-client
```

### Setup: MediaProvider

Wrap your app (or a subtree) with `MediaProvider`. In Next.js, put this in your root layout or a client component:

```tsx
// app/providers.tsx
"use client";

import { MediaProvider } from "@hubflora/media-client/react";

export function Providers({ children }: { children: React.ReactNode }) {
  return (
    <MediaProvider
      config={{
        baseUrl: process.env.NEXT_PUBLIC_MEDIA_SERVICE_URL!,
        apiKey: process.env.NEXT_PUBLIC_MEDIA_SERVICE_API_KEY!,
      }}
    >
      {children}
    </MediaProvider>
  );
}
```

### useUpload — Single File with Progress

```tsx
"use client";

import { useUpload } from "@hubflora/media-client/react";

export function UploadButton() {
  const { upload, progress, status, error, cancel, reset } = useUpload();

  const handleFile = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    try {
      const result = await upload(file, {
        orgSlug: "my-org",
        generateVariants: true,
        async: true,
      });
      console.log("Uploaded:", result.mediaFile?.url);
    } catch {
      // error state is already set via the hook
    }
  };

  return (
    <div>
      <input type="file" onChange={handleFile} disabled={status === "uploading"} />

      {status === "uploading" && (
        <div>
          <progress value={progress.percent} max={100} />
          <span>{progress.percent}%</span>
          <button onClick={cancel}>Cancel</button>
        </div>
      )}

      {status === "processing" && <p>Generating variants...</p>}
      {status === "completed" && <p>Done!</p>}
      {status === "failed" && <p>Error: {error}</p>}
    </div>
  );
}
```

### useMultiUpload — Multiple Files with Concurrency

```tsx
"use client";

import { useMultiUpload } from "@hubflora/media-client/react";

export function MultiUploader() {
  const { upload, files, totalProgress, isUploading, cancel, cancelAll, reset } =
    useMultiUpload();

  const handleFiles = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const selected = Array.from(e.target.files ?? []);
    if (selected.length === 0) return;

    const result = await upload(selected, {
      orgSlug: "my-org",
      generateVariants: true,
      async: true,
      concurrency: 3,
      maxFileSize: 50 * 1024 * 1024,
      allowedTypes: ["image/"],
    });

    console.log(`${result.completed.length} uploaded, ${result.failed.length} failed`);
  };

  return (
    <div>
      <input type="file" multiple onChange={handleFiles} disabled={isUploading} />

      {isUploading && (
        <div>
          <progress value={totalProgress.percent} max={100} />
          <span>{totalProgress.percent}%</span>
          <button onClick={cancelAll}>Cancel All</button>
        </div>
      )}

      <ul>
        {files.map((f) => (
          <li key={f.id}>
            {f.file.name} — {f.status}
            {f.status === "uploading" && ` (${f.progress.percent}%)`}
            {f.status === "failed" && ` — ${f.error}`}
            {(f.status === "uploading" || f.status === "queued") && (
              <button onClick={() => cancel(f.id)}>Cancel</button>
            )}
          </li>
        ))}
      </ul>

      {!isUploading && files.length > 0 && (
        <button onClick={reset}>Clear</button>
      )}
    </div>
  );
}
```

### useJobStatus — Poll Async Jobs

```tsx
"use client";

import { useJobStatus } from "@hubflora/media-client/react";

export function JobTracker({ jobId }: { jobId: string }) {
  const { status, isPolling, error } = useJobStatus(jobId);

  if (isPolling) return <p>Processing... {status?.progress}%</p>;
  if (error) return <p>Error: {error}</p>;
  if (status?.state === "completed") return <p>Done!</p>;
  if (status?.state === "failed") return <p>Failed: {status.failedReason}</p>;
  return null;
}
```

### Without MediaProvider

All hooks accept an optional config for standalone usage:

```tsx
const { upload } = useUpload({
  baseUrl: "http://localhost:8090",
  apiKey: "my-key",
});
```

---

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

All types are exported from both entrypoints:

```typescript
// Core types
import type {
  MediaFile, MediaVariant, UploadResponse, CropResponse,
  CropOptions, JobStatusResponse, UploadProgress,
  FileUploadState, MultiUploadResult,
} from "@hubflora/media-client";

// React types
import type {
  UseUploadReturn, UseMultiUploadReturn, UseJobStatusReturn,
  MediaProviderProps,
} from "@hubflora/media-client/react";
```
