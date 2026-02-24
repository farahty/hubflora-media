/** Classifies the uploaded file. */
export type MediaType = "IMAGE" | "VIDEO" | "AUDIO" | "DOCUMENT" | "OTHER";

/** File metadata returned by the service. */
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
  metadata?: Record<string, unknown>;
  variants?: MediaVariant[];
  createdAt: string;
}

/** A generated variant (e.g. thumbnail, small). */
export interface MediaVariant {
  name: string;
  width: number;
  height: number;
  fileSize: number;
  objectKey: string;
  url: string;
  mimeType: string;
}

/** Response from upload endpoint. */
export interface UploadResponse {
  success: boolean;
  mediaFile?: MediaFile;
  jobId?: string;
  error?: string;
}

/** Response from crop endpoint. */
export interface CropResponse {
  success: boolean;
  mediaFile?: MediaFile;
  jobId?: string;
  error?: string;
}

/** Response from delete endpoint. */
export interface DeleteResponse {
  success: boolean;
  error?: string;
}

/** Response from presigned download endpoint. */
export interface PresignedDownloadResponse {
  url: string;
  expiresAt: string;
}

/** Response from presigned upload endpoint. */
export interface PresignedUploadResponse {
  uploadUrl: string;
  objectKey: string;
  bucketName: string;
}

/** Response from job status endpoint. */
export interface JobStatusResponse {
  jobId: string;
  state: string;
  progress: number;
  processedOn?: number;
  finishedOn?: number;
  failedReason?: string;
}

/** Response from variant regeneration endpoint. */
export interface VariantRegenerateResponse {
  jobId: string;
}

/** Generic error response from the service. */
export interface ErrorResponse {
  error: string;
}

/** Options for the upload method. */
export interface UploadOptions {
  orgSlug: string;
  generateVariants?: boolean;
  async?: boolean;
  alt?: string;
  caption?: string;
  description?: string;
}

/** Options for the crop method. */
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

/** Options for presigned download. */
export interface PresignedDownloadOptions {
  objectKey: string;
  bucketName?: string;
  /** Expiry in seconds. Default: 86400 (24h). */
  expiry?: number;
}

/** Options for presigned upload. */
export interface PresignedUploadOptions {
  orgSlug: string;
  filename: string;
  mimeType: string;
}

/** Configuration for the media service client. */
export interface MediaClientConfig {
  /** Base URL of the hubflora-media service (e.g. http://localhost:8090). */
  baseUrl: string;
  /** API key for authentication. */
  apiKey: string;
  /** Request timeout in ms. Default: 120000 (2 min). */
  timeout?: number;
}
