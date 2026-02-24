// ── Request types ──

export interface UploadOptions {
  file: File | Blob;
  orgSlug: string;
  generateVariants?: boolean;
  async?: boolean;
  alt?: string;
  caption?: string;
  description?: string;
}

export interface UploadWithProgressOptions extends UploadOptions {
  /** Called with progress 0–1 during upload */
  onProgress?: (progress: number) => void;
  /** AbortSignal to cancel the upload */
  signal?: AbortSignal;
}

export interface MultiUploadOptions {
  files: (UploadOptions & { id?: string })[];
  /** Max concurrent uploads (default: 3) */
  concurrency?: number;
  /** Called when any individual file's progress changes */
  onFileProgress?: (fileIndex: number, progress: number) => void;
  /** Called when a file upload completes */
  onFileComplete?: (fileIndex: number, result: UploadResponse) => void;
  /** Called when a file upload fails */
  onFileError?: (fileIndex: number, error: Error) => void;
  /** AbortSignal to cancel all uploads */
  signal?: AbortSignal;
}

export interface MultiUploadResult {
  results: (UploadResponse | null)[];
  errors: (Error | null)[];
}

export interface PresignedUploadOptions {
  orgSlug: string;
  filename: string;
  mimeType?: string;
}

export interface CropOptions {
  objectKey: string;
  bucketName?: string;
  x: number;
  y: number;
  width: number;
  height: number;
  rotate?: number;
  scale?: number;
  quality?: number;
  format?: "webp" | "jpeg" | "png";
  regenerateVariants?: boolean;
  async?: boolean;
}

export interface VariantRegenerateOptions {
  objectKey: string;
  bucketName?: string;
}

export interface VariantsInfoOptions {
  objectKey: string;
  bucket?: string;
}

export interface DeleteOptions {
  objectKey: string;
  bucketName?: string;
}

export interface PresignDownloadOptions {
  objectKey: string;
  bucket?: string;
  /** Expiry in seconds (1–86400). Default: 3600 */
  expiry?: number;
}

export interface DownloadOptions {
  bucket: string;
  objectKey: string;
}

export type VariantName =
  | "thumbnail"
  | "small"
  | "medium"
  | "large"
  | "original_webp";

export interface VariantRedirectOptions {
  bucket: string;
  variantName: VariantName;
  path: string;
}

// ── Response types ──

export interface MediaFile {
  id: string;
  filename: string;
  originalFilename: string;
  mimeType: string;
  fileSize: number;
  width?: number;
  height?: number;
  bucketName: string;
  objectKey: string;
  url: string;
  thumbnailUrl?: string;
  metadata?: ImageMetadata;
  variants?: MediaVariant[];
  createdAt: string;
}

export interface ImageMetadata {
  format: string;
  space: string;
  channels: number;
  orientation: number;
}

export interface MediaVariant {
  name: string;
  width: number;
  height: number;
  fileSize: number;
  objectKey: string;
  url: string;
  mimeType: string;
}

export interface UploadResponse {
  success: boolean;
  mediaFile?: MediaFile;
  jobId?: string;
  error?: string;
}

export interface PresignedUploadResponse {
  uploadUrl: string;
  objectKey: string;
  bucketName: string;
}

export interface CropResponse {
  success: boolean;
  mediaFile?: MediaFile;
  jobId?: string;
  error?: string;
}

export interface VariantRegenerateResponse {
  jobId: string;
}

export interface VariantInfo {
  objectKey: string;
  url: string;
  fileSize: number;
  mimeType: string;
}

export interface VariantsInfoResponse {
  objectKey: string;
  variants: VariantInfo[];
}

export interface DeleteResponse {
  success: boolean;
  error?: string;
}

export interface PresignDownloadResponse {
  url: string;
  expiresAt: string;
}

export interface JobStatusResponse {
  jobId: string;
  state: "pending" | "active" | "completed" | "failed";
  progress: number;
  processedOn?: number;
  finishedOn?: number;
  failedReason?: string;
}

export interface HealthResponse {
  status: string;
}

// ── Client config ──

export interface HubfloraMediaConfig {
  /** Base URL of the media service (e.g. "https://media.hubflora.com") */
  baseUrl: string;
  /** API key sent as X-Media-API-Key header */
  apiKey: string;
  /** Custom fetch implementation (defaults to globalThis.fetch) */
  fetch?: typeof globalThis.fetch;
}

// ── React hook types ──

export type UploadStatus = "idle" | "uploading" | "success" | "error";

export interface FileUploadState {
  id: string;
  file: File;
  progress: number;
  status: UploadStatus;
  result?: UploadResponse;
  error?: Error;
}
