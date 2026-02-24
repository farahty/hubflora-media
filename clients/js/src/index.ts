// Core client
export { HubfloraMediaClient, MediaServiceError } from "./client";

// Upload with progress (XHR-based, browser only)
export { uploadWithProgress } from "./upload-progress";
export type {
  UploadProgress,
  UploadWithProgressOptions,
} from "./upload-progress";

// Multi-file uploader
export { MultiUploader } from "./multi-upload";
export type {
  FileUploadState,
  FileUploadStatus,
  MultiUploadOptions,
  MultiUploadResult,
} from "./multi-upload";

// Types
export type {
  MediaClientConfig,
  MediaFile,
  MediaVariant,
  MediaType,
  UploadOptions,
  UploadResponse,
  CropOptions,
  CropResponse,
  DeleteResponse,
  PresignedDownloadOptions,
  PresignedDownloadResponse,
  PresignedUploadOptions,
  PresignedUploadResponse,
  JobStatusResponse,
  VariantRegenerateResponse,
  ErrorResponse,
} from "./types";
