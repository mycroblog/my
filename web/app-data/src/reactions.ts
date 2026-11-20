// SPDX-License-Identifier: AGPL-3.0-only
// What the emoji reactions read (FR-116, ADR-0051, ADR-0091 §1): the custom emoji of the server that the picker offers
// (`GET /custom_emojis`, one list, not paged), and who reacted to a status, newest first (`GET
// /statuses/{id}/reactions`), with every emoji or with one. A reaction is set or taken back as a mark (interactions.ts).
import type { components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { pagedQuery, usePagedList, type PagedList } from "./paged";
import { useAppData } from "./provider";

/** A custom emoji of the server that the picker offers. */
export type PickerCustomEmoji = components["schemas"]["PickerCustomEmoji"];

/** One reaction to a status, with the account that reacted. */
export type StatusReactor = components["schemas"]["StatusReactor"];

/** The custom emoji of the server that the picker offers, by category (those without one first) and shortcode. */
export function customEmojisQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/custom_emojis");
}

/**
 * Reads the picker's custom emoji again after a write that changes them, past the browser's cache: the server answers
 * the list `private, max-age=60`, which a plain read again would take from the cache for up to a minute.
 */
export async function refreshCustomEmojis(data: AppData): Promise<void> {
  const { queryKey } = customEmojisQuery(data);
  await data.queryClient
    .query({
      queryKey,
      queryFn: async ({ signal }) => (await data.api.GET("/custom_emojis", { cache: "no-cache", signal })).data,
      staleTime: 0,
    })
    .catch(() => undefined);
}

/** The custom emoji of the picker; empty until the server answered, and when it failed. */
export function useCustomEmojis(): readonly PickerCustomEmoji[] {
  return useQuery(customEmojisQuery(useAppData())).data?.items ?? [];
}

/** How many reactions a page holds: the server's most (Limit), so that the rows under a post see the most faces. */
const reactorsLimit = 40;

/** The reactions to the status of `id`, newest first; with `emoji` (a reaction's `name`), those with it alone. */
export function statusReactorsQuery({ api, queries }: AppData, id: string, emoji?: string) {
  const query = { limit: reactorsLimit, ...(emoji === undefined ? {} : { emoji }) };
  return pagedQuery<StatusReactor>(
    queries.queryOptions("get", "/statuses/{id}/reactions", { params: { path: { id }, query } }).queryKey,
    async (cursor, signal) => {
      const { data } = await api.GET("/statuses/{id}/reactions", {
        params: { path: { id }, query: cursor === undefined ? query : { ...query, cursor } },
        signal,
      });
      return data;
    },
  );
}

/** The reactions to the status of `id`, with `emoji` alone if given; nothing is fetched while `enabled` is false. */
export function useStatusReactors(id: string, emoji?: string, enabled = true): PagedList<StatusReactor> {
  return usePagedList(statusReactorsQuery(useAppData(), id, emoji), enabled);
}
