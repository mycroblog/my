// SPDX-License-Identifier: AGPL-3.0-only
// The search of posts (FR-309, GET /statuses/search). A page can be short or empty while more follow, because the server
// leaves posts out after the search engine answered, so only `next_cursor` says whether the search has ended. Its
// answers are read as a timeline's are: the key follows openapi-react-query's `[method, path, init]` and ends with
// "pages" (features/timeline/api/timelines.ts).
import { infiniteQueryOptions } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import type { components } from "@mycroblog/mykit";

/** A post the search found. */
export type SearchedStatus = components["schemas"]["Status"];

/** The longest search the server accepts, in characters (api/openapi.yaml, searchStatuses `q`). */
export const maxPostQueryLength = 200;

/** The text as the search takes it, or null when it is not one (nothing but spaces, or too long). */
export function postQueryOf(text: string): string | null {
  const query = text.trim();
  return query === "" || Array.from(query).length > maxPostQueryLength ? null : query;
}

/** The pages of the posts that contain the words of `q`, newest first. */
export function statusSearchQueryOptions(api: Api, q: string) {
  return infiniteQueryOptions({
    queryKey: ["get", "/statuses/search", { params: { query: { q } } }, "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const query = pageParam === undefined ? { q } : { q, cursor: pageParam };
      const { data, error } = await api.fetch.GET("/statuses/search", { params: { query }, signal });
      // The client's middleware throws an ApiError for every error status (lib/api/client.ts).
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (lastPage) => lastPage.next_cursor,
    // Every request counts against the search's rate limit: a failure is shown at once with what to do next, and a
    // search stays as it is until a write marks it stale or the viewer sends it again. Coming back to the page shows
    // the same posts in the same place.
    retry: false,
    staleTime: Infinity,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
}
