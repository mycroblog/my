// SPDX-License-Identifier: AGPL-3.0-only
// What the search screen asks the server for (FR-305–FR-307, FR-309, ADR-0092 §1, ADR-0149 §9): the accounts whose
// handle or a word of whose name starts with the words, and this server's accounts that contain them
// (`GET /accounts/search`), each with the reader's relationship read in the same turn so that its follow shows at once;
// the hashtags that start with them (`GET /tags/search`); a remote account named by `@user@host` or by its URL, looked up
// and fetched from its server (`GET /accounts/lookup`, `resolve`); and the posts that contain every word, a page at a
// time (`GET /statuses/search`), only while the server has a search engine (`Instance.full_text_search`). An answer is
// fresh for a minute; a failure is not retried, and the screen says so with Try again.
import { ProblemError, type components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { Account } from "./accounts";
import type { AppData } from "./app-data";
import { readRelationships } from "./follows";
import { instanceQuery } from "./instance";
import { pagedQuery, usePagedList, type PagedList } from "./paged";
import { useAppData } from "./provider";
import type { Status } from "./statuses";

/** A hashtag the search found, with how many posts carry it. */
type FoundTag = components["schemas"]["TagSuggestion"];

/** How many accounts and hashtags a search asks for: the most the server gives. */
const searchLimit = 20;

/** How long an answer is fresh. */
const staleTime = 60_000;

/**
 * The handle or the address of a remote account in `q`, as the lookup takes it: `user@host` (a leading `@` dropped)
 * with a dot in its host, or an `https` or `http` URL; undefined for any other words.
 */
export function remoteOf(q: string): string | undefined {
  const words = q.trim();
  if (/^https?:\/\/\S+$/iu.test(words)) return words;
  const handle = /^@?([^\s@]+@[^\s@]+\.[^\s@]+)$/u.exec(words);
  return handle?.[1];
}

/** The accounts the words find, with the reader's relationships with them in the cache. */
export function accountResultsQuery(data: AppData, q: string) {
  const init = { params: { query: { q, limit: searchLimit } } };
  return {
    queryKey: [...data.queries.queryOptions("get", "/accounts/search", init).queryKey, "relationships"] as const,
    queryFn: async ({ signal }: { signal: AbortSignal }): Promise<readonly Account[]> => {
      const { data: found } = await data.api.GET("/accounts/search", { ...init, signal });
      const accounts = found?.items ?? [];
      await readRelationships(
        data,
        accounts.map((a) => a.id),
        signal,
      );
      return accounts;
    },
    staleTime,
    retry: false,
  };
}

/** The hashtags that start with the words. */
export function tagResultsQuery({ queries }: AppData, q: string) {
  return {
    ...queries.queryOptions("get", "/tags/search", {
      params: { query: { q: q.replace(/^#/u, ""), limit: searchLimit } },
    }),
    staleTime,
    retry: false,
  };
}

/** The remote account of `acct` (a handle or a URL), fetched from its server when this one does not know it yet. */
export function remoteAccountQuery(data: AppData, acct: string) {
  const init = { params: { query: { acct, resolve: true } } };
  return {
    queryKey: data.queries.queryOptions("get", "/accounts/lookup", init).queryKey,
    queryFn: async ({ signal }: { signal: AbortSignal }): Promise<Account | null> => {
      const found = await data.api.GET("/accounts/lookup", { ...init, signal }).catch((e: unknown) => {
        // An account that is not there is no failure: the search found none.
        if (e instanceof ProblemError && e.status === 404) return { data: null };
        throw e;
      });
      const account = found.data ?? null;
      if (account !== null) await readRelationships(data, [account.id], signal);
      return account;
    },
    staleTime,
    retry: false,
  };
}

/** The posts that contain every word, newest first, a page at a time. */
export function postResultsQuery({ api, queries }: AppData, q: string) {
  return pagedQuery<Status>(
    queries.queryOptions("get", "/statuses/search", { params: { query: { q } } }).queryKey,
    async (cursor, signal) => {
      const { data } = await api.GET("/statuses/search", {
        params: { query: cursor === undefined ? { q } : { q, cursor } },
        signal,
      });
      return data;
    },
  );
}

/** What a kind of result shows: its items once the server answered, whether it failed, and asking again. */
export type Found<T> = {
  readonly items: readonly T[];
  readonly status: "pending" | "error" | "success";
  readonly refetch: () => void;
};

/** What a search finds of its words. */
export type SearchResults = {
  /** The accounts, the remote one named by the words first once it was found. */
  readonly accounts: Found<Account>;
  readonly tags: Found<FoundTag>;
  /** The posts; undefined where the server has no search engine, which offers no search of posts at all. */
  readonly posts: PagedList<Status> | undefined;
  /** The remote account the words name (`@user@host`, a URL): undefined for other words. */
  readonly remote:
    { readonly acct: string; readonly status: "pending" | "error" | "success"; readonly found: boolean } | undefined;
};

const found = <T>(query: {
  data?: readonly T[] | undefined;
  status: "pending" | "error" | "success";
  refetch: () => unknown;
}): Found<T> => ({
  items: query.data ?? [],
  status: query.status,
  refetch: () => {
    void query.refetch();
  },
});

/** Whether the server offers the search of posts (`Instance.full_text_search`); undefined until it answered. */
export function usePostSearchOffered(): boolean | undefined {
  return useQuery(instanceQuery(useAppData())).data?.full_text_search.available;
}

/** What the words `q` find; nothing is asked for blank words. */
export function useSearch(q: string): SearchResults {
  const data = useAppData();
  const words = q.trim();
  const asked = words !== "";
  const acct = asked ? remoteOf(words) : undefined;
  const offered = usePostSearchOffered() === true;
  const accounts = useQuery({ ...accountResultsQuery(data, words), enabled: asked });
  const tags = useQuery({ ...tagResultsQuery(data, words), enabled: asked && acct === undefined });
  const remote = useQuery({ ...remoteAccountQuery(data, acct ?? ""), enabled: acct !== undefined });
  const posts = usePagedList(postResultsQuery(data, words), asked && offered && acct === undefined);
  const looked = remote.data ?? null;
  const accountItems =
    looked === null ? accounts.data : [looked, ...(accounts.data ?? []).filter((a) => a.id !== looked.id)];
  return {
    accounts: found({ ...accounts, data: accountItems }),
    tags: found({ ...tags, data: tags.data?.items }),
    posts: offered && acct === undefined ? posts : undefined,
    remote:
      acct === undefined
        ? undefined
        : { acct, status: remote.status, found: remote.data !== null && remote.data !== undefined },
  };
}
