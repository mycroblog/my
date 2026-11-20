// SPDX-License-Identifier: AGPL-3.0-only
// The custom emoji as a holder of `manage_custom_emojis` keeps them (FR-111, FR-915, ADR-0051, ADR-0070): the emoji,
// local and remote, newest first, a page at a time, kept to those of this server or of others and found by their
// shortcode; one emoji; registering a local one from an image uploaded for an emoji, replacing its image, changing its
// category, its aliases and whether the picker offers it, disabling and enabling one, deleting a local one, and
// copying a remote one into a local one; and the requests of the holders of `create_custom_emojis`, the pending ones
// by default, approved or rejected with a reason the requester reads. Every write reads again what it changes: the
// emoji, the requests, the picker's list every reader reads, and the requester's own lists.
import { ProblemError, uploadMedia, type components, type MediaAttachment } from "@mycroblog/mykit";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { MediaFailedError } from "./compose";
import { useAppData } from "./provider";
import { refreshCustomEmojis } from "./reactions";
import { readOf, type ScreenRead } from "./reads";

/** A custom emoji as the administrators manage it. */
export type AdminCustomEmoji = components["schemas"]["AdminCustomEmoji"];
/** A request for a custom emoji, with its requester. */
export type AdminCustomEmojiRequest = components["schemas"]["AdminCustomEmojiRequest"];
/** Where a request stands. */
export type CustomEmojiRequestState = components["schemas"]["CustomEmojiRequestState"];
/** Why the server refused a custom emoji (422 `invalid-custom-emoji`). */
export type CustomEmojiRefusal = components["schemas"]["CustomEmojiProblemReason"];
/** Why a request could not be decided or withdrawn (409 `custom-emoji-request-conflict`). */
export type CustomEmojiRequestConflict = components["schemas"]["CustomEmojiRequestConflictReason"];

/** The states of a request, pending first. */
export const emojiRequestStates = [
  "pending",
  "approved",
  "rejected",
  "withdrawn",
  "lapsed",
] as const satisfies readonly CustomEmojiRequestState[];

/** The rules of an emoji's fields, as the server checks them. */
export const emojiLimits = { category: 64, aliases: 10, reason: 500, warnBytes: 100 * 1024 } as const;

/** A shortcode the server takes: 2 to 30 letters (A to Z, a to z), digits and underscores. */
export const validShortcode = (shortcode: string): boolean => /^[A-Za-z0-9_]{2,30}$/.test(shortcode);

/** The aliases written in one field, split at spaces and commas, colons around each taken off, each once. */
export const aliasesOf = (text: string): string[] => [
  ...new Set(
    text
      .split(/[\s,、]+/)
      .map((a) => a.replace(/^:+|:+$/g, ""))
      .filter((a) => a !== ""),
  ),
];

/** What the list of emoji shows: those of this server (`local`) or of others (`remote`), a shortcode, a page. */
export type AdminEmojiQuery = {
  readonly origin?: "local" | "remote" | undefined;
  readonly shortcode?: string | undefined;
  readonly cursor?: string | undefined;
};

/** A page of the emoji. */
export function adminEmojisQuery({ queries }: AppData, q: AdminEmojiQuery) {
  return queries.queryOptions("get", "/admin/custom_emojis", {
    params: {
      query: {
        ...(q.origin === undefined ? {} : { local: q.origin === "local" }),
        ...(q.shortcode === undefined || q.shortcode === "" ? {} : { shortcode: q.shortcode }),
        ...(q.cursor === undefined ? {} : { cursor: q.cursor }),
      },
    },
  });
}

/** One emoji. */
export function adminEmojiQuery({ queries }: AppData, id: string) {
  return queries.queryOptions("get", "/admin/custom_emojis/{id}", { params: { path: { id } } });
}

/** A page of the requests in `state`, the pending ones by default. */
export function adminEmojiRequestsQuery({ queries }: AppData, state: CustomEmojiRequestState, cursor?: string) {
  return queries.queryOptions("get", "/admin/custom_emoji_requests", {
    params: { query: { state, ...(cursor === undefined ? {} : { cursor }) } },
  });
}

/** Whether `error` is the server's answer of `status`. */
const answeredWith = (error: unknown, status: number) => error instanceof ProblemError && error.status === status;

/** A page as a screen shows it: its items, the next page's cursor, and where its read is. */
export type EmojiPage<T> = ScreenRead & {
  readonly items: readonly T[] | undefined;
  readonly next: string | undefined;
  /** The page shows what the reader asked before while the one they asked for now is on its way. */
  readonly stale: boolean;
};

