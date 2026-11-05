// SPDX-License-Identifier: AGPL-3.0-only
// The full-text search of posts as the server offers it (FR-309, ADR-0149 §9), from GET /instance
// (`full_text_search`): whether it exists, and which visibilities it covers. A server without it shows nothing of it.
import { type QueryClient, useQuery } from "@tanstack/react-query";
import type { Api } from "./api/client";
import { useApi, useSessionStatus } from "./auth/SessionProvider";
import { ApiError } from "./errors";
import { shellQueryReader } from "./query";

/** The problem type of the 404 of the search of posts on a server without a search engine. */
const searchUnavailableType = "/api/v1/problems/search-unavailable";

/** What the search of posts covers on this server, as the catalog's `select` messages take it. */
export type SearchScope = "none" | "public" | "local" | "public_local";

/** What the item of GET /instance says the search covers. */
function searchScopeOf(info: { available: boolean; visibilities: readonly string[] }): SearchScope {
  if (!info.available) return "none";
  if (!info.visibilities.includes("local")) return "public";
  return info.visibilities.includes("public") ? "public_local" : "local";
}

/** What the search of posts covers on this server: `none` until GET /instance loads, and when it cannot. */
export function useSearchScope(): SearchScope {
  const api = useApi();
  // The shell reads GET /instance on every page and handles its failures; this only reads its answer.
  const instance = useQuery({ ...api.query.queryOptions("get", "/instance"), ...shellQueryReader });
  return instance.data === undefined ? "none" : searchScopeOf(instance.data.full_text_search);
}

/** Whether the viewer has the search of posts: the server has it, and only signed-in users can search. */
export function usePostSearchAvailable(): boolean {
  const scope = useSearchScope();
  return useSessionStatus() === "signed-in" && scope !== "none";
}

/** Whether `error` is the server's answer that it has no search engine. */
export function isSearchUnavailable(error: unknown): boolean {
  return error instanceof ApiError && error.type === searchUnavailableType;
}

/**
 * Fetches GET /instance again whenever a search of posts finds that the server has no search engine: the operators
 * turned it off after the page loaded, and the app should stop offering it.
 */
export function refetchInstanceOnSearchUnavailable(queryClient: QueryClient, api: Api): () => void {
  return queryClient.getQueryCache().subscribe((event) => {
    if (event.type === "updated" && event.action.type === "error" && isSearchUnavailable(event.action.error)) {
      void queryClient.invalidateQueries({ queryKey: api.query.queryOptions("get", "/instance").queryKey });
    }
  });
}
