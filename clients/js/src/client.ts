import type {
  MediaClientConfig,
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

export class MediaServiceError extends Error {
  constructor(
    message: string,
    public status: number,
    public body?: ErrorResponse,
  ) {
    super(message);
    this.name = "MediaServiceError";
  }
}

export class HubfloraMediaClient {
  private baseUrl: string;
  private apiKey: string;
  private timeout: number;

  constructor(config: MediaClientConfig) {
    this.baseUrl = config.baseUrl.replace(/\/+$/, "");
    this.apiKey = config.apiKey;
    this.timeout = config.timeout ?? 120_000;
  }

  // ─── Upload ────────────────────────────────────────────────────────

  /**
   * Upload a file to the media service.
   *
   * @param file - File, Blob, or Buffer to upload.
   * @param filename - Original filename (used for extension detection and folder structure).
   * @param options - Upload options (orgSlug is required).
   */
  async upload(
    file: File | Blob | Buffer,
    filename: string,
    options: UploadOptions,
  ): Promise<UploadResponse> {
    const form = new FormData();

    if (Buffer.isBuffer(file)) {
      form.append("file", new Blob([file]), filename);
    } else {
      form.append("file", file, filename);
    }

    form.append("orgSlug", options.orgSlug);
    if (options.generateVariants) form.append("generateVariants", "true");
    if (options.async) form.append("async", "true");
    if (options.alt) form.append("alt", options.alt);
    if (options.caption) form.append("caption", options.caption);
    if (options.description) form.append("description", options.description);

    return this.request<UploadResponse>("POST", "/api/v1/media/upload", {
      body: form,
    });
  }

  // ─── Delete ────────────────────────────────────────────────────────

  /**
   * Delete a file and all its variants from storage.
   */
  async delete(objectKey: string, bucketName?: string): Promise<DeleteResponse> {
    return this.request<DeleteResponse>("DELETE", "/api/v1/media", {
      json: { objectKey, bucketName },
    });
  }

  // ─── Crop ──────────────────────────────────────────────────────────

  /**
   * Crop an image, replace the original, and optionally regenerate variants.
   */
  async crop(options: CropOptions): Promise<CropResponse> {
    return this.request<CropResponse>("POST", "/api/v1/media/crop", {
      json: options,
    });
  }

  // ─── Presigned URLs ────────────────────────────────────────────────

  /**
   * Get a presigned download URL for an existing file.
   */
  async getPresignedDownloadUrl(
    options: PresignedDownloadOptions,
  ): Promise<PresignedDownloadResponse> {
    const params = new URLSearchParams();
    params.set("objectKey", options.objectKey);
    if (options.bucketName) params.set("bucket", options.bucketName);
    if (options.expiry) params.set("expiry", String(options.expiry));

    return this.request<PresignedDownloadResponse>(
      "GET",
      `/api/v1/media/presign?${params}`,
    );
  }

  /**
   * Get a presigned upload URL for direct browser-to-S3 uploads.
   */
  async getPresignedUploadUrl(
    options: PresignedUploadOptions,
  ): Promise<PresignedUploadResponse> {
    return this.request<PresignedUploadResponse>(
      "POST",
      "/api/v1/media/upload/presigned",
      { json: options },
    );
  }

  // ─── Variants ──────────────────────────────────────────────────────

  /**
   * Trigger async variant regeneration for an existing file.
   * Returns the job ID for status polling.
   */
  async regenerateVariants(
    objectKey: string,
    bucketName?: string,
  ): Promise<VariantRegenerateResponse> {
    return this.request<VariantRegenerateResponse>(
      "POST",
      "/api/v1/media/variants",
      { json: { objectKey, bucketName } },
    );
  }

  // ─── Job Status ────────────────────────────────────────────────────

  /**
   * Check the status of an async processing job.
   */
  async getJobStatus(jobId: string): Promise<JobStatusResponse> {
    return this.request<JobStatusResponse>(
      "GET",
      `/api/v1/media/job/${encodeURIComponent(jobId)}`,
    );
  }

  /**
   * Poll a job until it reaches a terminal state (completed/failed).
   *
   * @param jobId - The job ID to poll.
   * @param intervalMs - Polling interval in ms. Default: 1000.
   * @param maxAttempts - Maximum poll attempts. Default: 120.
   */
  async pollJobStatus(
    jobId: string,
    intervalMs = 1000,
    maxAttempts = 120,
  ): Promise<JobStatusResponse> {
    for (let i = 0; i < maxAttempts; i++) {
      const status = await this.getJobStatus(jobId);
      if (status.state === "completed" || status.state === "failed") {
        return status;
      }
      await new Promise((resolve) => setTimeout(resolve, intervalMs));
    }
    throw new MediaServiceError(
      `Job ${jobId} did not complete within ${maxAttempts} attempts`,
      408,
    );
  }

  // ─── Download ──────────────────────────────────────────────────────

  /**
   * Download a file from the media service.
   * Returns the raw Response so the caller can stream or buffer as needed.
   */
  async download(
    bucket: string,
    objectKey: string,
  ): Promise<{ data: ArrayBuffer; contentType: string }> {
    const resp = await this.rawRequest(
      "GET",
      `/api/v1/media/download/${encodeURIComponent(bucket)}/${objectKey}`,
    );
    const data = await resp.arrayBuffer();
    return { data, contentType: resp.headers.get("content-type") ?? "" };
  }

  // ─── Health ────────────────────────────────────────────────────────

  /**
   * Check if the media service is healthy.
   */
  async health(): Promise<boolean> {
    try {
      const resp = await this.rawRequest("GET", "/health");
      return resp.ok;
    } catch {
      return false;
    }
  }

  // ─── Internal ──────────────────────────────────────────────────────

  private async request<T>(
    method: string,
    path: string,
    options?: { body?: BodyInit; json?: unknown },
  ): Promise<T> {
    const resp = await this.rawRequest(method, path, options);
    const body = (await resp.json()) as T;

    if (!resp.ok) {
      throw new MediaServiceError(
        (body as unknown as ErrorResponse)?.error ?? `HTTP ${resp.status}`,
        resp.status,
        body as unknown as ErrorResponse,
      );
    }

    return body;
  }

  private async rawRequest(
    method: string,
    path: string,
    options?: { body?: BodyInit; json?: unknown },
  ): Promise<Response> {
    const url = `${this.baseUrl}${path}`;
    const headers: Record<string, string> = {
      "X-Media-API-Key": this.apiKey,
    };

    let body: BodyInit | undefined = options?.body;

    if (options?.json !== undefined) {
      headers["Content-Type"] = "application/json";
      body = JSON.stringify(options.json);
    }

    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), this.timeout);

    try {
      return await fetch(url, {
        method,
        headers,
        body,
        signal: controller.signal,
      });
    } finally {
      clearTimeout(timer);
    }
  }
}
