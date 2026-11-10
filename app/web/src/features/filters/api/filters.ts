// SPDX-License-Identifier: AGPL-3.0-only
// The user's keyword filters (FR-209, ADR-0157): the list, and adding, changing, and deleting a filter. Only the owner
// sees them. The server matches the posts it returns against them, so a change marks every list of posts, the posts,
// and the notifications stale (web-client/README.md S1, ADR-0028): the client never matches a post itself.
import type { QueryClient } from "@tanstack/react-query";
import type { components } from "@mycroblog/mykit";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import { isPostListQuery } from "../../../lib/query";

export type KeywordFilter = components["schemas"]["KeywordFilter"];
export type KeywordFilterRequest = components["schemas"]["KeywordFilterRequest"];
export type KeywordFilterContext = components["schemas"]["KeywordFilterContext"];
export type KeywordFilterAction = components["schemas"]["KeywordFilterAction"];
type KeywordFilterCollection = components["schemas"]["KeywordFilterCollection"];

/** The places a filter applies, in the order the form lists them (KeywordFilterContext). */
export const filterContexts: readonly KeywordFilterContext[] = ["home", "notifications", "public", "thread", "account"];

/** The most filters a user has (ADR-0157 §9). */
export const maxFilters = 100;

/** The most words of one filter (ADR-0157 §9). */
export const maxFilterWords = 100;

/** The most words of all the user's filters together (ADR-0157 §9). */
export const maxWordsInAll = 1000;

/** The most grapheme clusters of a filter's name (KeywordFilterRequest.title). */
export const filterTitleLimit = 50;

/** The most grapheme clusters of a word once normalized (KeywordFilterKeyword.keyword). */
export const filterWordLimit = 100;

/** The most characters the name and word fields take (`maxLength` of the request's strings). */
export const filterInputLimit = 400;

/** How many grapheme clusters (what a reader sees as one character) `text` has. */
function graphemeCount(text: string): number {
  return Array.from(new Intl.Segmenter(undefined, { granularity: "grapheme" }).segment(text)).length;
}

/** Why a name (trimmed) is not one the server takes, or undefined when it is: 1 to 50 grapheme clusters on one line. */
export function filterTitleProblem(title: string): "empty" | "too-long" | "invalid" | undefined {
  if (title === "") return "empty";
  if (/\p{Cc}/u.test(title)) return "invalid";
  if (graphemeCount(title) > filterTitleLimit) return "too-long";
  return undefined;
}

/**
 * Whether a word (trimmed) is longer than the server takes. The server counts the word once normalized (NFKC), so the
 * same normalization counts here; the server's answer decides the rest.
 */
export function filterWordTooLong(word: string): boolean {
  return graphemeCount(word.normalize("NFKC")) > filterWordLimit;
}

/** The user's keyword filters, oldest first, the expired ones too (GET /filters). */
export function keywordFiltersQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/filters");
}

/** The instance, whose version picks the version of the documentation site's pages (GET /instance). */
export function instanceQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/instance");
}

