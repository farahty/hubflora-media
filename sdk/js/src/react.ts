import {
  createContext,
  useContext,
  useState,
  useCallback,
  useRef,
  useMemo,
} from "react";
import { HubfloraMedia } from "./client.js";
import type {
  HubfloraMediaConfig,
  UploadOptions,
  UploadResponse,
  FileUploadState,
  UploadStatus,
} from "./types.js";

// ── Context ──

const HubfloraMediaContext = createContext<HubfloraMedia | null>(null);

export const HubfloraMediaProvider = HubfloraMediaContext.Provider;

/**
 * Create a HubfloraMedia client instance (stable across renders).
 *
 * ```tsx
 * const client = useHubfloraMediaClient({
 *   baseUrl: "https://media.hubflora.com",
 *   apiKey: "your-key",
 * });
 * ```
 */
export function useHubfloraMediaClient(config: HubfloraMediaConfig) {
  return useMemo(
    () => new HubfloraMedia(config),
    [config.baseUrl, config.apiKey],
  );
}

/**
 * Access the HubfloraMedia client from context.
 *
 * ```tsx
 * const media = useHubfloraMedia();
 * ```
 */
export function useHubfloraMedia(): HubfloraMedia {
  const ctx = useContext(HubfloraMediaContext);
  if (!ctx) {
    throw new Error(
      "useHubfloraMedia must be used within a <HubfloraMediaProvider>",
    );
  }
  return ctx;
}

// ── Single file upload hook ──

export interface UseUploadReturn {
  /** Trigger the upload */
  upload: (opts: UploadOptions) => Promise<UploadResponse>;
  /** Upload progress 0–1 */
  progress: number;
  /** Current status */
  status: UploadStatus;
  /** Upload result on success */
  result: UploadResponse | null;
  /** Error if failed */
  error: Error | null;
  /** Whether an upload is in progress */
  isUploading: boolean;
  /** Cancel the current upload */
  abort: () => void;
  /** Reset state back to idle */
  reset: () => void;
}

/**
 * Hook for uploading a single file with progress tracking.
 *
 * ```tsx
 * const { upload, progress, status, result } = useUpload();
 *
 * <input type="file" onChange={(e) => {
 *   upload({ file: e.target.files[0], orgSlug: "my-org", generateVariants: true });
 * }} />
 * <p>{Math.round(progress * 100)}%</p>
 * ```
 */
export function useUpload(client?: HubfloraMedia): UseUploadReturn {
  const ctxClient = useContext(HubfloraMediaContext);
  const media = client ?? ctxClient;
  if (!media) {
    throw new Error(
      "useUpload requires a HubfloraMedia client via argument or <HubfloraMediaProvider>",
    );
  }

  const [progress, setProgress] = useState(0);
  const [status, setStatus] = useState<UploadStatus>("idle");
  const [result, setResult] = useState<UploadResponse | null>(null);
  const [error, setError] = useState<Error | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  const abort = useCallback(() => {
    abortRef.current?.abort();
  }, []);

  const reset = useCallback(() => {
    abortRef.current?.abort();
    setProgress(0);
    setStatus("idle");
    setResult(null);
    setError(null);
  }, []);

  const upload = useCallback(
    async (opts: UploadOptions): Promise<UploadResponse> => {
      abortRef.current?.abort();
      const controller = new AbortController();
      abortRef.current = controller;

      setProgress(0);
      setStatus("uploading");
      setResult(null);
      setError(null);

      try {
        const res = await media.uploadWithProgress({
          ...opts,
          onProgress: setProgress,
          signal: controller.signal,
        });
        setStatus("success");
        setResult(res);
        setProgress(1);
        return res;
      } catch (err) {
        const e = err instanceof Error ? err : new Error(String(err));
        setStatus("error");
        setError(e);
        throw e;
      }
    },
    [media],
  );

  return {
    upload,
    progress,
    status,
    result,
    error,
    isUploading: status === "uploading",
    abort,
    reset,
  };
}

// ── Multi-file upload hook ──

export interface UseMultiUploadReturn {
  /** Add files and start uploading */
  upload: (
    files: File[],
    options: Omit<UploadOptions, "file">,
  ) => Promise<void>;
  /** Per-file upload states */
  files: FileUploadState[];
  /** Overall progress 0–1 */
  progress: number;
  /** Whether any upload is in progress */
  isUploading: boolean;
  /** Cancel all uploads */
  abort: () => void;
  /** Reset all state */
  reset: () => void;
  /** Remove a file by index */
  remove: (index: number) => void;
}

/**
 * Hook for uploading multiple files with per-file progress tracking.
 *
 * ```tsx
 * const { upload, files, progress, isUploading } = useMultiUpload({ concurrency: 3 });
 *
 * <input type="file" multiple onChange={(e) => {
 *   upload(Array.from(e.target.files), { orgSlug: "my-org", generateVariants: true });
 * }} />
 *
 * {files.map((f) => (
 *   <div key={f.id}>
 *     {f.file.name}: {Math.round(f.progress * 100)}% — {f.status}
 *   </div>
 * ))}
 * ```
 */
export function useMultiUpload(
  opts?: { concurrency?: number; client?: HubfloraMedia },
): UseMultiUploadReturn {
  const ctxClient = useContext(HubfloraMediaContext);
  const media = opts?.client ?? ctxClient;
  if (!media) {
    throw new Error(
      "useMultiUpload requires a HubfloraMedia client via options or <HubfloraMediaProvider>",
    );
  }

  const concurrency = opts?.concurrency ?? 3;
  const [files, setFiles] = useState<FileUploadState[]>([]);
  const abortRef = useRef<AbortController | null>(null);

  const progress = useMemo(() => {
    if (files.length === 0) return 0;
    return files.reduce((sum, f) => sum + f.progress, 0) / files.length;
  }, [files]);

  const isUploading = useMemo(
    () => files.some((f) => f.status === "uploading"),
    [files],
  );

  const abort = useCallback(() => {
    abortRef.current?.abort();
  }, []);

  const reset = useCallback(() => {
    abortRef.current?.abort();
    setFiles([]);
  }, []);

  const remove = useCallback((index: number) => {
    setFiles((prev) => prev.filter((_, i) => i !== index));
  }, []);

  const upload = useCallback(
    async (inputFiles: File[], options: Omit<UploadOptions, "file">) => {
      abortRef.current?.abort();
      const controller = new AbortController();
      abortRef.current = controller;

      const initial: FileUploadState[] = inputFiles.map((file, i) => ({
        id: `${Date.now()}-${i}`,
        file,
        progress: 0,
        status: "idle" as const,
      }));
      setFiles(initial);

      const uploadOpts = inputFiles.map((file) => ({
        ...options,
        file,
      }));

      await media.uploadMany({
        files: uploadOpts,
        concurrency,
        signal: controller.signal,
        onFileProgress: (index, p) => {
          setFiles((prev) =>
            prev.map((f, i) =>
              i === index ? { ...f, progress: p, status: "uploading" } : f,
            ),
          );
        },
        onFileComplete: (index, result) => {
          setFiles((prev) =>
            prev.map((f, i) =>
              i === index
                ? { ...f, progress: 1, status: "success", result }
                : f,
            ),
          );
        },
        onFileError: (index, error) => {
          setFiles((prev) =>
            prev.map((f, i) =>
              i === index ? { ...f, status: "error", error } : f,
            ),
          );
        },
      });
    },
    [media, concurrency],
  );

  return { upload, files, progress, isUploading, abort, reset, remove };
}
