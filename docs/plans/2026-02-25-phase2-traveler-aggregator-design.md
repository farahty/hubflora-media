# Phase 2: traveler-aggregator — Media Service Integration

## Overview

Update traveler-aggregator to use the Go media service as the source of truth for media
operations. The browser interacts with the media service directly via `@hubflora/media-client`.
Traveler-aggregator's role shrinks to: schema management, token provisioning, and direct
DB reads for joins.

## What Changes

| Area | Before (Phase 1) | After (Phase 2) |
|------|-------------------|------------------|
| Upload flow | Browser → Next.js route → mediaClient → Go service → Next.js persists to DB | Browser → Go service (via SDK, JWT auth, Go persists to DB) |
| Delete flow | Server action → mediaClient.delete() → server action deletes from DB | Browser → Go service (or server action → Go service, both persist to DB) |
| List/search | Server action queries DB directly via Drizzle | Browser → Go service API (or direct DB reads for joins) |
| Update metadata | Server action updates DB via Drizzle | Browser → Go service PATCH endpoint |
| Auth | API key only (service-to-service) | Better Auth JWT (browser) + API key (server) |
| React components | Call server actions | Call SDK hooks directly |

## What Stays the Same

- **Drizzle schema** (`db/schema/media.ts`) — source of truth for table definitions and migrations
- **Direct DB reads for joins** — e.g., "get page with featured image" uses SQL join, not Go service API
- **Foreign key references** — `pages.featuredImageId`, `opportunityAttachments.mediaFileId`, etc.
- **React UI components** (`lib/media/components/`) — same components, different data source

---

## 1. Better Auth Plugin Additions

### 1.1 Add JWT + Bearer Plugins

File: `lib/auth/index.ts`

```typescript
import { jwt, bearer } from "better-auth/plugins";

export const auth = betterAuth({
  // ... existing config ...
  plugins: [
    // ... existing plugins (nextCookies, anonymous, organization, apiKey, admin, phoneNumber) ...

    // NEW: JWT access tokens with custom claims
    jwt({
      jwt: {
        issuer: process.env.BETTER_AUTH_URL,
        audience: process.env.BETTER_AUTH_URL,
        expirationTime: "15m",
        definePayload: async ({ user, session }) => {
          // Resolve orgSlug from activeOrganizationId
          const orgSlug = session.activeOrganizationId
            ? await resolveOrgSlug(session.activeOrganizationId)
            : null;

          return {
            sub: user.id,
            orgId: session.activeOrganizationId ?? null,
            orgSlug: orgSlug,
          };
        },
      },
    }),

    // NEW: Enable Authorization: Bearer header support
    bearer(),
  ],
});
```

### 1.2 Org Slug Resolver

The `definePayload` callback needs to resolve `organizationId` → `orgSlug`. This uses
the existing organization cache:

```typescript
import { getCachedOrganization } from "@/lib/cache/organization-cache";

async function resolveOrgSlug(orgId: string): Promise<string | null> {
  try {
    const org = await getCachedOrganization(orgId);
    return org?.slug ?? null;
  } catch {
    return null;
  }
}
```

### 1.3 What This Enables

- `GET /api/auth/jwks` — Public endpoint exposing JWKS for external JWT validation
- `GET /api/auth/token` — Returns a signed JWT access token to authenticated clients
- The Go service validates tokens by fetching JWKS from this endpoint
- JWT signing uses EdDSA (Ed25519) by default — no shared secret needed

### 1.4 Client-Side Plugin

File: `lib/auth/auth-client.ts`

Add the JWT client plugin (if Better Auth requires a client-side counterpart — check
docs at implementation time):

```typescript
import { jwtClient } from "better-auth/client/plugins";

export const authClient = createAuthClient({
  plugins: [
    // ... existing plugins ...
    jwtClient(),
  ],
});
```

### 1.5 Trusted Origins Update

Add the media service domain to trusted origins:

```typescript
// In trustedOrigins function, add:
"https://media.hubflora.com",
"http://media.lvh.me:8090",  // dev
```

---

## 2. Environment Variables

### 2.1 New Variables

```env
# Public URL for the media service (used by browser clients)
NEXT_PUBLIC_MEDIA_SERVICE_URL=https://media.hubflora.com
```

### 2.2 Existing Variables (no change)

```env
# Server-side media service URL (for server-to-server calls)
MEDIA_SERVICE_URL=https://media.hubflora.com
MEDIA_SERVICE_API_KEY=<shared-secret>
```

---

## 3. Media Client Configuration

### 3.1 Server-Side Client (unchanged)

File: `lib/media-service/client.ts`