/** Adds a keyword filter (POST /filters), as the options of useMutation. */
export function createKeywordFilterMutation(api: Api) {
  return {
    mutationKey: ["post", "/filters"] as const,
    mutationFn: async (body: KeywordFilterRequest): Promise<KeywordFilter> => {
      const { data, error } = await api.fetch.POST("/filters", { body });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Replaces a keyword filter (PATCH /filters/{id}), as the options of useMutation. */
export function updateKeywordFilterMutation(api: Api) {
  return {
    mutationKey: ["patch", "/filters/{id}"] as const,
    mutationFn: async ({ id, body }: { id: string; body: KeywordFilterRequest }): Promise<KeywordFilter> => {
      const { data, error } = await api.fetch.PATCH("/filters/{id}", { params: { path: { id } }, body });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Deletes a keyword filter (DELETE /filters/{id}), as the options of useMutation. */
export function deleteKeywordFilterMutation(api: Api) {
  return {
    mutationKey: ["delete", "/filters/{id}"] as const,
    mutationFn: async (filter: KeywordFilter): Promise<KeywordFilter> => {
      const { error } = await api.fetch.DELETE("/filters/{id}", { params: { path: { id: filter.id } } });
      if (error !== undefined) throw new ApiError(error);
      return filter;
    },
  };
}

/**
 * The posts and notifications a filter changes: every list of posts (the timelines of lists too), the posts, and the
 * notifications and their unread count (a hidden notification is not counted). Resolves once those on screen are
 * fetched again (ADR-0157 §10); the others are fetched when their page opens (S7).
 */
function refreshFiltered(queryClient: QueryClient): Promise<void> {
  return queryClient.invalidateQueries({
    predicate: (query) => {
      const [method, path] = query.queryKey;
      if (method !== "get") return false;
      return (
        path === "/statuses/{id}" ||
        path === "/timelines/list/{id}" ||
        path === "/notifications" ||
        path === "/notifications/marker" ||
        isPostListQuery(query.queryKey)
      );
    },
  });
}

/**
 * What adding or changing a filter changes: the filter in the cached list (in its place, or last when new), and the
 * posts and notifications it applies to. Resolves once the shown ones are fetched again.
 */
export async function cacheSavedFilter(api: Api, queryClient: QueryClient, filter: KeywordFilter): Promise<void> {
  queryClient.setQueryData<KeywordFilterCollection>(keywordFiltersQueryOptions(api).queryKey, (data) => {
    if (data === undefined) return data;
    const known = data.items.some((item) => item.id === filter.id);
    return {
      items: known ? data.items.map((item) => (item.id === filter.id ? filter : item)) : [...data.items, filter],
    };
  });
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: keywordFiltersQueryOptions(api).queryKey }),
    refreshFiltered(queryClient),
  ]);
}

/** What deleting a filter changes: it leaves the cached list at once, and the posts it applied to are fetched again. */
export async function cacheDeletedFilter(api: Api, queryClient: QueryClient, filter: KeywordFilter): Promise<void> {
  queryClient.setQueryData<KeywordFilterCollection>(
    keywordFiltersQueryOptions(api).queryKey,
    (data) => data && { items: data.items.filter((item) => item.id !== filter.id) },
  );
  await refreshFiltered(queryClient);
}

/** Why saving a filter failed, by what the user can do about it. */
export type FilterFailure = "too-many-filters" | "too-many-words" | "other";

/** Classifies an error of saving a filter (api/openapi.yaml, createKeywordFilter). */
export function filterFailure(error: unknown): FilterFailure {
  if (!(error instanceof ApiError)) return "other";
  switch (error.type) {
    case "/api/v1/problems/too-many-keyword-filters":
      return "too-many-filters";
    case "/api/v1/problems/too-many-filter-keywords":
      return "too-many-words";
    default:
      return "other";
  }
}

/** The documentation site (app/docs), which serves each version of `my` under a path of its own. */
const docsOrigin = "https://mycroblog.dev";

/** The path prefix of each locale of the documentation site that is not its root locale `en` (app/docs/astro.config.ts). */
const docsLocales: Readonly<Record<string, string>> = {
  ja: "ja",
  "zh-CN": "zh-cn",
  "zh-TW": "zh-tw",
  es: "es",
  "pt-BR": "pt-br",
  de: "de",
  fr: "fr",
};

/**
 * The page of the user manual about how the words of keyword filters match, at the documentation site's version of
 * the server (`/vX.Y/` for a stable release vX.Y.Z, `/next/` for any other build, as internal/platform/docsite picks
 * it) and in the UI's locale when the site has it (English otherwise).
 */
export function wordMatchingManualUrl(serverVersion: string | undefined, locale: string): string {
  const release = /^v?(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.exec(serverVersion ?? "");
  const version = release === null ? "next" : `v${release[1] ?? ""}.${release[2] ?? ""}`;
  const prefix = docsLocales[locale];
  return `${docsOrigin}/${version}/${prefix === undefined ? "" : `${prefix}/`}guide/keyword-filters/#how-words-match`;
}
