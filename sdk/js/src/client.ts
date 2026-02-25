import type {
  HubfloraMediaConfig,
  UploadOptions,
  UploadResponse,
  UploadWithProgressOptions,
  MultiUploadOptions,
  MultiUploadResult,
  PresignedUploadOptions,
  PresignedUploadResponse,
  CropOptions,
  CropResponse,
  VariantRegenerateOptions,
  VariantRegenerateResponse,
  VariantsInfoOptions,
  VariantsInfoResponse,
  DeleteOptions,
  DeleteResponse,
  PresignDownloadOptions,
  PresignDownloadResponse,
  DownloadOptions,
  VariantRedirectOptions,
  JobStatusResponse,
  HealthResponse,
  ListMediaOptions,
  ListMediaResponse,
  BatchGetResponse,
  UpdateMediaFields,
  GetMediaResponse,
} from "./types.js";

export class HubfloraMediaError extends Error {
  constructor(
    public status: number,
    public body: string,
  ) {
    super(`HTTP ${status}: ${body}`);
    this.name = "HubfloraMediaError";
  }
}

export class HubfloraMedia {
  private baseUrl: string;
  private apiKey?: string;
  private tokenProvider?: () => Promise<string> | string;
  private _fetch: typeof globalThis.fetch;

  constructor(config: HubfloraMediaConfig) {
    this.baseUrl = config.baseUrl.replace(/\/+$/, "");
    this.apiKey = config.apiKey;
    this.tokenProvider = config.tokenProvider;
    this._fetch = config.fetch ?? globalThis.fetch.bind(globalThis);

    if (!this.apiKey && !this.tokenProvider) {
      throw new Error("HubfloraMedia: either apiKey or tokenProvider is required");
    }
  }

  // ── Internal helpers ──

  private async authHeaders(): Promise<Record<string, string>> {
    if (this.tokenProvider) {
      const token = await this.tokenProvider();
      return { Authorization: `Bearer ${token}` };
    }
    if (this.apiKey) {
      return { "X-Media-API-Key": this.apiKey };
    }
    throw new Error("HubfloraMedia: no auth configured");
  }

  private async request<T>(
    method: string,
    path: string,
    opts?: { headers?: Record<string, string>; body?: BodyInit },
  ): Promise<T> {
    const auth = await this.authHeaders();
    const res = await this._fetch(`${this.baseUrl}${path}`, {
      method,
      headers: { ...auth, ...opts?.headers },
      body: opts?.body,
    });

    const text = await res.text();

    if (!res.ok) {
      throw new HubfloraMediaError(res.status, text);
    }

    return JSON.parse(text) as T;
  }

  private qs(params: Record<string, string | number | undefined>): string {
    const entries = Object.entries(params).filter(
      (e): e is [string, string | number] => e[1] !== undefined,
    );
    if (entries.length === 0) return "";
    return (
      "?" +
      new URLSearchParams(entries.map(([k, v]) => [k, String(v)])).toString()
    );
  }

  private buildUploadFormData(opts: UploadOptions): FormData {
    const fd = new FormData();
    fd.append("file", opts.file);
    if (opts.orgSlug) fd.append("orgSlug", opts.orgSlug);
    if (opts.generateVariants !== undefined)
      fd.append("generateVariants", String(opts.generateVariants));
    if (opts.async !== undefined) fd.append("async", String(opts.async));
    if (opts.alt) fd.append("alt", opts.alt);
    if (opts.caption) fd.append("caption", opts.caption);
    if (opts.description) fd.append("description", opts.description);
    return fd;
  }

  // ── Public API ──

  /** Check if the service is running. No authentication required. */
  async health(): Promise<HealthResponse> {
    const res = await this._fetch(`${this.baseUrl}/healthz`);
    const text = await res.text();
    if (!res.ok) throw new HubfloraMediaError(res.status, text);
    return JSON.parse(text) as HealthResponse;
  }