```typescript
import { HubfloraMedia } from "@hubflora/media-client";

// Server-to-server: uses API key auth
export const mediaClient = new HubfloraMedia({
  baseUrl: process.env.MEDIA_SERVICE_URL || "http://localhost:8090",
  apiKey: process.env.MEDIA_SERVICE_API_KEY || "",
});
```

This is used by server actions that still need to call the media service
(e.g., background PDF uploads for invoices, server-initiated operations).

### 3.2 Browser-Side Client (new)

The browser client is created by the React provider. No separate file needed —
the provider handles client creation with JWT auth.

---

## 4. React Provider Integration

### 4.1 Media Provider Setup

File: `lib/media/providers/media-provider.tsx` (new)

```typescript
"use client";

import { HubfloraMediaProvider } from "@hubflora/media-client/react";
import { authClient } from "@/lib/auth/auth-client";
import { useOrganization } from "@/hooks/use-organization"; // or equivalent

export function MediaProvider({ children }: { children: React.ReactNode }) {
  const { organizationId } = useOrganization();

  return (
    <HubfloraMediaProvider
      baseUrl={process.env.NEXT_PUBLIC_MEDIA_SERVICE_URL!}
      organizationId={organizationId}
      getToken={async () => {
        // 1. Sync active org (closes the stale-JWT gap)
        if (organizationId) {
          await authClient.organization.setActive({ organizationId });
        }
        // 2. Get fresh JWT access token from Better Auth
        const res = await authClient.$fetch("/token");
        return res.data.token;
      }}
    >
      {children}
    </HubfloraMediaProvider>
  );
}
```

### 4.2 Mount in Layout

File: `app/admin/[industry]/layout.tsx` (or wherever org-scoped admin layout lives)

```typescript
import { MediaProvider } from "@/lib/media/providers/media-provider";

export default function AdminLayout({ children }) {
  return (
    <MediaProvider>
      {/* ... existing layout ... */}
      {children}
    </MediaProvider>
  );
}
```

---

## 5. Component Migration

### 5.1 Upload Components

**Before:** Components call server actions which call `mediaClient.upload()`.

**After:** Components use SDK hooks directly — the upload goes straight to the Go service.

Example migration for `useMediaUpload` hook:

```typescript
// Before (calls Next.js API route)
const response = await fetch("/api/media/enhanced-upload", {
  method: "POST",
  body: formData,
});

// After (calls Go service directly via SDK)
import { useUpload } from "@hubflora/media-client/react";

const { upload, progress, status } = useUpload();
const result = await upload({
  file,
  generateVariants: true,
  alt: "...",
});
// result.mediaFile.id is the DB record ID — ready to use as FK
```

### 5.2 Media Library

The media library currently calls `getMediaFiles()` server action which queries DB
via Drizzle. After Phase 2, it can either:

**Option A (recommended):** Call the Go service SDK:
```typescript
import { useHubfloraMedia } from "@hubflora/media-client/react";

const media = useHubfloraMedia();
const { items, total } = await media.list({ limit: 50, search: "..." });
```

**Option B:** Keep the server action for listing (direct DB read). This is valid
since it's the same database. Choose based on whether the media library needs to
work without traveler-aggregator context.

### 5.3 Media Selector / MediaField

These components use `getMediaFilesByIds()` to display selected media. Migration:

```typescript
// Before
const files = await getMediaFilesByIds(selectedIds);

// After — via SDK
const { items } = await media.batchGet(selectedIds);
```

### 5.4 Image Crop

```typescript
// Before (server action)
const result = await cropImage({ objectKey, x, y, width, height, ... });

// After (SDK direct)
const result = await media.crop({ objectKey, x, y, width, height, ... });
```

### 5.5 Delete

```typescript
// Before (server action deletes from DB + calls Go service)
await deleteMediaFile(fileId);

// After (SDK — Go service deletes from DB + S3)
await media.delete({ id: fileId });
```

---

## 6. Code to Remove

After Phase 2 is complete and validated:

### 6.1 Remove

| File | Reason |
|------|--------|
| `app/api/media/enhanced-upload/route.ts` | Browser uploads go directly to Go service |
| `lib/media/actions/upload-internal.ts` | Server-side uploads use `mediaClient` directly (API key auth) |
| `lib/media/hooks/use-media-upload.ts` | Replaced by `useUpload` / `useMultiUpload` from SDK |

### 6.2 Simplify

| File | Change |
|------|--------|
| `lib/media/actions/media.ts` | Remove `deleteMediaFile` (Go service handles), keep `getMediaFiles` and `getMediaFilesByIds` for direct DB reads if needed for joins |
| `lib/actions/image-crop.ts` | Remove — crop goes through SDK directly |
| `lib/actions/invoice-actions.ts` | Keep — uses `mediaClient` (API key, server-side), but no longer needs to persist to DB |
| `lib/actions/quote-actions.ts` | Same as invoice-actions |
| `lib/actions/activity-attachment-actions.ts` | Simplify — only needs to store FK reference, not persist media record |
| `lib/actions/opportunity-attachment-actions.ts` | Same as activity-attachments |