/** The page of emoji `q` asks for; while another is on its way, the one shown before stays. */
export function useAdminEmojis(q: AdminEmojiQuery): EmojiPage<AdminCustomEmoji> {
  const query = useQuery({ ...adminEmojisQuery(useAppData(), q), placeholderData: keepPreviousData });
  const next = query.data?.next_cursor;
  return {
    ...readOf([query]),
    items: query.data?.items,
    next: next === undefined || next === "" ? undefined : next,
    stale: query.isPlaceholderData,
  };
}

/** Reads the page of emoji `q` asks for, for a route's loader. */
export async function loadAdminEmojis(data: AppData, q: AdminEmojiQuery): Promise<void> {
  await data.queryClient.query(adminEmojisQuery(data, q)).catch(() => undefined);
}

/** The emoji of `id`, and whether the server has none of it (404). */
export function useAdminEmoji(id: string): ScreenRead & {
  readonly emoji: AdminCustomEmoji | undefined;
  readonly missing: boolean;
} {
  const query = useQuery(adminEmojiQuery(useAppData(), id));
  return { ...readOf([query]), emoji: query.data, missing: answeredWith(query.error, 404) };
}

/** Reads the emoji of `id`, for a route's loader. */
export async function loadAdminEmoji(data: AppData, id: string): Promise<void> {
  await data.queryClient.query(adminEmojiQuery(data, id)).catch(() => undefined);
}

/** The page of requests in `state` after `cursor`; while another is on its way, the one shown before stays. */
export function useAdminEmojiRequests(
  state: CustomEmojiRequestState,
  cursor?: string,
): EmojiPage<AdminCustomEmojiRequest> {
  const query = useQuery({
    ...adminEmojiRequestsQuery(useAppData(), state, cursor),
    placeholderData: keepPreviousData,
  });
  const next = query.data?.next_cursor;
  return {
    ...readOf([query]),
    items: query.data?.items,
    next: next === undefined || next === "" ? undefined : next,
    stale: query.isPlaceholderData,
  };
}

/** Reads the page of requests in `state`, and the pending ones the tab counts, for a route's loader. */
export async function loadAdminEmojiRequests(
  data: AppData,
  state: CustomEmojiRequestState,
  cursor?: string,
): Promise<void> {
  await Promise.all([
    data.queryClient.query(adminEmojiRequestsQuery(data, state, cursor)).catch(() => undefined),
    data.queryClient.query(adminEmojiRequestsQuery(data, "pending")).catch(() => undefined),
  ]);
}

/**
 * How many requests wait, as the first page of the pending ones holds them, and whether more wait past it (the API
 * counts none); undefined until it is read.
 */
export function usePendingEmojiRequests(): { readonly count: number; readonly more: boolean } | undefined {
  const page = useQuery(adminEmojiRequestsQuery(useAppData(), "pending")).data;
  if (page === undefined) return undefined;
  return { count: page.items.length, more: page.next_cursor !== undefined && page.next_cursor !== "" };
}

/** Why the server refused an emoji or a request (`reason` of a 422 or a 409); undefined for any other failure. */
export function emojiRefusal(error: unknown): CustomEmojiRefusal | CustomEmojiRequestConflict | undefined {
  if (!(error instanceof ProblemError) || (error.status !== 422 && error.status !== 409)) return undefined;
  const reason: unknown = error.problem["reason"];
  return typeof reason === "string" ? (reason as CustomEmojiRefusal | CustomEmojiRequestConflict) : undefined;
}

/** How long to wait between two looks at an image the server processes. */
const processingPollMs = 1000;

/**
 * Uploads `file` as the image of a custom emoji (the `emoji` purpose) and waits until the server has processed it;
 * resolves with the ready file, rejects with MediaFailedError when the server could not process it, or with the
 * server's problem.
 */
export async function uploadEmojiImage(data: AppData, file: File): Promise<MediaAttachment> {
  let media = await uploadMedia(data.api, { file, purpose: "emoji" });
  while (media.state === "processing") {
    await new Promise((resolve) => setTimeout(resolve, processingPollMs));
    const { data: next } = await data.api.GET("/media/{id}", { params: { path: { id: media.id } } });
    if (next === undefined) throw new Error("app-data: the media answered without a file");
    media = next;
  }
  if (media.state === "failed") throw new MediaFailedError(`app-data: the server could not process ${media.id}`);
  return media;
}

