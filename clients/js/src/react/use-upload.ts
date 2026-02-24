"use client";

import { useState, useCallback, useRef } from "react";
import type { UploadResponse } from "../types";
import type { UploadProgress, UploadWithProgressOptions } from "../upload-progress";
import { uploadWithProgress } from "../upload-progress";
import { useMediaClient } from "./context";

export type SingleUploadStatus =
  | "idle"
  | "uploading"
  | "processing"
  | "completed"
  | "failed";

export interface UseUploadState {
  /** Current upload status. */
  status: SingleUploadStatus;
  /** Upload progress. */
  progress: UploadProgress;
  /** Upload result (when completed). */
  result: UploadResponse | null;
  /** Error message (when failed). */
  error: string | null;
  /** Whether an upload is in progress. */
  isUploading: boolean;
}

export interface UseUploadActions {
  /** Upload a single file with progress tracking. */
  upload: (
    file: File,
    options: Omit<UploadWithProgressOptions, "onProgress" | "signal">,
  ) => Promise<UploadResponse>;
  /** Cancel the current upload. */
  cancel: () => void;
  /** Reset state back to idle. */
  reset: () => void;
}

export type UseUploadReturn = UseUploadState & UseUploadActions;

/** Config passed to the hook (provides baseUrl/apiKey without needing context). */
export interface UseUploadConfig {
  baseUrl: string;
  apiKey: string;
}

const INITIAL_PROGRESS: UploadProgress = { loaded: 0, total: 0, percent: 0 };

/**
 * Hook for uploading a single file with progress tracking.
 *
 * Uses the MediaProvider context for config. For standalone usage
 * without context, pass `config` directly.
 *
 * ```tsx
 * const { upload, progress, status, cancel } = useUpload();
 * await upload(file, { orgSlug: "my-org", generateVariants: true });
 * ```
 */
export function useUpload(config?: UseUploadConfig): UseUploadReturn {
  // Try context first, fall back to explicit config
  let client: ReturnType<typeof useMediaClient> | null = null;
  try {
    client = useMediaClient();
  } catch {
    // No context — config must be provided
  }

  const [status, setStatus] = useState<SingleUploadStatus>("idle");
  const [progress, setProgress] = useState<UploadProgress>(INITIAL_PROGRESS);
  const [result, setResult] = useState<UploadResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  const upload = useCallback(
    async (
      file: File,
      options: Omit<UploadWithProgressOptions, "onProgress" | "signal">,
    ): Promise<UploadResponse> => {
      // Resolve baseUrl/apiKey from explicit config or from the client context
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
          "useUpload requires either a <MediaProvider> or a config with baseUrl/apiKey",
        );
      }

      abortRef.current?.abort();
      const controller = new AbortController();
      abortRef.current = controller;

      setStatus("uploading");
      setProgress(INITIAL_PROGRESS);
      setResult(null);
      setError(null);

      try {
        const uploadResult = await uploadWithProgress(
          resolvedBaseUrl,
          resolvedApiKey,
          file,
          file.name,
          {
            ...options,
            onProgress: setProgress,
            signal: controller.signal,
          },
        );

        // Poll for variant processing if async
        if (uploadResult.jobId && client) {
          setStatus("processing");
          setResult(uploadResult);
          try {
            await client.pollJobStatus(uploadResult.jobId, 1000, 120);
          } catch {
            // Variants failed but upload succeeded
          }
        }

        setStatus("completed");
        setResult(uploadResult);
        setProgress({ loaded: file.size, total: file.size, percent: 100 });
        return uploadResult;
      } catch (err) {
        const msg = err instanceof Error ? err.message : "Upload failed";
        setStatus("failed");
        setError(msg);
        throw err;
      }
    },
    [client, config?.baseUrl, config?.apiKey],
  );

  const cancel = useCallback(() => {
    abortRef.current?.abort();
    setStatus("idle");
    setProgress(INITIAL_PROGRESS);
  }, []);

  const reset = useCallback(() => {
    abortRef.current?.abort();
    setStatus("idle");
    setProgress(INITIAL_PROGRESS);
    setResult(null);
    setError(null);
  }, []);

  return {
    status,
    progress,
    result,
    error,
    isUploading: status === "uploading" || status === "processing",
    upload,
    cancel,
    reset,
  };
}
