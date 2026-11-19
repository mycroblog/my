// SPDX-License-Identifier: AGPL-3.0-only
// The custom emoji a holder of `create_custom_emojis` asks for and keeps (FR-915, ADR-0070 §2–§4): the emoji they
// made, newest first, with what they used of their monthly limit (the approved and pending requests to create one
// this UTC month, the limit, none for no limit, and when the count resets); their requests, newest first, in every
// state; requesting a new emoji from an image uploaded for an emoji, withdrawing a pending request, and changing the
// category, the aliases and the place in the picker of one of theirs, or deleting it (which gives no place back).
// Every write reads again what it changes: their lists, the picker every reader reads, and the administrators' lists.
import type { components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { uploadEmojiImage, type EmojiSettings } from "./admin-custom-emojis";
import { useAppData } from "./provider";
import { customEmojisQuery } from "./reactions";
import { readOf, type ScreenRead } from "./reads";

/** A local custom emoji the reader made. */
export type OwnCustomEmoji = components["schemas"]["OwnCustomEmoji"];
/** A request of the reader's for a custom emoji. */
export type OwnCustomEmojiRequest = components["schemas"]["CustomEmojiRequest"];

/** The reader's own emoji, the most a page holds (Limit), with what they used of their monthly limit. */
export function ownEmojisQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/custom_emojis/mine", { params: { query: { limit: 40 } } });
}

/** The reader's requests, the most a page holds. */
export function ownEmojiRequestsQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/custom_emoji_requests", { params: { query: { limit: 40 } } });
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

/** The reader's emoji, their requests and what they used of their limit, once both are read. */
export function useOwnEmojis(): ScreenRead & {
  readonly emojis: readonly OwnCustomEmoji[] | undefined;
  readonly requests: readonly OwnCustomEmojiRequest[] | undefined;
  readonly quota: EmojiQuota | undefined;
} {
  const data = useAppData();
  const mine = useQuery(ownEmojisQuery(data));
  const requests = useQuery(ownEmojiRequestsQuery(data));
  const page = mine.data;
  return {
    ...readOf([mine, requests]),
    emojis: page?.items,
    requests: requests.data?.items,
    quota:
      page === undefined
        ? undefined
        : { used: page.used, pending: page.pending, limit: page.limit, resetsAt: page.resets_at },
  };
}

/** The emoji of `id` the reader made, found in their list; `missing` once the list holds none of it. */
export function useOwnEmoji(id: string): ReturnType<typeof useOwnEmojis> & {
  readonly emoji: OwnCustomEmoji | undefined;
  readonly missing: boolean;
} {
  const read = useOwnEmojis();
  const emoji = read.emojis?.find((e) => e.id === id);
  return { ...read, emoji, missing: read.emojis !== undefined && emoji === undefined };
}

/** Reads the reader's emoji and requests, for a route's loader. */
export async function loadOwnEmojis(data: AppData): Promise<void> {
  await Promise.all([
    data.queryClient.query(ownEmojisQuery(data)).catch(() => undefined),
    data.queryClient.query(ownEmojiRequestsQuery(data)).catch(() => undefined),
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
  void data.queryClient.invalidateQueries({ queryKey: customEmojisQuery(data).queryKey });
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
