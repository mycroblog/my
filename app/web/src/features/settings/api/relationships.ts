// SPDX-License-Identifier: AGPL-3.0-only
// The lists of the accounts the viewer blocks, mutes, or has follow requests from (FR-302, FR-303), as
// infinite queries over the API's keyset pagination: each page's `next_cursor` is the `cursor` of the next.
// openapi-react-query has no infiniteQueryOptions, so these are TanStack Query's, on the same client.
// Their keys start like the client's own ([method, path]), so invalidating ["get", path] reaches them.
import { infiniteQueryOptions, type QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";
import { refreshRelationship } from "../../profile";

type Schemas = components["schemas"];

export type Account = Schemas["Account"];
export type MutedAccount = Schemas["MutedAccount"];
export type Relationship = Schemas["Relationship"];

/** The page size of the lists. */
const pageSize = 40;

/** The paths of the lists, the first part of their query keys after the method. */
const listPaths = { blocks: "/blocks", mutes: "/mutes", followRequests: "/follow_requests" } as const;

/** The accounts the viewer blocks, newest first (GET /blocks). */
export function blocksQueryOptions(api: Api) {
  return infiniteQueryOptions({
    queryKey: ["get", listPaths.blocks, "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const { data, error } = await api.fetch.GET("/blocks", { params: { query: pageQuery(pageParam) }, signal });
      // The middleware throws an ApiError for every error status, so this is only for the types.
      if (data === undefined) throw new Error(error.title);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** The accounts the viewer mutes, with each mute's settings, newest first (GET /mutes). */
export function mutesQueryOptions(api: Api) {
  return infiniteQueryOptions({
    queryKey: ["get", listPaths.mutes, "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const { data, error } = await api.fetch.GET("/mutes", { params: { query: pageQuery(pageParam) }, signal });
      // The middleware throws an ApiError for every error status, so this is only for the types.
      if (data === undefined) throw new Error(error.title);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** The accounts whose follow requests await the viewer's approval, newest first (GET /follow_requests). */
export function followRequestsQueryOptions(api: Api) {
  return infiniteQueryOptions({
    queryKey: ["get", listPaths.followRequests, "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const { data, error } = await api.fetch.GET("/follow_requests", { params: { query: pageQuery(pageParam) }, signal });
      // The middleware throws an ApiError for every error status, so this is only for the types.
      if (data === undefined) throw new Error(error.title);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** The actions on one account of the lists, each on the account's ID. */
type AccountActionPath = "/accounts/{id}/unblock" | "/accounts/{id}/unmute" | "/follow_requests/{id}/authorize" | "/follow_requests/{id}/reject";

/**
 * An action on the account with the given ID (POST `path`), as the options of TanStack Query's useMutation
 * (updatePreferencesMutation in me.ts says why not the client's own).
 */
export function accountActionMutation(api: Api, path: AccountActionPath) {
  return {
    mutationKey: ["post", path] as const,
    mutationFn: async (id: string) => {
      const { data, error } = await api.fetch.POST(path, { params: { path: { id } } });
      // The middleware throws an ApiError for every error status, so this is only for the types.
      if (data === undefined) throw new Error(error.title);
      return data;
    },
  };
}

/** The changes that the lists make, each to the viewer's relationship with one account of a list. */
export type ListChange = "unblock" | "unmute" | "accept" | "reject";

/**
 * Takes the account `id` off the cached list that `change` acts on: the blocks, the mutes, or the follow
 * requests. The screen moves focus to the next row once it is gone.
 */
export function removeFromList(queryClient: QueryClient, api: Api, change: ListChange, id: string): void {
  switch (change) {
    case "unblock":
      queryClient.setQueryData(blocksQueryOptions(api).queryKey, (data) =>
        data && { ...data, pages: data.pages.map((page) => ({ ...page, accounts: page.accounts.filter((account) => account.id !== id) })) },
      );
      return;
    case "unmute":
      queryClient.setQueryData(mutesQueryOptions(api).queryKey, (data) =>
        data && { ...data, pages: data.pages.map((page) => ({ ...page, mutes: page.mutes.filter((mute) => mute.account.id !== id) })) },
      );
      return;
    case "accept":
    case "reject":
      queryClient.setQueryData(followRequestsQueryOptions(api).queryKey, (data) =>
        data && { ...data, pages: data.pages.map((page) => ({ ...page, accounts: page.accounts.filter((account) => account.id !== id) })) },
      );
  }
}

/**
 * What a change from the lists changes elsewhere (web-client/README.md S1, ADR-0045): the relationship as the server
 * answered it goes into every cached answer of GET /accounts/relationships that holds the account (the
 * profile's buttons, the new-post form's mentions), and an unblock, an unmute or an accepted request fetches
 * again what refreshRelationship names (the follow counts and lists, the lists of posts). A rejected request
 * changes no list and no count. Resolves once that is done; the list itself is left to removeFromList.
 */
export async function cacheListChange(queryClient: QueryClient, change: ListChange, relationship: Relationship, viewerId: string): Promise<void> {
  queryClient.setQueriesData<Relationship[]>(
    { predicate: (query) => query.queryKey[0] === "get" && query.queryKey[1] === "/accounts/relationships" && query.state.data !== undefined },
    (data) => (data?.some((item) => item.id === relationship.id) ? data.map((item) => (item.id === relationship.id ? relationship : item)) : data),
  );
  if (change !== "reject") await refreshRelationship(queryClient, change, relationship.id, viewerId);
}

function pageQuery(cursor: string | undefined): { limit: number; cursor?: string } {
  return cursor === undefined ? { limit: pageSize } : { limit: pageSize, cursor };
}