  /** Upload a file with optional variant generation. */
  async upload(opts: UploadOptions): Promise<UploadResponse> {
    return this.request<UploadResponse>("POST", "/api/v1/media/upload", {
      body: this.buildUploadFormData(opts),
    });
  }

  /**
   * Upload a file with real-time progress tracking.
   * Uses XMLHttpRequest internally since fetch doesn't support upload progress.
   */
  async uploadWithProgress(opts: UploadWithProgressOptions): Promise<UploadResponse> {
    const auth = await this.authHeaders();
    return new Promise((resolve, reject) => {
      const xhr = new XMLHttpRequest();
      const url = `${this.baseUrl}/api/v1/media/upload`;

      xhr.open("POST", url);
      for (const [key, value] of Object.entries(auth)) {
        xhr.setRequestHeader(key, value);
      }

      // Handle abort
      if (opts.signal) {
        if (opts.signal.aborted) {
          reject(new DOMException("Aborted", "AbortError"));
          return;
        }
        opts.signal.addEventListener("abort", () => xhr.abort());
      }

      xhr.upload.addEventListener("progress", (e) => {
        if (e.lengthComputable) {
          opts.onProgress?.(e.loaded / e.total);
        }
      });

      xhr.addEventListener("load", () => {
        if (xhr.status >= 200 && xhr.status < 300) {
          try {
            resolve(JSON.parse(xhr.responseText) as UploadResponse);
          } catch {
            reject(new HubfloraMediaError(xhr.status, xhr.responseText));
          }
        } else {
          reject(new HubfloraMediaError(xhr.status, xhr.responseText));
        }
      });

      xhr.addEventListener("error", () => {
        reject(new Error("Upload network error"));
      });

      xhr.addEventListener("abort", () => {
        reject(new DOMException("Aborted", "AbortError"));
      });

      xhr.send(this.buildUploadFormData(opts));
    });
  }

  /**
   * Upload multiple files with concurrency control and per-file progress.
   */
  async uploadMany(opts: MultiUploadOptions): Promise<MultiUploadResult> {
    const { files, concurrency = 3, signal } = opts;
    const results: (UploadResponse | null)[] = new Array(files.length).fill(
      null,
    );
    const errors: (Error | null)[] = new Array(files.length).fill(null);

    let nextIndex = 0;

    const runNext = async (): Promise<void> => {
      while (nextIndex < files.length) {
        if (signal?.aborted) return;

        const index = nextIndex++;
        const file = files[index];

        try {
          const result = await this.uploadWithProgress({
            ...file,
            onProgress: (p) => opts.onFileProgress?.(index, p),
            signal,
          });
          results[index] = result;
          opts.onFileComplete?.(index, result);
        } catch (err) {
          const error = err instanceof Error ? err : new Error(String(err));
          errors[index] = error;
          opts.onFileError?.(index, error);
        }
      }
    };

    const workers = Array.from(
      { length: Math.min(concurrency, files.length) },
      () => runNext(),
    );

    await Promise.all(workers);

    return { results, errors };
  }

  /** Get a presigned URL for client-side uploads directly to S3. */
  async presignedUpload(
    opts: PresignedUploadOptions,
  ): Promise<PresignedUploadResponse> {
    return this.request<PresignedUploadResponse>(
      "POST",
      "/api/v1/media/upload/presigned",
      {
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(opts),
      },
    );
  }

  /** Crop, rotate, or reformat an uploaded image in place. */
  async crop(opts: CropOptions): Promise<CropResponse> {
    return this.request<CropResponse>("POST", "/api/v1/media/crop", {
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(opts),
    });
  }

  /** Queue async regeneration of all variants for an image. */
  async regenerateVariants(
    opts: VariantRegenerateOptions,
  ): Promise<VariantRegenerateResponse> {
    return this.request<VariantRegenerateResponse>(
      "POST",
      "/api/v1/media/variants",
      {
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(opts),
      },
    );
  }

