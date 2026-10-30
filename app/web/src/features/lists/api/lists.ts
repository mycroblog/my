// SPDX-License-Identifier: AGPL-3.0-only
// The user's lists of the accounts they follow (ADR-0122): the lists, their members, the lists an account is in, and
// the followed accounts to add. Only the owner sees them. Every write says what it changes in the cache next to it
// (web-client/README.md S1, ADR-0045).
import { infiniteQueryOptions, type QueryClient, queryOptions } from "@tanstack/react-query";
import type { components } from "@mycroblog/mykit";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";

/** A list of the user, with its name and its number of members. */
export type List = components["schemas"]["List"];

/** The user's lists, as GET /lists answers them. */
type ListCollection = components["schemas"]["ListCollection"];

/** An account as the client API returns it. */
export type Account = components["schemas"]["Account"];

/** The most lists a user has (ADR-0122 §5). */
export const maxLists = 50;

/** The most members a list has (ADR-0122 §5). */
export const maxListMembers = 1000;

/** The most grapheme clusters of a list's name (ListRequest.name). */
export const listNameLimit = 50;

/** The most characters the name field takes (ListRequest.name `maxLength`). */
export const listNameInputLimit = 200;

/** How many grapheme clusters (what a reader sees as one character) `text` has. */
function graphemeCount(text: string): number {
  return Array.from(new Intl.Segmenter(undefined, { granularity: "grapheme" }).segment(text)).length;
}

/** Why a name (trimmed) is not one the server takes, or undefined when it is: 1 to 50 grapheme clusters on one line. */
export function listNameProblem(name: string): "empty" | "too-long" | "invalid" | undefined {
  if (name === "") return "empty";
  if (/\p{Cc}/u.test(name)) return "invalid";
  if (graphemeCount(name) > listNameLimit) return "too-long";
  return undefined;
}

/** The user's lists, oldest first (GET /lists). */
export function listsQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/lists");
}

/** One list of the user (GET /lists/{id}); another user's list is a 404. */
export function listQueryOptions(api: Api, id: string) {
  return api.query.queryOptions("get", "/lists/{id}", { params: { path: { id } } });
}

/** The user's lists that the account `id` is in (GET /accounts/{id}/lists). */
export function accountListsQueryOptions(api: Api, id: string) {
  return api.query.queryOptions("get", "/accounts/{id}/lists", { params: { path: { id } } });
}

/** The members of a list a page at a time, in the order of their IDs (GET /lists/{id}/accounts). */
export function listMembersQueryOptions(api: Api, id: string) {
  return infiniteQueryOptions({
    queryKey: ["get", "/lists/{id}/accounts", { params: { path: { id } } }, "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const { data, error } = await api.fetch.GET("/lists/{id}/accounts", {
        params: { path: { id }, query: pageParam === undefined ? {} : { cursor: pageParam } },
        signal,
      });
      // The client's middleware throws an ApiError for every error status (lib/api/client.ts).
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (lastPage) => lastPage.next_cursor,
    retry: false,
  });
}

/** The most accounts the search for followed accounts asks for (SearchLimit's maximum). */
const followingSearchLimit = 20;

/**
 * The accounts that the user actively follows whose names match `q`, best match first: the name search
 * (GET /accounts/search, which ranks followed accounts first), kept to the accounts whose relationship says the user
 * follows them (GET /accounts/relationships). Requested follows and other accounts are left out (ADR-0122 §2).
 */
export function followingSearchQueryOptions(api: Api, q: string) {
  return queryOptions({
    queryKey: ["get", "/accounts/search", { params: { query: { q, limit: followingSearchLimit } } }, "following"],
    queryFn: async ({ signal }): Promise<Account[]> => {
      const found = await api.fetch.GET("/accounts/search", {
        params: { query: { q, limit: followingSearchLimit } },
        signal,
      });
      if (found.data === undefined) throw new ApiError(found.error);
      if (found.data.length === 0) return [];
      const relationships = await api.fetch.GET("/accounts/relationships", {
        params: { query: { id: found.data.map((account) => account.id) } },
        signal,
      });
      if (relationships.data === undefined) throw new ApiError(relationships.error);
      const followed = new Set(relationships.data.filter((r) => r.following).map((r) => r.id));
      return found.data.filter((account) => followed.has(account.id));
    },
    retry: false,
  });
}

