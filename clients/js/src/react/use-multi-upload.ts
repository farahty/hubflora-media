"use client";

import { useState, useCallback, useRef } from "react";
import type { UploadProgress } from "../upload-progress";
import type {
  FileUploadState,
  MultiUploadOptions,
  MultiUploadResult,
} from "../multi-upload";
import { MultiUploader } from "../multi-upload";
import { useMediaClient } from "./context";

export interface UseMultiUploadState {
  /** All file upload states. */
  files: FileUploadState[];
  /** Aggregate progress across all files. */
  totalProgress: UploadProgress;
  /** Whether uploads are in progress. */
  isUploading: boolean;
  /** Batch result (set when all uploads finish). */
  result: MultiUploadResult | null;
}

export interface UseMultiUploadActions {
  /** Start uploading files. Returns when all are done. */
  upload: (
    files: File[],
    options: Omit<
      MultiUploadOptions,
      "onFileStateChange" | "onTotalProgress"
    >,
  ) => Promise<MultiUploadResult>;
  /** Cancel a specific file upload by ID. */
  cancel: (id: string) => void;
  /** Cancel all uploads. */
  cancelAll: () => void;
  /** Reset all state. */
  reset: () => void;
}

export type UseMultiUploadReturn = UseMultiUploadState & UseMultiUploadActions;

/** Config for standalone usage without context. */
export interface UseMultiUploadConfig {
  baseUrl: string;
  apiKey: string;
}

const INITIAL_PROGRESS: UploadProgress = { loaded: 0, total: 0, percent: 0 };

/**
 * Hook for uploading multiple files with per-file progress and concurrency control.
 *
 * ```tsx
 * const { upload, files, totalProgress, cancelAll } = useMultiUpload();
 *
 * const result = await upload(selectedFiles, {
 *   orgSlug: "my-org",
 *   generateVariants: true,
 *   async: true,
 *   concurrency: 3,
 *   maxFileSize: 50 * 1024 * 1024,
 *   allowedTypes: ["image/", "application/pdf"],
 * });
 * ```
 */
export function useMultiUpload(
  config?: UseMultiUploadConfig,
): UseMultiUploadReturn {
  let client: ReturnType<typeof useMediaClient> | null = null;
  try {
    client = useMediaClient();
  } catch {
    // No context
  }

  const [files, setFiles] = useState<FileUploadState[]>([]);
  const [totalProgress, setTotalProgress] =
    useState<UploadProgress>(INITIAL_PROGRESS);
  const [isUploading, setIsUploading] = useState(false);
  const [result, setResult] = useState<MultiUploadResult | null>(null);
  const uploaderRef = useRef<MultiUploader | null>(null);

  const upload = useCallback(
    async (
      inputFiles: File[],
      options: Omit<
        MultiUploadOptions,
        "onFileStateChange" | "onTotalProgress"
      >,
    ): Promise<MultiUploadResult> => {
      let resolvedBaseUrl: string;
      let resolvedApiKey: string;

      if (config) {
        resolvedBaseUrl = config.baseUrl;
        resolvedApiKey = config.apiKey;
      } else if (client) {
        const clientConfig = client.getConfig();
        resolvedBaseUrl = clientConfig.baseUrl;
        resolvedApiKey = clientConfig.apiKey;
      } else {
        throw new Error(
          "useMultiUpload requires either a <MediaProvider> or a config with baseUrl/apiKey",
        );
      }

      // Create fresh uploader instance
      const uploader = new MultiUploader(
        client!,
        resolvedBaseUrl,
        resolvedApiKey,
        {
          ...options,
          onFileStateChange: (_file, allFiles) => {
            setFiles([...allFiles]);
          },
          onTotalProgress: (progress) => {
            setTotalProgress(progress);
          },
        },
      );

      uploaderRef.current = uploader;
      uploader.addFiles(inputFiles);
      setFiles(uploader.getFiles());
      setIsUploading(true);
      setResult(null);
      setTotalProgress(INITIAL_PROGRESS);

      const uploadResult = await uploader.start();
      setIsUploading(false);
      setResult(uploadResult);
      return uploadResult;
    },
    [client, config?.baseUrl, config?.apiKey],
  );

  const cancel = useCallback(
    (id: string) => {
      uploaderRef.current?.cancel(id);
    },
    [],
  );

  const cancelAll = useCallback(() => {
    uploaderRef.current?.cancelAll();
  }, []);

  const reset = useCallback(() => {
    uploaderRef.current?.cancelAll();
    uploaderRef.current = null;
    setFiles([]);
    setTotalProgress(INITIAL_PROGRESS);
    setIsUploading(false);
    setResult(null);
  }, []);

  return {
    files,
    totalProgress,
    isUploading,
    result,
    upload,
    cancel,
    cancelAll,
    reset,
  };
}