  /** List all generated variants for a given file. */
  async variantsInfo(
    opts: VariantsInfoOptions,
  ): Promise<VariantsInfoResponse> {
    const query = this.qs({
      objectKey: opts.objectKey,
      bucket: opts.bucket,
    });
    return this.request<VariantsInfoResponse>(
      "GET",
      `/api/v1/media/variants/info${query}`,
    );
  }

  /** Get a redirect URL to a specific variant by name. */
  async variantUrl(opts: VariantRedirectOptions): Promise<string> {
    const path = `/api/v1/media/variant/${opts.bucket}/${opts.variantName}/${opts.path}`;
    const auth = await this.authHeaders();
    const res = await this._fetch(`${this.baseUrl}${path}`, {
      method: "GET",
      headers: auth,
      redirect: "manual",
    });

    const location = res.headers.get("location");
    if (location) return location;

    if (res.ok) return res.url;

    const text = await res.text();
    throw new HubfloraMediaError(res.status, text);
  }

  /** Delete a file and all its variants. */
  async delete(opts: DeleteOptions): Promise<DeleteResponse> {
    return this.request<DeleteResponse>("DELETE", "/api/v1/media/", {
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(opts),
    });
  }

  /** Generate a temporary presigned download URL. */
  async presignDownload(
    opts: PresignDownloadOptions,
  ): Promise<PresignDownloadResponse> {
    const query = this.qs({
      objectKey: opts.objectKey,
      bucket: opts.bucket,
      expiry: opts.expiry,
    });
    return this.request<PresignDownloadResponse>(
      "GET",
      `/api/v1/media/presign${query}`,
    );
  }

  /** Download a file as a Blob (proxied through the media service). */
  async download(opts: DownloadOptions): Promise<Blob> {
    const path = `/api/v1/media/download/${opts.bucket}/${opts.objectKey}`;
    const auth = await this.authHeaders();
    const res = await this._fetch(`${this.baseUrl}${path}`, {
      method: "GET",
      headers: auth,
    });

    if (!res.ok) {
      const text = await res.text();
      throw new HubfloraMediaError(res.status, text);
    }

    return res.blob();
  }

  /** Poll the status of an async job. */
  async jobStatus(jobId: string): Promise<JobStatusResponse> {
    return this.request<JobStatusResponse>(
      "GET",
      `/api/v1/media/job/${jobId}`,
    );
  }

  /**
   * Poll a job until it completes or fails.
   * @param jobId - The job ID to poll
   * @param intervalMs - Polling interval in milliseconds (default: 1000)
   * @param onProgress - Optional callback for progress updates
   */
  async waitForJob(
    jobId: string,
    intervalMs = 1000,
    onProgress?: (status: JobStatusResponse) => void,
  ): Promise<JobStatusResponse> {
    while (true) {
      const status = await this.jobStatus(jobId);
      onProgress?.(status);

      if (status.state === "completed" || status.state === "failed") {
        return status;
      }

      await new Promise((r) => setTimeout(r, intervalMs));
    }
  }

  /** Get a single media file by ID */
  async get(id: string): Promise<GetMediaResponse> {
    return this.request<GetMediaResponse>("GET", `/api/v1/media/${id}`);
  }

  /** List media files for the authenticated org */
  async list(opts?: ListMediaOptions): Promise<ListMediaResponse> {
    const query = this.qs({
      limit: opts?.limit,
      offset: opts?.offset,
      search: opts?.search,
      type: opts?.type,
      sort: opts?.sort,
      order: opts?.order,
    });
    return this.request<ListMediaResponse>("GET", `/api/v1/media/list${query}`);
  }

  /** Get multiple media files by IDs */
  async batchGet(ids: string[]): Promise<BatchGetResponse> {
    return this.request<BatchGetResponse>("POST", "/api/v1/media/batch", {
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ ids }),
    });
  }

  /** Update media metadata */
  async update(id: string, fields: UpdateMediaFields): Promise<GetMediaResponse> {
    return this.request<GetMediaResponse>("PATCH", `/api/v1/media/${id}`, {
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(fields),
    });
  }
}
