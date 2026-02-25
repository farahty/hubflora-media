# @hubflora/media-client

TypeScript/JavaScript client for the [hubflora-media](https://github.com/farahty/hubflora-media) API. Includes React/Next.js hooks with upload progress tracking.

## Install

```bash
npm install @hubflora/media-client
```

## Quick Start

### Server-to-server (API key)

```ts
import { HubfloraMedia } from "@hubflora/media-client";

const media = new HubfloraMedia({
  baseUrl: "https://media.hubflora.com",
  apiKey: "your-api-key",
});

const result = await media.upload({
  file: myFile,
  orgSlug: "my-org",
  generateVariants: true,
});

console.log(result.mediaFile?.url);
```

### Browser (JWT via token provider)

```ts
import { HubfloraMedia } from "@hubflora/media-client";

const media = new HubfloraMedia({
  baseUrl: "https://media.hubflora.com",
  tokenProvider: async () => {
    const res = await fetch("/api/auth/token");
    const data = await res.json();
    return data.token;
  },
});

// orgSlug is optional with JWT — derived from the token
const result = await media.upload({
  file: myFile,
  generateVariants: true,
});
```

## Upload with Progress

Uses XMLHttpRequest internally for real-time upload progress (fetch doesn't support this).

```ts
const result = await media.uploadWithProgress({
  file: myFile,
  orgSlug: "my-org",
  generateVariants: true,
  onProgress: (progress) => {
    console.log(`${Math.round(progress * 100)}%`);
  },
});
```

### Cancel an upload

```ts
const controller = new AbortController();

media.uploadWithProgress({
  file: myFile,
  orgSlug: "my-org",
  signal: controller.signal,
  onProgress: (p) => console.log(p),
});

// Cancel it
controller.abort();
```

## Multi-File Upload

Upload multiple files with concurrency control and per-file callbacks.

```ts
const { results, errors } = await media.uploadMany({
  files: [
    { file: file1, orgSlug: "my-org", generateVariants: true },
    { file: file2, orgSlug: "my-org", generateVariants: true },
    { file: file3, orgSlug: "my-org" },
  ],
  concurrency: 3,
  onFileProgress: (index, progress) => {
    console.log(`File ${index}: ${Math.round(progress * 100)}%`);
  },
  onFileComplete: (index, result) => {
    console.log(`File ${index} done:`, result.mediaFile?.url);
  },
  onFileError: (index, error) => {
    console.error(`File ${index} failed:`, error.message);
  },
});
```

## Query Media

### Get by ID

```ts
const { mediaFile } = await media.get("550e8400-e29b-41d4-a716-446655440000");
console.log(mediaFile.url);
```

### List with pagination and search

```ts
const { items, total } = await media.list({
  limit: 20,
  offset: 0,
  search: "sunset",
  sort: "created_at",
  order: "desc",
});
```

### Batch get multiple IDs

```ts
const { items } = await media.batchGet(["id-1", "id-2", "id-3"]);
```

### Update metadata

```ts
const { mediaFile } = await media.update("550e8400-...", {
  alt: "Updated alt text",
  caption: "New caption",
  description: "A longer description",
  isPrivate: false,
});
```

## React / Next.js

Import hooks from `@hubflora/media-client/react`. React is an optional peer dependency.

### Setup with Context Provider

```tsx
import { HubfloraMediaProvider } from "@hubflora/media-client/react";
import { useHubfloraMediaClient } from "@hubflora/media-client/react";

function App({ children }) {
  const client = useHubfloraMediaClient({
    baseUrl: "https://media.hubflora.com",
    apiKey: "your-api-key",
  });

  return (
    <HubfloraMediaProvider value={client}>
      {children}
    </HubfloraMediaProvider>
  );
}
```

### Session-Synced Provider (recommended for multi-org)

Use `HubfloraMediaSessionProvider` when your app supports multiple organizations. It handles JWT token management and refreshes the token when the org changes.

```tsx
import { HubfloraMediaSessionProvider } from "@hubflora/media-client/react";

function App({ children }) {
  const org = useCurrentOrg(); // your org context

  return (
    <HubfloraMediaSessionProvider
      baseUrl="https://media.hubflora.com"
      organizationId={org.id}
      getToken={async () => {
        await authClient.organization.setActive({ organizationId: org.id });
        const res = await authClient.$fetch("/token");
        return res.data.token;
      }}
      fallback={<div>Loading...</div>}
    >
      {children}
    </HubfloraMediaSessionProvider>
  );
}
```

All hooks (`useUpload`, `useMultiUpload`, `useHubfloraMedia`) work with both providers.

### `useUpload` — Single file with progress

```tsx
import { useUpload } from "@hubflora/media-client/react";

function FileUploader() {
  const { upload, progress, status, result, error, abort, reset } = useUpload();

  return (
    <div>
      <input
        type="file"
        onChange={(e) => {
          const file = e.target.files?.[0];
          if (file) {
            upload({ file, orgSlug: "my-org", generateVariants: true });
          }
        }}
      />

      {status === "uploading" && (
        <div>
          <progress value={progress} max={1} />
          <span>{Math.round(progress * 100)}%</span>
          <button onClick={abort}>Cancel</button>
        </div>
      )}

      {status === "success" && (
        <div>
          <img src={result?.mediaFile?.thumbnailUrl} alt="" />
          <button onClick={reset}>Upload another</button>
        </div>
      )}

      {status === "error" && (
        <div>
          <p>Error: {error?.message}</p>
          <button onClick={reset}>Retry</button>
        </div>
      )}
    </div>
  );
}
```

### `useMultiUpload` — Multiple files with per-file progress

```tsx
import { useMultiUpload } from "@hubflora/media-client/react";

function MultiUploader() {
  const { upload, files, progress, isUploading, abort, reset, remove } =
    useMultiUpload({ concurrency: 3 });

  return (
    <div>
      <input
        type="file"
        multiple
        onChange={(e) => {
          const selected = Array.from(e.target.files ?? []);
          if (selected.length > 0) {
            upload(selected, { orgSlug: "my-org", generateVariants: true });
          }
        }}
      />

      {files.length > 0 && (
        <div>
          <p>Overall: {Math.round(progress * 100)}%</p>

          {files.map((f, i) => (
            <div key={f.id}>
              <span>{f.file.name}</span>
              <progress value={f.progress} max={1} />
              <span>{f.status}</span>
              {f.status !== "uploading" && (
                <button onClick={() => remove(i)}>Remove</button>
              )}
            </div>
          ))}

          {isUploading && <button onClick={abort}>Cancel All</button>}
          {!isUploading && <button onClick={reset}>Clear</button>}
        </div>
      )}
    </div>
  );
}
```

### Without Context Provider

You can pass a client directly to hooks instead of using the provider:

```tsx
import { HubfloraMedia } from "@hubflora/media-client";
import { useUpload, useMultiUpload } from "@hubflora/media-client/react";

const client = new HubfloraMedia({ baseUrl: "...", apiKey: "..." });

function MyComponent() {
  const { upload, progress } = useUpload(client);
  const multi = useMultiUpload({ client, concurrency: 5 });
  // ...
}
```

## All API Methods

| Method | Description |
|--------|-------------|
| `health()` | Health check (no auth) |
| `upload(opts)` | Upload file |
| `uploadWithProgress(opts)` | Upload with real-time progress |
| `uploadMany(opts)` | Upload multiple files concurrently |
| `presignedUpload(opts)` | Get presigned upload URL |
| `crop(opts)` | Crop/rotate/reformat an image |
| `regenerateVariants(opts)` | Queue variant regeneration |
| `variantsInfo(opts)` | List variants for a file |
| `variantUrl(opts)` | Get CDN URL for a variant |
| `delete(opts)` | Delete file + variants |
| `presignDownload(opts)` | Get presigned download URL |
| `download(opts)` | Download file as Blob |
| `jobStatus(jobId)` | Poll async job status |
| `waitForJob(jobId, interval?, onProgress?)` | Wait for job completion |
| `get(id)` | Get a single media file by ID |
| `list(opts?)` | List media files (paginated, searchable) |
| `batchGet(ids)` | Get multiple media files by IDs |
| `update(id, fields)` | Update media metadata (alt, caption, etc.) |

## Error Handling

```ts
import { HubfloraMediaError } from "@hubflora/media-client";

try {
  await media.upload({ ... });
} catch (err) {
  if (err instanceof HubfloraMediaError) {
    console.error(err.status, err.body);
  }
}
```

## License

MIT
