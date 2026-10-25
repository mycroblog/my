// SPDX-License-Identifier: AGPL-3.0-only
// The queries and mutations of the user's page of their custom emoji (web-client/README.md S1, ADR-0070 §2, §3, §6,
// create_custom_emojis): the emoji the user made with what they used of their limit (GET /custom_emojis/mine), their
// requests (GET /custom_emoji_requests), requesting a new emoji or a new image of one with an uploaded image (POST
// /custom_emoji_requests), withdrawing a pending request (POST .../withdraw), and changing (PATCH) and deleting
// (DELETE /custom_emojis/mine/{id}) an emoji the user made. A request that breaks a rule is a 422
// `invalid-custom-emoji` problem, and one that cannot be withdrawn a 409 `custom-emoji-request-conflict`, whose
// detail says why.
import { infiniteQueryOptions, type InfiniteData, type QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";
import { ApiError } from "../../../lib/errors";
import { pageQuery } from "../../moderation";
import { uploadImage } from "../../settings";

type Schemas = components["schemas"];

export type OwnCustomEmoji = Schemas["OwnCustomEmoji"];
export type CustomEmojiRequest = Schemas["CustomEmojiRequest"];
export type OwnCustomEmojiChange = Schemas["UpdateOwnCustomEmojiRequest"];

/** The user's emoji, newest first, each page with what they used of their limit. */
export function ownCustomEmojisQueryOptions(api: Api) {
  return infiniteQueryOptions({
    queryKey: ["get", "/custom_emojis/mine", "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const { data, error } = await api.fetch.GET("/custom_emojis/mine", { params: { query: pageQuery(pageParam) }, signal });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** The user's requests in every state, newest first. */
export function ownCustomEmojiRequestsQueryOptions(api: Api) {
  return infiniteQueryOptions({
    queryKey: ["get", "/custom_emoji_requests", "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const { data, error } = await api.fetch.GET("/custom_emoji_requests", { params: { query: pageQuery(pageParam) }, signal });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** A request with the image file to upload for it: a new emoji under a shortcode, or a new image of one of the user's. */
export type CustomEmojiRequestDraft = { kind: "create"; shortcode: string; file: File } | { kind: "image"; emojiId: string; file: File };

/** Uploads the image of a request and sends the request, as the options of useMutation. Returns the request as recorded. */
export function requestCustomEmojiMutation(api: Api) {
  return {
    mutationKey: ["post", "/custom_emoji_requests"] as const,
    mutationFn: async (draft: CustomEmojiRequestDraft): Promise<CustomEmojiRequest> => {
      const media = await uploadImage(api, draft.file, "emoji");
      const body = draft.kind === "create" ? { kind: draft.kind, shortcode: draft.shortcode, media_id: media.id } : { kind: draft.kind, emoji_id: draft.emojiId, media_id: media.id };
      const { data, error } = await api.fetch.POST("/custom_emoji_requests", { body });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Withdraws a pending request, as the options of useMutation. Returns the request as withdrawn. */
export function withdrawCustomEmojiRequestMutation(api: Api) {
  return {
    mutationKey: ["post", "/custom_emoji_requests/{id}/withdraw"] as const,
    mutationFn: async (id: string): Promise<CustomEmojiRequest> => {
      const { data, error } = await api.fetch.POST("/custom_emoji_requests/{id}/withdraw", { params: { path: { id } } });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Changes the settings of one of the user's emoji, as the options of useMutation. Returns the emoji as recorded. */
export function updateOwnCustomEmojiMutation(api: Api) {
  return {
    mutationKey: ["patch", "/custom_emojis/mine/{id}"] as const,
    mutationFn: async ({ id, change }: { id: string; change: OwnCustomEmojiChange }): Promise<OwnCustomEmoji> => {
      const { data, error } = await api.fetch.PATCH("/custom_emojis/mine/{id}", { params: { path: { id } }, body: change });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Deletes one of the user's emoji, as the options of useMutation. */
export function deleteOwnCustomEmojiMutation(api: Api) {
  return {
    mutationKey: ["delete", "/custom_emojis/mine/{id}"] as const,
    mutationFn: async (id: string): Promise<void> => {
      // The middleware throws for every error status; a 204 has no body to read.
      await api.fetch.DELETE("/custom_emojis/mine/{id}", { params: { path: { id } } });
    },
  };
}

type EmojiPages = InfiniteData<{ items: OwnCustomEmoji[]; next_cursor?: string; used: number; limit?: number }>;
type RequestPages = InfiniteData<{ items: CustomEmojiRequest[]; next_cursor?: string }>;

/** A new request: the requests gain it (newest first), and a request to create counts against the limit. */
export async function cacheRequestedCustomEmoji(queryClient: QueryClient): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: ["get", "/custom_emoji_requests"] }),
    queryClient.invalidateQueries({ queryKey: ["get", "/custom_emojis/mine"] }),
  ]);
}

/** A withdrawn request: its row shows the answer at once, and it no longer counts against the limit. */
export async function cacheWithdrawnCustomEmojiRequest(queryClient: QueryClient, request: CustomEmojiRequest): Promise<void> {
  queryClient.setQueriesData<RequestPages>({ queryKey: ["get", "/custom_emoji_requests"] }, (data) =>
    data && { ...data, pages: data.pages.map((page) => ({ ...page, items: page.items.map((item) => (item.id === request.id ? request : item)) })) },
  );
  await queryClient.invalidateQueries({ queryKey: ["get", "/custom_emojis/mine"] });
}

/** A changed emoji: its row shows the answer at once (ADR-0045), and the picker may offer it differently. */
export async function cacheChangedOwnCustomEmoji(queryClient: QueryClient, emoji: OwnCustomEmoji): Promise<void> {
  queryClient.setQueriesData<EmojiPages>({ queryKey: ["get", "/custom_emojis/mine"] }, (data) =>
    data && { ...data, pages: data.pages.map((page) => ({ ...page, items: page.items.map((item) => (item.id === emoji.id ? emoji : item)) })) },
  );
  await queryClient.invalidateQueries({ queryKey: ["get", "/custom_emojis"] });
}

/**
 * A deleted emoji: it leaves the cached list, which is fetched again for the limit it frees, with the requests (a
 * pending request for a new image of it lapsed) and the picker.
 */
export async function cacheDeletedOwnCustomEmoji(queryClient: QueryClient, id: string): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: ["get", "/custom_emojis/mine"] }),
    queryClient.invalidateQueries({ queryKey: ["get", "/custom_emoji_requests"] }),
    queryClient.invalidateQueries({ queryKey: ["get", "/custom_emojis"] }),
  ]);
  queryClient.setQueriesData<EmojiPages>({ queryKey: ["get", "/custom_emojis/mine"] }, (data) =>
    data && { ...data, pages: data.pages.map((page) => ({ ...page, items: page.items.filter((item) => item.id !== id) })) },
  );
}
