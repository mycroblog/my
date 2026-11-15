// SPDX-License-Identifier: AGPL-3.0-only
// What the composer's suggestions ask the server for (ADR-0092 §1): the accounts whose handle or a word of whose name
// starts with what follows `@` (`GET /accounts/search`; the server puts the followed first and leaves the blocked and
// muted out), and the hashtags that start with what follows `#` (`GET /tags/search`, the most used first). Each answer
// is fresh for a minute, so that a word typed again after it was erased asks nothing; a newer word cancels the request
// of the older one (the query's signal). A failure, a 429 among them, is not retried: the suggestions just close. When
// what follows `@` is a whole `user@domain` that no account found matches, its account is looked up once, fetched
// from its server, and put first.
import type { components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import { accountQuery, type Account } from "./accounts";
import type { AppData } from "./app-data";
import { useAppData } from "./provider";

/** A hashtag the search found, with how many posts carry it. */
export type TagSuggestion = components["schemas"]["TagSuggestion"];

/** How many suggestions a search asks for: as many as the list shows. */
const suggestionLimit = 8;

/** How long an answer is fresh: the word typed again within it asks nothing (ADR-0092 §1). */
const suggestionStaleTime = 60_000;

/** The `q` of a search's query key (`[method, path, { params: { query: { q } } }]`). */
function qOf(key: readonly unknown[] | undefined): string | undefined {
  const init = key?.[2] as { params?: { query?: { q?: unknown } } } | undefined;
  const q = init?.params?.query?.q;
  return typeof q === "string" ? q : undefined;
}

/**
 * While the answer for `q` is asked, the answer for the same word typed shorter or longer stands in for it, so that the
 * list does not blink between keystrokes; another word's never does.
 */
const sameWord =
  (q: string) =>
  <T>(previous: T | undefined, query: { queryKey: readonly unknown[] } | undefined): T | undefined => {
    const before = qOf(query?.queryKey)?.toLowerCase();
    const now = q.toLowerCase();
    return before !== undefined && (now.startsWith(before) || before.startsWith(now)) ? previous : undefined;
  };

/** The accounts that start with `q` (without its `@`). */
export function accountSearchQuery({ queries }: AppData, q: string) {
  return {
    ...queries.queryOptions("get", "/accounts/search", { params: { query: { q, limit: suggestionLimit } } }),
    staleTime: suggestionStaleTime,
    retry: false,
    placeholderData: sameWord(q),
  };
}

/** The hashtags that start with `q` (without its `#`). */
export function tagSearchQuery({ queries }: AppData, q: string) {
  return {
    ...queries.queryOptions("get", "/tags/search", { params: { query: { q, limit: suggestionLimit } } }),
    staleTime: suggestionStaleTime,
    retry: false,
    placeholderData: sameWord(q),
  };
}

/** A whole handle of another server: `user@domain`, its domain with a dot and ending in a letter or a digit. */
const wholeHandle = /^[A-Za-z0-9_]+@(?:[A-Za-z0-9-]+\.)+[A-Za-z0-9]+$/u;

/** What a search of suggestions answered: undefined until it did; whether it failed. */
export type Searched<T> = { items: readonly T[] | undefined; failed: boolean };

/**
 * The accounts for what follows `@` (undefined: nothing is asked), best first, with the account of a whole
 * `user@domain` looked up first when the search found no account with that handle.
 */
export function useAccountSearch(q: string | undefined): Searched<Account> {
  const data = useAppData();
  const search = useQuery({ ...accountSearchQuery(data, q ?? ""), enabled: q !== undefined && q !== "" });
  const found = search.data?.items;
  const lookFor =
    q !== undefined &&
    wholeHandle.test(q) &&
    found !== undefined &&
    !search.isPlaceholderData &&
    !found.some((a) => a.acct.toLowerCase() === q.toLowerCase());
  const lookup = useQuery({
    ...accountQuery(data, q ?? ""),
    enabled: lookFor,
    staleTime: suggestionStaleTime,
    retry: false,
  });
  if (q === undefined || search.isError) return { items: undefined, failed: search.isError };
  if (found === undefined) return { items: undefined, failed: false };
  // The lookup comes after the search; while it is asked, or when it found nothing, the search's accounts stand.
  const looked = lookFor ? lookup.data : undefined;
  return { items: looked === undefined ? found : [looked, ...found], failed: false };
}

/** The hashtags for what follows `#` (undefined: nothing is asked), the most used first. */
export function useTagSearch(q: string | undefined): Searched<TagSuggestion> {
  const search = useQuery({ ...tagSearchQuery(useAppData(), q ?? ""), enabled: q !== undefined && q !== "" });
  if (q === undefined) return { items: undefined, failed: false };
  return { items: search.data?.items, failed: search.isError };
}
