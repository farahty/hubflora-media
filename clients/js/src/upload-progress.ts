import type { UploadOptions, UploadResponse } from "./types";
import { MediaServiceError } from "./client";

/** Progress info emitted during upload. */
export interface UploadProgress {
  /** Bytes uploaded so far. */
  loaded: number;
  /** Total bytes to upload. */
  total: number;
  /** Percentage 0-100. */
  percent: number;
}

/** Options for upload with progress tracking. */
export interface UploadWithProgressOptions extends UploadOptions {
  /** Called with progress updates during upload. */
  onProgress?: (progress: UploadProgress) => void;
  /** AbortSignal to cancel the upload. */
  signal?: AbortSignal;
}

/**
 * Upload a file using XMLHttpRequest for real progress events.
 * Works in browsers only (uses XHR).
 */
export function uploadWithProgress(
  baseUrl: string,
  apiKey: string,
  file: File | Blob,
  filename: string,
  options: UploadWithProgressOptions,
): Promise<UploadResponse> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    const url = `${baseUrl.replace(/\/+$/, "")}/api/v1/media/upload`;

    const form = new FormData();
    form.append("file", file, filename);
    form.append("orgSlug", options.orgSlug);
    if (options.generateVariants) form.append("generateVariants", "true");
    if (options.async) form.append("async", "true");
    if (options.alt) form.append("alt", options.alt);
    if (options.caption) form.append("caption", options.caption);
    if (options.description) form.append("description", options.description);

    xhr.open("POST", url);
    xhr.setRequestHeader("X-Media-API-Key", apiKey);

    // Progress tracking
    xhr.upload.addEventListener("progress", (e) => {
      if (e.lengthComputable && options.onProgress) {
        options.onProgress({
          loaded: e.loaded,
          total: e.total,
          percent: Math.round((e.loaded / e.total) * 100),
        });
      }
    });

    xhr.addEventListener("load", () => {
      try {
        const body = JSON.parse(xhr.responseText) as UploadResponse;
        if (xhr.status >= 200 && xhr.status < 300) {
          resolve(body);
        } else {
          reject(
            new MediaServiceError(
              body.error ?? `HTTP ${xhr.status}`,
              xhr.status,
              { error: body.error ?? "Upload failed" },
            ),
          );
        }
      } catch {
        reject(new MediaServiceError("Invalid response", xhr.status));
      }
    });

    xhr.addEventListener("error", () => {
      reject(new MediaServiceError("Network error", 0));
    });

    xhr.addEventListener("abort", () => {
      reject(new MediaServiceError("Upload aborted", 0));
    });

    // Abort support
    if (options.signal) {
      if (options.signal.aborted) {
        xhr.abort();
        return;
      }
      options.signal.addEventListener("abort", () => xhr.abort(), {
        once: true,
      });
    }

    xhr.send(form);
  });
}
