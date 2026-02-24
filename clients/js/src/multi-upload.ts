import type { UploadResponse } from "./types";
import type { UploadProgress, UploadWithProgressOptions } from "./upload-progress";
import { uploadWithProgress } from "./upload-progress";
import { HubfloraMediaClient } from "./client";

/** Status of a single file in the queue. */
export type FileUploadStatus =
  | "queued"
  | "uploading"
  | "processing"
  | "completed"
  | "failed"
  | "cancelled";

/** State for a single file in the multi-upload queue. */
export interface FileUploadState {
  /** Unique ID for this upload entry. */
  id: string;
  /** The file being uploaded. */
  file: File;
  /** Current status. */
  status: FileUploadStatus;
  /** Upload progress (bytes). */
  progress: UploadProgress;
  /** Upload result (when completed). */
  result?: UploadResponse;
  /** Error message (when failed). */
  error?: string;
  /** AbortController to cancel this upload. */
  abortController: AbortController;
}

/** Options for multi-file upload. */
export interface MultiUploadOptions
  extends Omit<UploadWithProgressOptions, "onProgress" | "signal"> {
  /** Max concurrent uploads. Default: 3. */
  concurrency?: number;
  /** Called when an individual file's state changes. */
  onFileStateChange?: (file: FileUploadState, allFiles: FileUploadState[]) => void;
  /** Called with aggregate progress across all files. */
  onTotalProgress?: (progress: UploadProgress) => void;
  /** Max file size in bytes. Files exceeding this are rejected immediately. */
  maxFileSize?: number;
  /** Allowed MIME type prefixes (e.g. ["image/", "application/pdf"]). */
  allowedTypes?: string[];
}

/** Result of the entire multi-upload batch. */
export interface MultiUploadResult {
  completed: FileUploadState[];
  failed: FileUploadState[];
  cancelled: FileUploadState[];
  totalFiles: number;
}

let idCounter = 0;
function generateId(): string {
  return `upload_${Date.now()}_${++idCounter}`;
}

/**
 * Upload multiple files with concurrency control and per-file progress.
 */
export class MultiUploader {
  private baseUrl: string;
  private apiKey: string;
  private client: HubfloraMediaClient;
  private files: FileUploadState[] = [];
  private options: MultiUploadOptions;
  private active = 0;
  private queue: FileUploadState[] = [];
  private resolveAll?: (result: MultiUploadResult) => void;

  constructor(
    client: HubfloraMediaClient,
    baseUrl: string,
    apiKey: string,
    options: MultiUploadOptions,
  ) {
    this.client = client;
    this.baseUrl = baseUrl;
    this.apiKey = apiKey;
    this.options = options;
  }

  /** Add files to the upload queue. Returns the created states. */
  addFiles(files: File[]): FileUploadState[] {
    const states: FileUploadState[] = [];

    for (const file of files) {
      // Validate file size
      if (this.options.maxFileSize && file.size > this.options.maxFileSize) {
        const state: FileUploadState = {
          id: generateId(),
          file,
          status: "failed",
          progress: { loaded: 0, total: file.size, percent: 0 },
          error: `File too large (max ${formatBytes(this.options.maxFileSize)})`,
          abortController: new AbortController(),
        };
        states.push(state);
        continue;
      }

      // Validate file type
      if (
        this.options.allowedTypes &&
        !this.options.allowedTypes.some((t) => file.type.startsWith(t))
      ) {
        const state: FileUploadState = {
          id: generateId(),
          file,
          status: "failed",
          progress: { loaded: 0, total: file.size, percent: 0 },
          error: `File type ${file.type} not allowed`,
          abortController: new AbortController(),
        };
        states.push(state);
        continue;
      }

      const state: FileUploadState = {
        id: generateId(),
        file,
        status: "queued",
        progress: { loaded: 0, total: file.size, percent: 0 },
        abortController: new AbortController(),
      };
      states.push(state);
    }

    this.files.push(...states);
    return states;
  }

