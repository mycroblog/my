// SPDX-License-Identifier: AGPL-3.0-only
// The custom emoji a holder of `create_custom_emojis` asks for and keeps (FR-915, ADR-0070 §2–§4): the emoji they
// made, newest first, with what they used of their monthly limit (the approved and pending requests to create one
// this UTC month, the limit, none for no limit, and when the count resets); their requests, newest first, in every
// state; requesting a new emoji from an image uploaded for an emoji, withdrawing a pending request, and changing the
// category, the aliases and the place in the picker of one of theirs, or deleting it (which gives no place back).
// Every write reads again what it changes: their lists, the picker every reader reads, and the administrators' lists.
import type { components } from "@mycroblog/mykit";
import { useEffect } from "react";
import type { AppData } from "./app-data";
import { uploadEmojiImage, type EmojiSettings } from "./admin-custom-emojis";
import { pagedQuery, usePagedList, usePagedListOf, type PagedList } from "./paged";
import { useAppData } from "./provider";
import { refreshCustomEmojis } from "./reactions";
import { readOf, type ScreenRead } from "./reads";

/** A local custom emoji the reader made. */
export type OwnCustomEmoji = components["schemas"]["OwnCustomEmoji"];
/** A request of the reader's for a custom emoji. */
export type OwnCustomEmojiRequest = components["schemas"]["CustomEmojiRequest"];

/** A page of the reader's own emoji, with what they used of their monthly limit. */
type OwnEmojiPage = components["schemas"]["OwnCustomEmojiPage"];

/** The reader's own emoji, newest first, a page at a time; each page says what they used of their monthly limit. */
export function ownEmojisQuery({ api, queries }: AppData) {
  return pagedQuery<OwnCustomEmoji, OwnEmojiPage>(
    queries.queryOptions("get", "/custom_emojis/mine").queryKey,
    async (cursor, signal) =>
      (await api.GET("/custom_emojis/mine", { params: { query: cursor === undefined ? {} : { cursor } }, signal }))
        .data,
  );
}

/** The reader's requests, newest first, a page at a time. */
export function ownEmojiRequestsQuery({ api, queries }: AppData) {
  return pagedQuery<OwnCustomEmojiRequest>(
    queries.queryOptions("get", "/custom_emoji_requests").queryKey,
    async (cursor, signal) =>
      (await api.GET("/custom_emoji_requests", { params: { query: cursor === undefined ? {} : { cursor } }, signal }))
        .data,
  );
}

/** What the reader used of their monthly limit. */
export type EmojiQuota = {
  /** The approved and pending requests to create an emoji they submitted this month. */
  readonly used: number;
  /** The part of `used` that pending requests take. */
  readonly pending: number;
  /** How many they may submit in a month; undefined for no limit. */
  readonly limit: number | undefined;
  /** When the count resets, the first day of the next month at 00:00 UTC. */
  readonly resetsAt: string;
};

/** A list of the reader's that has more pages than it shows: the way to the next one. */
export type OwnMore = {
  readonly hasNext: boolean;
  readonly fetching: boolean;
  /** The last next page failed; the pages before it stay. */
  readonly failed: boolean;
  readonly next: () => void;
};

const moreOf = (list: PagedList<unknown>): OwnMore => ({
  hasNext: list.hasNextPage,
  fetching: list.isFetchingNextPage,
  failed: list.failedNextPage,
  next: list.fetchNextPage,
});

/** Where a paged list's first read is, as a screen's reads are told (`readOf`). */
const firstRead = (list: PagedList<unknown>, read: boolean) => ({
  data: read ? list.items : undefined,
  status: list.status,
  isFetching: list.status === "pending",
  refetch: list.refetch,
});

/**
 * The reader's emoji and their requests, the pages read so far of each with the way to the next, and what they used of
 * their limit (the newest page's), once the first page of both is read.
 */
export function useOwnEmojis(): ScreenRead & {
  readonly emojis: readonly OwnCustomEmoji[] | undefined;
  readonly requests: readonly OwnCustomEmojiRequest[] | undefined;
  readonly quota: EmojiQuota | undefined;
  readonly moreEmojis: OwnMore;
  readonly moreRequests: OwnMore;
} {
  const data = useAppData();
  const mine = usePagedListOf<OwnCustomEmoji, OwnEmojiPage>(ownEmojisQuery(data));
  const requests = usePagedList<OwnCustomEmojiRequest>(ownEmojiRequestsQuery(data));
  const page = mine.firstPage;
  const mineRead = page !== undefined;
  const requestsRead = requests.status === "success" || requests.items.length > 0;
  return {
    ...readOf([firstRead(mine, mineRead), firstRead(requests, requestsRead)]),
    emojis: mineRead ? mine.items : undefined,
    requests: requestsRead ? requests.items : undefined,
    quota:
      page === undefined
        ? undefined
        : { used: page.used, pending: page.pending, limit: page.limit, resetsAt: page.resets_at },
    moreEmojis: moreOf(mine),
    moreRequests: moreOf(requests),
  };
}

