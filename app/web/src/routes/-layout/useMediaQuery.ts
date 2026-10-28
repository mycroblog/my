// SPDX-License-Identifier: AGPL-3.0-only
import { useSyncExternalStore } from "react";

/**
 * Whether the viewport matches `query`, following its changes (react.dev, useSyncExternalStore: subscribing
 * to a browser API). Only the app shell uses viewport queries (web-client/README.md D2). Without matchMedia
 * (jsdom), nothing matches.
 */
export function useMediaQuery(query: string): boolean {
  const subscribe = (onChange: () => void) => {
    if (typeof window.matchMedia !== "function") return () => undefined;
    const list = window.matchMedia(query);
    list.addEventListener("change", onChange);
    return () => {
      list.removeEventListener("change", onChange);
    };
  };
  return useSyncExternalStore(
    subscribe,
    () => typeof window.matchMedia === "function" && window.matchMedia(query).matches,
  );
}
