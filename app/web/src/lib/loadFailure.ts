// SPDX-License-Identifier: AGPL-3.0-only
// The failure of a query's first load, kept on screen while it loads again (web-client/16-look-states-and-wording.md G16).
import { useState } from "react";

/** What of a query's result (useQuery, useInfiniteQuery) decides its failure. */
type LoadState<TError> = { data: unknown; error: TError | null; errorUpdatedAt: number; isFetching: boolean };

export type LoadFailure<TError = unknown> = {
  /** Why the query failed, also while it is loading again; null when it has not failed. */
  error: TError | null;
  /** The failed query is loading again. */
  isRetrying: boolean;
};

/**
 * The failure a region shows in place of its data. A query without data goes back to `pending` with no `error` when
 * a fetch starts (TanStack Query, `fetchState`), so a region that reads `query.error` would swap its error and its
 * "Try again" button for its loading state during the retry, and the button's focus would be lost. The query keeps
 * `errorUpdatedAt`, which says that the error kept here is that query's, also after a change of the query's key.
 */
export function useLoadFailure<TError>(query: LoadState<TError>): LoadFailure<TError> {
  const [last, setLast] = useState<{ error: TError; at: number } | null>(null);
  // Adjusting state while rendering, on a change of the query's error (react.dev: "Adjusting some state when a prop changes").
  if (query.error !== null && query.error !== last?.error) setLast({ error: query.error, at: query.errorUpdatedAt });
  if (query.error !== null) return { error: query.error, isRetrying: query.isFetching };
  if (query.data === undefined && query.isFetching && last?.at === query.errorUpdatedAt) {
    return { error: last.error, isRetrying: true };
  }
  return { error: null, isRetrying: false };
}
