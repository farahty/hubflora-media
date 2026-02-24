// Context & Provider
export { MediaProvider, useMediaClient } from "./context";
export type { MediaProviderProps } from "./context";

// Single upload hook
export { useUpload } from "./use-upload";
export type {
  UseUploadReturn,
  UseUploadState,
  UseUploadActions,
  UseUploadConfig,
  SingleUploadStatus,
} from "./use-upload";

// Multi upload hook
export { useMultiUpload } from "./use-multi-upload";
export type {
  UseMultiUploadReturn,
  UseMultiUploadState,
  UseMultiUploadActions,
  UseMultiUploadConfig,
} from "./use-multi-upload";

// Job status hook
export { useJobStatus } from "./use-job-status";
export type { UseJobStatusReturn, UseJobStatusOptions } from "./use-job-status";

// Re-export core types for convenience
export type {
  MediaFile,
  MediaVariant,
  UploadResponse,
  CropResponse,
  DeleteResponse,
  JobStatusResponse,
  UploadOptions,
  CropOptions,
  MediaClientConfig,
} from "../types";

export type { UploadProgress, UploadWithProgressOptions } from "../upload-progress";
export type {
  FileUploadState,
  FileUploadStatus,
  MultiUploadOptions,
  MultiUploadResult,
} from "../multi-upload";
