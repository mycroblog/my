// SPDX-License-Identifier: AGPL-3.0-only
// The instance's status page (FR-810, ADR-0085 §6): where its operators tell of outages and maintenance, from GET
// /instance's `status_page_url`. It is needed most when the server does not answer, and then GET /instance cannot
// be read, so the app keeps the last answer in localStorage (lib/storage's statusPageKey), as it keeps the home
// timeline: the Service Worker never answers the client API (web-client/11-pwa.md §11, ADR-0045).
import { useQuery } from "@tanstack/react-query";
import { useEffect } from "react";
import { useApi } from "./auth/SessionProvider";
import { shellQueryReader } from "./query";
import { statusPageKey, useStorage } from "./storage";

/**
 * The address of the instance's status page, or undefined when it has none. Once GET /instance has answered, its
 * answer counts and is kept; until then, and when it cannot be read (offline, the server down), the address kept
 * from the last answer on this browser.
 */
export function useStatusPageUrl(): string | undefined {
  const api = useApi();
  // The shell reads GET /instance on every page and handles its failures; this only reads its answer.
  const instance = useQuery({ ...api.query.queryOptions("get", "/instance"), ...shellQueryReader });
  const stored = useStorage(statusPageKey);
  const loaded = instance.data !== undefined;
  const answered = instance.data?.status_page_url;
  useEffect(() => {
    // Writing the same value again changes nothing; null (no status page) removes the key.
    if (loaded) statusPageKey.write(answered ?? null);
  }, [loaded, answered]);
  return loaded ? answered : (stored ?? undefined);
}