/** Reads again what a write of the emoji changes: the emoji, the picker every reader reads, the requesters' lists. */
async function emojisChanged(data: AppData): Promise<void> {
  const paths = new Set([
    "/admin/custom_emojis",
    "/admin/custom_emojis/{id}",
    "/admin/custom_emoji_requests",
    "/custom_emojis/mine",
    "/custom_emoji_requests",
  ]);
  void refreshCustomEmojis(data);
  await data.queryClient.invalidateQueries({ predicate: ({ queryKey }) => paths.has(String(queryKey[1])) });
}

/** What a new local emoji is: its shortcode, its image, and where the picker offers it. */
export type NewEmoji = {
  readonly shortcode: string;
  readonly file: File;
  readonly category: string;
  readonly aliases: readonly string[];
  readonly visibleInPicker: boolean;
};

/** The settings of a local emoji. */
export type EmojiSettings = {
  readonly category: string;
  readonly aliases: readonly string[];
  readonly visibleInPicker: boolean;
};

/** The writes of the emoji and their requests; each rejects with the server's problem (`emojiRefusal` says why). */
export type AdminEmojiWrites = {
  /** Uploads the image and registers the emoji, and resolves with it. */
  readonly register: (emoji: NewEmoji) => Promise<AdminCustomEmoji>;
  readonly replaceImage: (id: string, file: File) => Promise<AdminCustomEmoji>;
  readonly change: (id: string, settings: EmojiSettings) => Promise<AdminCustomEmoji>;
  readonly enable: (id: string) => Promise<AdminCustomEmoji>;
  readonly disable: (id: string) => Promise<AdminCustomEmoji>;
  /** Deletes a local emoji for good. */
  readonly remove: (id: string) => Promise<void>;
  /** Copies a remote emoji into a local one under `shortcode` (the remote one's where empty), and resolves with it. */
  readonly copy: (id: string, shortcode: string) => Promise<AdminCustomEmoji>;
  readonly approve: (id: string) => Promise<AdminCustomEmojiRequest>;
  /** Rejects a request with `reason`, which the requester reads (none where empty). */
  readonly reject: (id: string, reason: string) => Promise<AdminCustomEmojiRequest>;
};

/** The writes of the emoji, for `data`. */
export function adminEmojiWrites(data: AppData): AdminEmojiWrites {
  const path = (id: string) => ({ params: { path: { id } } });
  const answered = async <T>(answer: T | undefined): Promise<T> => {
    if (answer === undefined) throw new Error("app-data: a write of an emoji answered without the emoji");
    await emojisChanged(data);
    return answer;
  };
  return {
    register: async (emoji) => {
      const media = await uploadEmojiImage(data, emoji.file);
      return answered(
        (
          await data.api.POST("/admin/custom_emojis", {
            body: {
              shortcode: emoji.shortcode,
              media_id: media.id,
              category: emoji.category,
              aliases: [...emoji.aliases],
              visible_in_picker: emoji.visibleInPicker,
            },
          })
        ).data,
      );
    },
    replaceImage: async (id, file) => {
      const media = await uploadEmojiImage(data, file);
      return answered(
        (await data.api.PUT("/admin/custom_emojis/{id}/image", { ...path(id), body: { media_id: media.id } })).data,
      );
    },
    change: async (id, s) =>
      answered(
        (
          await data.api.PATCH("/admin/custom_emojis/{id}", {
            ...path(id),
            body: { category: s.category, aliases: [...s.aliases], visible_in_picker: s.visibleInPicker },
          })
        ).data,
      ),
    enable: async (id) => answered((await data.api.POST("/admin/custom_emojis/{id}/enable", path(id))).data),
    disable: async (id) => answered((await data.api.POST("/admin/custom_emojis/{id}/disable", path(id))).data),
    remove: async (id) => {
      await data.api.DELETE("/admin/custom_emojis/{id}", path(id));
      await emojisChanged(data);
    },
    copy: async (id, shortcode) =>
      answered(
        (
          await data.api.POST("/admin/custom_emojis/{id}/copy", {
            ...path(id),
            body: shortcode === "" ? {} : { shortcode },
          })
        ).data,
      ),
    approve: async (id) => answered((await data.api.POST("/admin/custom_emoji_requests/{id}/approve", path(id))).data),
    reject: async (id, reason) =>
      answered(
        (
          await data.api.POST("/admin/custom_emoji_requests/{id}/reject", {
            ...path(id),
            body: reason.trim() === "" ? {} : { reason: reason.trim() },
          })
        ).data,
      ),
  };
}

/** The writes of the emoji, for a screen. */
export function useAdminEmojiWrites(): AdminEmojiWrites {
  return adminEmojiWrites(useAppData());
}
