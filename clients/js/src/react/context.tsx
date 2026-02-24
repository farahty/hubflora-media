"use client";

import {
  createContext,
  useContext,
  useMemo,
  type ReactNode,
} from "react";
import { HubfloraMediaClient, type MediaClientConfig } from "../index";

const MediaClientContext = createContext<HubfloraMediaClient | null>(null);

export interface MediaProviderProps {
  children: ReactNode;
  config: MediaClientConfig;
}

/**
 * Provides the HubfloraMediaClient to all child components.
 *
 * ```tsx
 * <MediaProvider config={{ baseUrl: "...", apiKey: "..." }}>
 *   <App />
 * </MediaProvider>
 * ```
 */
export function MediaProvider({ children, config }: MediaProviderProps) {
  const client = useMemo(
    () => new HubfloraMediaClient(config),
    [config.baseUrl, config.apiKey, config.timeout],
  );

  return (
    <MediaClientContext.Provider value={client}>
      {children}
    </MediaClientContext.Provider>
  );
}

/**
 * Returns the HubfloraMediaClient from context.
 * Must be used inside a `<MediaProvider>`.
 */
export function useMediaClient(): HubfloraMediaClient {
  const client = useContext(MediaClientContext);
  if (!client) {
    throw new Error("useMediaClient must be used within a <MediaProvider>");
  }
  return client;
}
