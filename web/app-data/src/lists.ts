// SPDX-License-Identifier: AGPL-3.0-only
// The reader's lists (FR-211, ADR-0013 §10–§15): the lists they made, oldest first, and one of them; a list's members,
// a page at a time; the lists that hold an account; and the accounts the reader follows that a search finds, which
// alone may be members (§11). The writes make, rename and delete a list and put an account in it or take it out; each
// changes the cache at once (a list's name, its count, its members, the lists of the account), and the server's
// refusal at its limits (§14: 50 lists, 1,000 members) is told apart from other failures.
import { ProblemError, type components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import { relationshipQuery, type Account } from "./accounts";
import type { AppData } from "./app-data";
import { pagedQuery, usePagedList, type PagedList, type Pages } from "./paged";
import { useAppData } from "./provider";
import { listTimelineQuery } from "./timeline";

/** A list of the reader's: its name and how many accounts are in it. */
export type List = components["schemas"]["List"];

/** The reader's lists, oldest first. */
export function listsQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/lists");
}

/** The reader's list of `id`. */
export function listQuery({ queries }: AppData, id: string) {
  return queries.queryOptions("get", "/lists/{id}", { params: { path: { id } } });
}

/** What a screen of the reader's lists shows: the lists, whether they are being read or failed, and trying again. */
export type Lists = {
  readonly lists: readonly List[];
  readonly status: "pending" | "error" | "success";
  readonly refetch: () => void;
};

/** The reader's lists, empty until the server answered; nothing is asked while `enabled` is false. */
export function useLists(enabled = true): Lists {
  const query = useQuery({ ...listsQuery(useAppData()), enabled });
  return {
    lists: query.data?.items ?? [],
    status: query.status,
    refetch: () => {
      void query.refetch();
    },
  };
}

/** The members of the reader's list of `id`, a page at a time, in the order of their IDs. */
export function listMembersQuery({ api, queries }: AppData, id: string) {
  return pagedQuery<Account>(
    queries.queryOptions("get", "/lists/{id}/accounts", { params: { path: { id } } }).queryKey,
    async (cursor, signal) => {
      const { data } = await api.GET("/lists/{id}/accounts", {
        params: { path: { id }, query: cursor === undefined ? {} : { cursor } },
        signal,
      });
      return data;
    },
  );
}

/** The members of the reader's list of `id`, those added here first. */
export function useListMembers(id: string): PagedList<Account> {
  return usePagedList(listMembersQuery(useAppData(), id));
}

/** The reader's lists that hold the account of `id`. */
export function accountListsQuery({ queries }: AppData, id: string) {
  return queries.queryOptions("get", "/accounts/{id}/lists", { params: { path: { id } } });
}

/** The ids of the reader's lists that hold the account of `id`; undefined until the server answered, or unasked. */
export function useAccountLists(id: string, enabled: boolean): ReadonlySet<string> | undefined {
  const query = useQuery({ ...accountListsQuery(useAppData(), id), enabled });
  return query.data === undefined ? undefined : new Set(query.data.items.map((l) => l.id));
}

/** How many accounts a search for a list's members finds at most, before those the reader does not follow go. */
const findLimit = 20;

/** The accounts the reader follows among those a search for `q` finds, best match first. */
export function followedSearchQuery(data: AppData, q: string) {
  return {
    queryKey: ["app-data", "followed-search", q] as const,
    queryFn: async ({ signal }: { signal: AbortSignal }): Promise<readonly Account[]> => {
      const { data: found } = await data.api.GET("/accounts/search", {
        params: { query: { q, limit: findLimit } },
        signal,
      });
      const accounts = found?.items ?? [];
      if (accounts.length === 0) return [];
      const { data: relations } = await data.api.GET("/accounts/relationships", {
        params: { query: { id: accounts.map((a) => a.id) } },
        signal,
      });
      for (const r of relations?.items ?? [])
        data.queryClient.setQueryData(relationshipQuery(data, r.id).queryKey, { items: [r] });
      const followed = new Set((relations?.items ?? []).filter((r) => r.following).map((r) => r.id));
      return accounts.filter((a) => followed.has(a.id));
    },
    staleTime: 60_000,
    retry: false,
  };
}