/** Creates a list (POST /lists), as the options of useMutation. */
export function createListMutation(api: Api) {
  return {
    mutationKey: ["post", "/lists"] as const,
    mutationFn: async (name: string): Promise<List> => {
      const { data, error } = await api.fetch.POST("/lists", { body: { name } });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Renames a list (PATCH /lists/{id}), as the options of useMutation. */
export function renameListMutation(api: Api) {
  return {
    mutationKey: ["patch", "/lists/{id}"] as const,
    mutationFn: async ({ id, name }: { id: string; name: string }): Promise<List> => {
      const { data, error } = await api.fetch.PATCH("/lists/{id}", { params: { path: { id } }, body: { name } });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Deletes a list (DELETE /lists/{id}), as the options of useMutation. */
export function deleteListMutation(api: Api) {
  return {
    mutationKey: ["delete", "/lists/{id}"] as const,
    mutationFn: async (list: List): Promise<List> => {
      const { error } = await api.fetch.DELETE("/lists/{id}", { params: { path: { id: list.id } } });
      if (error !== undefined) throw new ApiError(error);
      return list;
    },
  };
}

/** A member to add or remove: the list and the account. */
export type MemberChange = { list: List; account: Account };

/** Adds a followed account to a list (POST /lists/{id}/accounts/{account_id}), as the options of useMutation. */
export function addListMemberMutation(api: Api) {
  return {
    mutationKey: ["post", "/lists/{id}/accounts/{account_id}"] as const,
    mutationFn: async (change: MemberChange): Promise<MemberChange> => {
      const { error } = await api.fetch.POST("/lists/{id}/accounts/{account_id}", {
        params: { path: { id: change.list.id, account_id: change.account.id } },
      });
      if (error !== undefined) throw new ApiError(error);
      return change;
    },
  };
}

/** Takes an account out of a list (DELETE /lists/{id}/accounts/{account_id}), as the options of useMutation. */
export function removeListMemberMutation(api: Api) {
  return {
    mutationKey: ["delete", "/lists/{id}/accounts/{account_id}"] as const,
    mutationFn: async (change: MemberChange): Promise<MemberChange> => {
      const { error } = await api.fetch.DELETE("/lists/{id}/accounts/{account_id}", {
        params: { path: { id: change.list.id, account_id: change.account.id } },
      });
      if (error !== undefined) throw new ApiError(error);
      return change;
    },
  };
}

/** Puts a list the server answered into the cache: the collection (in its place, or last when new) and the list. */
function cacheList(api: Api, queryClient: QueryClient, list: List): void {
  queryClient.setQueryData<ListCollection>(listsQueryOptions(api).queryKey, (data) => {
    if (data === undefined) return data;
    const known = data.items.some((item) => item.id === list.id);
    return { items: known ? data.items.map((item) => (item.id === list.id ? list : item)) : [...data.items, list] };
  });
  queryClient.setQueryData<List>(listQueryOptions(api, list.id).queryKey, list);
}

/**
 * What creating or renaming a list changes: the collection and the list, from the answer, and the lists of every
 * account (they hold the list's name). Resolves once the shown ones are fetched again.
 */
export async function cacheSavedList(api: Api, queryClient: QueryClient, list: List): Promise<void> {
  cacheList(api, queryClient, list);
  await queryClient.invalidateQueries({ queryKey: ["get", "/accounts/{id}/lists"] });
}

/**
 * What deleting a list changes: it leaves the collection, and its own queries (the list, its members, its timeline)
 * are dropped, since they would answer 404 now. The lists of every account are marked stale.
 */
export async function cacheDeletedList(api: Api, queryClient: QueryClient, list: List): Promise<void> {
  queryClient.setQueryData<ListCollection>(
    listsQueryOptions(api).queryKey,
    (data) => data && { items: data.items.filter((item) => item.id !== list.id) },
  );
  const path = { params: { path: { id: list.id } } };
  queryClient.removeQueries({ queryKey: ["get", "/lists/{id}", path] });
  queryClient.removeQueries({ queryKey: ["get", "/lists/{id}/accounts", path] });
  queryClient.removeQueries({ queryKey: ["get", "/timelines/list/{id}", path] });
  await queryClient.invalidateQueries({ queryKey: ["get", "/accounts/{id}/lists"] });
}

/**
 * What adding or removing a member changes: the list's count (`members_count`, by `delta`) in the collection and the
 * list, the account's lists, the list's members and its timeline (the member's posts join or leave it). Resolves once
 * the shown ones are fetched again.
 */
export async function cacheMemberChange(
  api: Api,
  queryClient: QueryClient,
  { list, account }: MemberChange,
  delta: 1 | -1,
): Promise<void> {
  const counted = (item: List): List =>
    item.id === list.id ? { ...item, members_count: Math.max(0, item.members_count + delta) } : item;
  queryClient.setQueryData<ListCollection>(
    listsQueryOptions(api).queryKey,
    (data) => data && { items: data.items.map(counted) },
  );
  queryClient.setQueryData<List>(listQueryOptions(api, list.id).queryKey, (data) => data && counted(data));
  const path = { params: { path: { id: list.id } } };
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: accountListsQueryOptions(api, account.id).queryKey }),
    queryClient.invalidateQueries({ queryKey: ["get", "/lists/{id}/accounts", path] }),
    queryClient.invalidateQueries({ queryKey: ["get", "/timelines/list/{id}", path] }),
  ]);
}

/** Why a change of lists failed, by what the user can do about it. */
export type ListFailure = "too-many-lists" | "too-many-members" | "not-followed" | "not-found" | "other";

/** Classifies an error of a list operation (api/openapi.yaml, Problem). */
export function listFailure(error: unknown): ListFailure {
  if (!(error instanceof ApiError)) return "other";
  if (error.status === 404) return "not-found";
  switch (error.type) {
    case "/api/v1/problems/too-many-lists":
      return "too-many-lists";
    case "/api/v1/problems/too-many-list-members":
      return "too-many-members";
    case "/api/v1/problems/not-followed":
      return "not-followed";
    default:
      return "other";
  }
}
