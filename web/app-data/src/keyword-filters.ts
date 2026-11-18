// SPDX-License-Identifier: AGPL-3.0-only
// The viewer's keyword filters (ADR-0157): the words that fold or hide a post where the filter applies. The
// server filters; the client lists, makes, changes and deletes them. After a filter is saved or deleted, the timelines
// and the other places the posts show are read again (ADR-0157 §10), since a post may now be folded or shown.
import type { components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { useAppData } from "./provider";

/** A keyword filter: its name, what it does, where, its words, and whether it expired. */
export type KeywordFilter = components["schemas"]["KeywordFilter"];

/** What a filter is made or replaced with. */
export type KeywordFilterRequest = components["schemas"]["KeywordFilterRequest"];

/** A word or phrase of a filter, and whether it matches whole words only. */
export type FilterKeyword = components["schemas"]["KeywordFilterKeyword"];

/** What a filter does to a post it matches: fold it behind its name (`warn`) or leave it out (`hide`). */
export type FilterAction = components["schemas"]["KeywordFilterAction"];

/** Where a filter applies. */
export type FilterContext = components["schemas"]["KeywordFilterContext"];

/** Every place a filter may apply, in the order the server names them. */
export const filterContexts: readonly FilterContext[] = ["home", "notifications", "public", "thread", "account"];

/** The viewer's filters, the expired ones too. */
export function keywordFiltersQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/filters");
}

/** The filter of `id`. */
export function keywordFilterQuery({ queries }: AppData, id: string) {
  return queries.queryOptions("get", "/filters/{id}", { params: { path: { id } } });
}

/** The viewer's filters, empty until the server answered. */
export function useKeywordFilters(): { filters: readonly KeywordFilter[]; status: "pending" | "error" | "success" } {
  const query = useQuery(keywordFiltersQuery(useAppData()));
  return { filters: query.data?.items ?? [], status: query.status };
}

/** The filter of `id`, undefined until the server answered, and its failure. */
export function useKeywordFilter(id: string): { filter: KeywordFilter | undefined; error: Error | null } {
  const query = useQuery(keywordFilterQuery(useAppData(), id));
  return { filter: query.data, error: query.error as Error | null };
}

/** Reads the shown timelines and notifications, posts and threads again, now that the filters changed. */
async function reloadFiltered(data: AppData): Promise<void> {
  await data.queryClient.invalidateQueries({
    predicate: ({ queryKey }) =>
      typeof queryKey[1] === "string" &&
      (queryKey[1].startsWith("/timelines/") ||
        queryKey[1] === "/notifications" ||
        queryKey[1] === "/accounts/{id}/statuses" ||
        queryKey[1] === "/statuses/{id}/context" ||
        queryKey[1] === "/bookmarks"),
  });
}

/**
 * Makes a filter (no `id`) or replaces the one of `id`, and resolves with the filter as saved once the cache holds it
 * and the open timelines are loaded again. It rejects with the server's problem (too many filters or words).
 */
export async function saveKeywordFilter(
  data: AppData,
  filter: KeywordFilterRequest,
  id?: string,
): Promise<KeywordFilter> {
  const { api, queryClient } = data;
  const { data: saved } =
    id === undefined
      ? await api.POST("/filters", { body: filter })
      : await api.PATCH("/filters/{id}", { params: { path: { id } }, body: filter });
  if (saved === undefined) throw new Error("app-data: the filter was saved without an answer");
  queryClient.setQueryData(keywordFilterQuery(data, saved.id).queryKey, saved);
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: keywordFiltersQuery(data).queryKey }),
    reloadFiltered(data),
  ]);
  return saved;
}

/** Deletes the filter of `id`, and resolves once the list is without it and the open timelines are loaded again. */
export async function deleteKeywordFilter(data: AppData, id: string): Promise<void> {
  const { api, queryClient } = data;
  await api.DELETE("/filters/{id}", { params: { path: { id } } });
  queryClient.removeQueries({ queryKey: keywordFilterQuery(data, id).queryKey, exact: true });
  queryClient.setQueryData<components["schemas"]["KeywordFilterCollection"]>(
    keywordFiltersQuery(data).queryKey,
    (now) => (now === undefined ? now : { ...now, items: now.items.filter((f) => f.id !== id) }),
  );
  await reloadFiltered(data);
}

/** Makes, replaces and deletes filters, from a screen (`saveKeywordFilter`, `deleteKeywordFilter`). */
export function useKeywordFilterWrites(): {
  readonly save: (filter: KeywordFilterRequest, id?: string) => Promise<KeywordFilter>;
  readonly remove: (id: string) => Promise<void>;
} {
  const data = useAppData();
  return { save: (filter, id) => saveKeywordFilter(data, filter, id), remove: (id) => deleteKeywordFilter(data, id) };
}
