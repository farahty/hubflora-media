"use client";

import { useState, useEffect, useCallback, useRef } from "react";
import type { JobStatusResponse } from "../types";
import { useMediaClient } from "./context";

export interface UseJobStatusOptions {
  /** Polling interval in ms. Default: 1000. */
  interval?: number;
  /** Auto-start polling when jobId is set. Default: true. */
  autoStart?: boolean;
}

export interface UseJobStatusReturn {
  /** Current job status. */
  status: JobStatusResponse | null;
  /** Whether polling is active. */
  isPolling: boolean;
  /** Error if polling failed. */
  error: string | null;
  /** Start polling for a job ID. */
  startPolling: (jobId: string) => void;
  /** Stop polling. */
  stopPolling: () => void;
}

/**
 * Hook to poll the status of an async processing job.
 *
 * ```tsx
 * const { status, isPolling } = useJobStatus(jobId);
 * ```
 */
export function useJobStatus(
  jobId?: string | null,
  options: UseJobStatusOptions = {},
): UseJobStatusReturn {
  const client = useMediaClient();
  const { interval = 1000, autoStart = true } = options;

  const [status, setStatus] = useState<JobStatusResponse | null>(null);
  const [isPolling, setIsPolling] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const activeJobIdRef = useRef<string | null>(null);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const stopPolling = useCallback(() => {
    if (timerRef.current) {
      clearInterval(timerRef.current);
      timerRef.current = null;
    }
    activeJobIdRef.current = null;
    setIsPolling(false);
  }, []);

  const startPolling = useCallback(
    (id: string) => {
      stopPolling();
      activeJobIdRef.current = id;
      setIsPolling(true);
      setError(null);

      const poll = async () => {
        if (activeJobIdRef.current !== id) return;
        try {
          const result = await client.getJobStatus(id);
          setStatus(result);
          if (result.state === "completed" || result.state === "failed") {
            stopPolling();
          }
        } catch (err) {
          setError(err instanceof Error ? err.message : "Polling failed");
          stopPolling();
        }
      };

      // Immediate first poll
      poll();
      timerRef.current = setInterval(poll, interval);
    },
    [client, interval, stopPolling],
  );

  // Auto-start when jobId changes
  useEffect(() => {
    if (jobId && autoStart) {
      startPolling(jobId);
    }
    return stopPolling;
  }, [jobId, autoStart, startPolling, stopPolling]);

  return { status, isPolling, error, startPolling, stopPolling };
}