  /** Start uploading all queued files. Returns when all are done. */
  async start(): Promise<MultiUploadResult> {
    this.queue = this.files.filter((f) => f.status === "queued");

    if (this.queue.length === 0) {
      return this.getResult();
    }

    return new Promise((resolve) => {
      this.resolveAll = resolve;
      this.processQueue();
    });
  }

  /** Cancel a specific file upload. */
  cancel(id: string): void {
    const file = this.files.find((f) => f.id === id);
    if (file && (file.status === "queued" || file.status === "uploading")) {
      file.abortController.abort();
      file.status = "cancelled";
      this.notifyChange(file);
    }
  }

  /** Cancel all pending and active uploads. */
  cancelAll(): void {
    for (const file of this.files) {
      if (file.status === "queued" || file.status === "uploading") {
        file.abortController.abort();
        file.status = "cancelled";
        this.notifyChange(file);
      }
    }
  }

  /** Get current state of all files. */
  getFiles(): FileUploadState[] {
    return [...this.files];
  }

  /** Get aggregate result. */
  getResult(): MultiUploadResult {
    return {
      completed: this.files.filter((f) => f.status === "completed"),
      failed: this.files.filter((f) => f.status === "failed"),
      cancelled: this.files.filter((f) => f.status === "cancelled"),
      totalFiles: this.files.length,
    };
  }

  private processQueue(): void {
    const concurrency = this.options.concurrency ?? 3;

    while (this.active < concurrency && this.queue.length > 0) {
      const file = this.queue.shift()!;
      if (file.status === "cancelled") continue;
      this.active++;
      this.uploadOne(file);
    }
  }

  private async uploadOne(state: FileUploadState): Promise<void> {
    state.status = "uploading";
    this.notifyChange(state);

    try {
      const result = await uploadWithProgress(
        this.baseUrl,
        this.apiKey,
        state.file,
        state.file.name,
        {
          ...this.options,
          onProgress: (progress) => {
            state.progress = progress;
            this.notifyChange(state);
            this.emitTotalProgress();
          },
          signal: state.abortController.signal,
        },
      );

      // If async with jobId, poll for variant completion
      if (result.jobId) {
        state.status = "processing";
        state.result = result;
        this.notifyChange(state);

        try {
          await this.client.pollJobStatus(result.jobId, 1000, 120);
        } catch {
          // Variant generation failed but upload succeeded — still mark completed
        }
      }

      state.status = "completed";
      state.result = result;
      state.progress = {
        loaded: state.file.size,
        total: state.file.size,
        percent: 100,
      };
    } catch (err) {
      if (state.status !== "cancelled") {
        state.status = "failed";
        state.error = err instanceof Error ? err.message : "Upload failed";
      }
    }

    this.notifyChange(state);
    this.emitTotalProgress();
    this.active--;

    // Check if everything is done
    const pending = this.files.some(
      (f) =>
        f.status === "queued" ||
        f.status === "uploading" ||
        f.status === "processing",
    );
    if (!pending && this.resolveAll) {
      this.resolveAll(this.getResult());
    } else {
      this.processQueue();
    }
  }

  private notifyChange(file: FileUploadState): void {
    this.options.onFileStateChange?.(file, [...this.files]);
  }

  private emitTotalProgress(): void {
    if (!this.options.onTotalProgress) return;

    let totalLoaded = 0;
    let totalSize = 0;
    for (const f of this.files) {
      totalSize += f.file.size;
      if (f.status === "completed") {
        totalLoaded += f.file.size;
      } else {
        totalLoaded += f.progress.loaded;
      }
    }

    this.options.onTotalProgress({
      loaded: totalLoaded,
      total: totalSize,
      percent: totalSize > 0 ? Math.round((totalLoaded / totalSize) * 100) : 0,
    });
  }
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}