### 6.3 Keep (no change)

| File | Reason |
|------|--------|
| `db/schema/media.ts` | Source of truth for schema/migrations |
| `lib/media-service/client.ts` | Server-side API key client for background operations |
| `lib/media/components/*` | UI components — updated to use SDK hooks but structure stays |
| `lib/media/hooks/use-media-editor.ts` | Editor UI logic stays |
| `lib/media/hooks/use-media-variant.ts` | May simplify but concept stays |
| `lib/media/hooks/use-drop-zone.ts` | Drop zone logic stays |
| `components/media/media-renderer.tsx` | Image rendering component stays |

---

## 7. Server Action Changes for Attachment Workflows

Some server actions create media records as part of larger workflows (invoice PDFs,
opportunity attachments). These continue using the API key client, but no longer need
to persist to the media tables — the Go service handles that.

### 7.1 Before (Phase 1)

```typescript
// lib/actions/invoice-actions.ts
const { mediaClient } = await import("@/lib/media-service/client");
const uploadResult = await mediaClient.upload({ file, orgSlug, generateVariants: true });

// Then persist to DB manually:
await db.insert(mediaFiles).values({
  id: uploadResult.mediaFile.id,
  filename: uploadResult.mediaFile.filename,
  // ... 15+ fields
});
```

### 7.2 After (Phase 2)

```typescript
// lib/actions/invoice-actions.ts
const { mediaClient } = await import("@/lib/media-service/client");
const uploadResult = await mediaClient.upload({
  file,
  orgSlug,
  generateVariants: true,
});

// DB record already created by Go service — just use the ID
const mediaFileId = uploadResult.mediaFile.id;

// Use mediaFileId as FK in invoice/attachment record
await db.insert(invoiceAttachments).values({
  invoiceId,
  mediaFileId,  // FK reference — no need to insert into media_files
});
```

---

## 8. Direct DB Reads (Joins)

Traveler-aggregator keeps the ability to read media tables directly for SQL joins.
This is efficient because it's the same database — no API call overhead.

### 8.1 Example: Page with Featured Image

```typescript
// This stays unchanged — direct Drizzle query with join
const page = await db.query.page.findFirst({
  where: eq(pages.id, pageId),
  with: {
    featuredImage: true,  // joins media_files via FK
  },
});
```

### 8.2 Example: List Media for Org (if keeping server action)

```typescript
// This can stay as a direct DB read for performance
// OR be replaced by SDK call — your choice per use case
const files = await db.query.mediaFiles.findMany({
  where: eq(mediaFiles.organizationId, orgId),
  limit: 50,
  orderBy: desc(mediaFiles.createdAt),
});
```

### 8.3 Rule of Thumb

- **Writes** (upload, delete, update, crop): Always go through the Go service
- **Reads with joins**: Use direct DB query (same database, no network hop)
- **Standalone reads** (media library, search): Either direct DB or Go service API — both valid

---

## 9. Migration Strategy

### Step 1: Add Better Auth plugins
- Add `jwt()` and `bearer()` plugins to `lib/auth/index.ts`
- Add client-side plugin if needed
- Verify `/api/auth/jwks` endpoint works
- Verify `/api/auth/token` returns JWT with orgId + orgSlug claims
- Add `NEXT_PUBLIC_MEDIA_SERVICE_URL` env var

### Step 2: Create Media Provider
- Create `lib/media/providers/media-provider.tsx`
- Mount in admin layout
- Verify session sync + token flow works

### Step 3: Migrate components incrementally
- Start with upload flow (highest impact)
- Then media library (list/search)
- Then crop/edit
- Then delete
- Keep old code paths working alongside new ones during migration

### Step 4: Simplify server actions
- Remove DB persistence from upload-related server actions
- Simplify attachment actions to only store FK references
- Keep direct DB reads for joins

### Step 5: Clean up
- Remove `app/api/media/enhanced-upload/route.ts`
- Remove `lib/media/actions/upload-internal.ts`
- Remove `lib/media/hooks/use-media-upload.ts`
- Remove `lib/actions/image-crop.ts`
- Simplify remaining server actions

### Step 6: Verify
- All upload flows work (browser direct + server-side)
- Media library lists/searches correctly
- Crop/edit works via SDK
- Delete cascades (DB + S3) correctly
- Attachment workflows still function
- Direct DB joins still work
- Old upload route is gone, no regressions
