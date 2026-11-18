// SPDX-License-Identifier: AGPL-3.0-only
// The accounts the viewer blocks and the accounts they mute (FR-304), each list a page at a time. The writes that put an
// account in them or take it out are the relationship's (changeRelationship in accounts.ts), which marks these lists;
// the domains the viewer mutes are domains.ts's.
import type { components } from "@mycroblog/mykit";
import type { Account } from "./accounts";
import type { AppData } from "./app-data";
import { pagedQuery, usePagedList, type PagedList } from "./paged";
import { useAppData } from "./provider";

/** An account the viewer mutes, whether the mute also hides its notifications, and when it ends. */
export type MutedAccount = components["schemas"]["MutedAccount"];

/** The accounts the viewer blocks, a page at a time. */
export function blocksQuery({ api, queries }: AppData) {
  return pagedQuery<Account>(
    queries.queryOptions("get", "/blocks", { params: { query: {} } }).queryKey,
    async (cursor, signal) => {
      const { data } = await api.GET("/blocks", { params: { query: cursor === undefined ? {} : { cursor } }, signal });
      return data;
    },
  );
}

/** The accounts the viewer mutes, a page at a time. */
export function mutesQuery({ api, queries }: AppData) {
  return pagedQuery<MutedAccount>(
    queries.queryOptions("get", "/mutes", { params: { query: {} } }).queryKey,
    async (cursor, signal) => {
      const { data } = await api.GET("/mutes", { params: { query: cursor === undefined ? {} : { cursor } }, signal });
      return data;
    },
  );
}

/** The accounts the viewer blocks, for a screen. */
export function useBlocks(): PagedList<Account> {
  return usePagedList(blocksQuery(useAppData()));
}

/** The accounts the viewer mutes, for a screen. */
export function useMutes(): PagedList<MutedAccount> {
  return usePagedList(mutesQuery(useAppData()));
}