/**
 * The emoji of `id` the reader made, found in their list, read on page by page until it is found or the list ends;
 * `missing` once the whole list holds none of it.
 */
export function useOwnEmoji(id: string): ReturnType<typeof useOwnEmojis> & {
  readonly emoji: OwnCustomEmoji | undefined;
  readonly missing: boolean;
} {
  const read = useOwnEmojis();
  const emoji = read.emojis?.find((e) => e.id === id);
  const { hasNext, fetching, failed, next } = read.moreEmojis;
  const seeking = read.emojis !== undefined && emoji === undefined && hasNext && !fetching && !failed;
  useEffect(() => {
    if (seeking) next();
  }, [seeking, next]);
  return { ...read, emoji, missing: read.emojis !== undefined && emoji === undefined && !hasNext };
}

/** Reads the first page of the reader's emoji and of their requests, for a route's loader. */
export async function loadOwnEmojis(data: AppData): Promise<void> {
  await Promise.all([
    data.queryClient.infiniteQuery(ownEmojisQuery(data)).catch(() => undefined),
    data.queryClient.infiniteQuery(ownEmojiRequestsQuery(data)).catch(() => undefined),
  ]);
}

/** Whether the reader may ask for one more emoji this month: always without a limit. */
export const mayRequest = (quota: EmojiQuota): boolean => quota.limit === undefined || quota.used < quota.limit;

/** Reads again what the reader's writes change: their lists, the picker, and the administrators' lists. */
async function ownChanged(data: AppData): Promise<void> {
  const paths = new Set([
    "/custom_emojis/mine",
    "/custom_emoji_requests",
    "/admin/custom_emojis",
    "/admin/custom_emojis/{id}",
    "/admin/custom_emoji_requests",
  ]);
  void refreshCustomEmojis(data);
  await data.queryClient.invalidateQueries({ predicate: ({ queryKey }) => paths.has(String(queryKey[1])) });
}

/** The reader's writes of their emoji; each rejects with the server's problem (`emojiRefusal` says why). */
export type OwnEmojiWrites = {
  /** Uploads the image and asks for a new emoji of `shortcode`, and resolves with the request. */
  readonly request: (shortcode: string, file: File) => Promise<OwnCustomEmojiRequest>;
  readonly withdraw: (id: string) => Promise<OwnCustomEmojiRequest>;
  readonly change: (id: string, settings: EmojiSettings) => Promise<OwnCustomEmoji>;
  /** Deletes the reader's emoji of `id` for good. */
  readonly remove: (id: string) => Promise<void>;
};

/** The reader's writes of their emoji, for `data`. */
export function ownEmojiWrites(data: AppData): OwnEmojiWrites {
  const path = (id: string) => ({ params: { path: { id } } });
  const answered = async <T>(answer: T | undefined): Promise<T> => {
    if (answer === undefined) throw new Error("app-data: a write of an own emoji answered without a body");
    await ownChanged(data);
    return answer;
  };
  return {
    request: async (shortcode, file) => {
      const media = await uploadEmojiImage(data, file);
      return answered(
        (await data.api.POST("/custom_emoji_requests", { body: { kind: "create", shortcode, media_id: media.id } }))
          .data,
      );
    },
    withdraw: async (id) => answered((await data.api.POST("/custom_emoji_requests/{id}/withdraw", path(id))).data),
    change: async (id, s) =>
      answered(
        (
          await data.api.PATCH("/custom_emojis/mine/{id}", {
            ...path(id),
            body: { category: s.category, aliases: [...s.aliases], visible_in_picker: s.visibleInPicker },
          })
        ).data,
      ),
    remove: async (id) => {
      await data.api.DELETE("/custom_emojis/mine/{id}", path(id));
      await ownChanged(data);
    },
  };
}

/** The reader's writes of their emoji, for a screen. */
export function useOwnEmojiWrites(): OwnEmojiWrites {
  return ownEmojiWrites(useAppData());
}