/** What a search of the accounts the reader follows answered: undefined until it did (or unasked), and whether it failed. */
export type FollowedSearch = { readonly items: readonly Account[] | undefined; readonly failed: boolean };

/** The accounts the reader follows that `q` finds; nothing is asked for an empty `q`. */
export function useFollowedSearch(q: string): FollowedSearch {
  const query = useQuery({ ...followedSearchQuery(useAppData(), q), enabled: q.trim() !== "" });
  return { items: q.trim() === "" ? undefined : query.data, failed: query.isError };
}

// ---- The writes ------------------------------------------------------------------------------------------------------

/** A limit of ADR-0013 §14 the server answered: the reader has 50 lists, or the list 1,000 members. */
export type ListLimit = "lists" | "members";

/** The limit `error` is the server's refusal at, or undefined for any other failure. */
export function listLimitOf(error: unknown): ListLimit | undefined {
  if (!(error instanceof ProblemError) || error.status !== 422) return undefined;
  if (error.type === "/api/v1/problems/too-many-lists") return "lists";
  if (error.type === "/api/v1/problems/too-many-list-members") return "members";
  return undefined;
}

type ListCollection = { readonly items: readonly List[] };

/** Writes `change` over the list of `id` in the cached lists and the list's own; `undefined` takes it out. */
function cacheList(data: AppData, id: string, change: (list: List) => List | undefined): void {
  const { queryClient } = data;
  queryClient.setQueryData<ListCollection>(listsQuery(data).queryKey, (cached) =>
    cached === undefined
      ? cached
      : { ...cached, items: cached.items.flatMap((l) => (l.id === id ? (change(l) ?? []) : [l])) },
  );
  const own = queryClient.getQueryData<List>(listQuery(data, id).queryKey);
  if (own === undefined) return;
  const changed = change(own);
  if (changed === undefined) queryClient.removeQueries({ queryKey: listQuery(data, id).queryKey, exact: true });
  else queryClient.setQueryData(listQuery(data, id).queryKey, changed);
}

/** Makes a list of `name`, last of the reader's lists; it rejects with the server's problem. */
async function createList(data: AppData, name: string): Promise<List> {
  const { data: made } = await data.api.POST("/lists", { body: { name } });
  if (made === undefined) throw new Error("app-data: the new list answered without the list");
  data.queryClient.setQueryData<ListCollection>(listsQuery(data).queryKey, (cached) =>
    cached === undefined ? { items: [made] } : { ...cached, items: [...cached.items, made] },
  );
  data.queryClient.setQueryData(listQuery(data, made.id).queryKey, made);
  return made;
}

/** Renames the list of `id`; it rejects with the server's problem. */
async function renameList(data: AppData, id: string, name: string): Promise<void> {
  const { data: renamed } = await data.api.PATCH("/lists/{id}", { params: { path: { id } }, body: { name } });
  if (renamed !== undefined) cacheList(data, id, () => renamed);
}

/** Deletes the list of `id`; it rejects with the server's problem. */
async function deleteList(data: AppData, id: string): Promise<void> {
  await data.api.DELETE("/lists/{id}", { params: { path: { id } } });
  cacheList(data, id, () => undefined);
  data.queryClient.removeQueries({ queryKey: listMembersQuery(data, id).queryKey, exact: true });
  void data.queryClient.invalidateQueries({ queryKey: ["get", "/accounts/{id}/lists"] });
}

/** Whether the cache knows the account of `accountId` to be in the list of `id`: undefined when it does not know. */
function knownMember(data: AppData, id: string, accountId: string): boolean | undefined {
  const lists = data.queryClient.getQueryData<ListCollection>(accountListsQuery(data, accountId).queryKey);
  if (lists !== undefined) return lists.items.some((l) => l.id === id);
  const pages = data.queryClient.getQueryData<Pages<Account>>(listMembersQuery(data, id).queryKey);
  if (pages?.pages.some((p) => p.items.some((a) => a.id === accountId)) === true) return true;
  return pages !== undefined && pages.pages.at(-1)?.next_cursor === undefined ? false : undefined;
}

/**
 * Puts the account in the list of `id` (`add`) or takes it out, and resolves once the server answered: the members,
 * the list's count and the lists of the account change at once; the list's timeline is read again. It rejects with the
 * server's problem.
 */
async function changeMember(data: AppData, id: string, account: Account, add: boolean): Promise<void> {
  const { queryClient } = data;
  const path = { params: { path: { id, account_id: account.id } } };
  if (add) await data.api.POST("/lists/{id}/accounts/{account_id}", path);
  else await data.api.DELETE("/lists/{id}/accounts/{account_id}", path);
  const was = knownMember(data, id, account.id);
  if (was !== add) cacheList(data, id, (l) => ({ ...l, members_count: Math.max(0, l.members_count + (add ? 1 : -1)) }));
  queryClient.setQueryData<Pages<Account>>(listMembersQuery(data, id).queryKey, (cached) => {
    if (cached === undefined) return cached;
    const pages = cached.pages.map((p) => ({ ...p, items: p.items.filter((a) => a.id !== account.id) }));
    const [first, ...rest] = pages;
    return add && first !== undefined
      ? { ...cached, pages: [{ ...first, items: [account, ...first.items] }, ...rest] }
      : { ...cached, pages };
  });
  const list =
    queryClient.getQueryData<List>(listQuery(data, id).queryKey) ??
    queryClient.getQueryData<ListCollection>(listsQuery(data).queryKey)?.items.find((l) => l.id === id);
  queryClient.setQueryData<ListCollection>(accountListsQuery(data, account.id).queryKey, (cached) => {
    if (cached === undefined) return cached;
    const others = cached.items.filter((l) => l.id !== id);
    return { ...cached, items: add && list !== undefined ? [...others, list] : others };
  });
  void queryClient.invalidateQueries({ queryKey: listTimelineQuery(data, id).queryKey });
}

/** The writes of the reader's lists, each rejecting with the server's problem (`listLimitOf` tells its limits). */
export type ListWrites = {
  readonly create: (name: string) => Promise<List>;
  readonly rename: (id: string, name: string) => Promise<void>;
  readonly remove: (id: string) => Promise<void>;
  /** Puts `account` in the list of `id` (`add`), or takes it out. */
  readonly member: (id: string, account: Account, add: boolean) => Promise<void>;
};

/** The writes of the reader's lists, from a screen. */
export function useListWrites(): ListWrites {
  const data = useAppData();
  return {
    create: (name) => createList(data, name),
    rename: (id, name) => renameList(data, id, name),
    remove: (id) => deleteList(data, id),
    member: (id, account, add) => changeMember(data, id, account, add),
  };
}

/** What a list's screen shows of the list: the list once read, why it could not be, and reading it again. */
export type OneList = {
  /** The list, undefined until the server answered with it. */
  readonly list: List | undefined;
  /** `pending` until the server answered, `gone` when it has no such list (deleted), `error` when reading it failed. */
  readonly status: "pending" | "gone" | "error" | "success";
  /** Reads the list again: the reader's try again after it failed. */
  readonly refetch: () => void;
};

/** The reader's list of `id`. */
export function useList(id: string): OneList {
  const query = useQuery(listQuery(useAppData(), id));
  const { error } = query;
  return {
    list: query.data,
    status:
      query.status === "error"
        ? error instanceof ProblemError && error.status === 404
          ? "gone"
          : "error"
        : query.status,
    refetch: () => {
      void query.refetch();
    },
  };
}
