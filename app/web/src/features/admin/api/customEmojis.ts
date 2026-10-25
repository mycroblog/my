// SPDX-License-Identifier: AGPL-3.0-only
// The queries and mutations of the custom emoji screen (web-client/README.md S1, ADR-0051 §4, manage_custom_emojis): the
// emoji, local or remote, newest first (GET /admin/custom_emojis), registering a local one with an uploaded image
// (POST), changing its settings or enabling a remote one (PATCH), replacing its image (PUT .../image), deleting
// it (DELETE) and copying a remote one into a local one (POST .../copy). An emoji that breaks a rule is a 422 `invalid-custom-emoji` problem whose detail says why. Every
// change is recorded in the audit log and changes what the picker offers (GET /custom_emojis).
import { infiniteQueryOptions, type InfiniteData, type QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";
import { ApiError } from "../../../lib/errors";
import { pageQuery, refreshAuditLog } from "../../moderation";
import { uploadImage } from "../../settings";

type Schemas = components["schemas"];

export type AdminCustomEmoji = Schemas["AdminCustomEmoji"];
type NewCustomEmoji = Schemas["CreateCustomEmojiRequest"];
export type CustomEmojiChange = Schemas["UpdateCustomEmojiRequest"];

/** The image types that a custom emoji may be (createCustomEmoji): PNG, GIF and WebP, no JPEG. */
export const acceptedEmojiTypes = ["image/png", "image/gif", "image/webp"] as const;

/** The size above which GoToSocial does not fetch a remote emoji by default (ADR-0051 §2); the screen notes it. */
export const federatedEmojiBytes = 100 * 1024;

/** The rules of a shortcode and of an alias (ADR-0051 §1). */
export const shortcodePattern = "[A-Za-z0-9_]{2,30}";
export const maxAliases = 10;
export const categoryLimit = 64;

/** Which emoji the list shows: those of this server or of others, and a search (web-client/README.md S3). */
export type CustomEmojiOrigin = "local" | "remote";
export const customEmojiOrigins = ["local", "remote"] as const satisfies readonly CustomEmojiOrigin[];
export type CustomEmojiFilter = { origin: CustomEmojiOrigin; shortcode?: string | undefined; domain?: string | undefined };

/** The custom emoji under `filter`, newest first. */
export function adminCustomEmojisQueryOptions(api: Api, filter: CustomEmojiFilter) {
  const query = {
    local: filter.origin === "local",
    ...(filter.shortcode === undefined ? {} : { shortcode: filter.shortcode }),
    ...(filter.origin === "remote" && filter.domain !== undefined ? { domain: filter.domain } : {}),
  };
  return infiniteQueryOptions({
    queryKey: ["get", "/admin/custom_emojis", query, "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const { data, error } = await api.fetch.GET("/admin/custom_emojis", { params: { query: { ...query, ...pageQuery(pageParam) } }, signal });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** A new local emoji with the image file to upload for it. */
export type CustomEmojiRegistration = Omit<NewCustomEmoji, "media_id"> & { file: File };

/** Uploads the image of a new emoji and registers it, as the options of useMutation. Returns the emoji as recorded. */
export function createCustomEmojiMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/custom_emojis"] as const,
    mutationFn: async ({ file, ...emoji }: CustomEmojiRegistration): Promise<AdminCustomEmoji> => {
      const media = await uploadImage(api, file, "emoji");
      const { data, error } = await api.fetch.POST("/admin/custom_emojis", { body: { ...emoji, media_id: media.id } });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Changes the settings of an emoji, as the options of useMutation. Returns the emoji as recorded. */
export function updateCustomEmojiMutation(api: Api) {
  return {
    mutationKey: ["patch", "/admin/custom_emojis/{id}"] as const,
    mutationFn: async ({ id, change }: { id: string; change: CustomEmojiChange }): Promise<AdminCustomEmoji> => {
      const { data, error } = await api.fetch.PATCH("/admin/custom_emojis/{id}", { params: { path: { id } }, body: change });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Uploads a new image of a local emoji and makes it its image, as the options of useMutation. */
export function replaceCustomEmojiImageMutation(api: Api) {
  return {
    mutationKey: ["put", "/admin/custom_emojis/{id}/image"] as const,
    mutationFn: async ({ id, file }: { id: string; file: File }): Promise<AdminCustomEmoji> => {
      const media = await uploadImage(api, file, "emoji");
      const { data, error } = await api.fetch.PUT("/admin/custom_emojis/{id}/image", { params: { path: { id } }, body: { media_id: media.id } });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Copies a remote emoji into a local one under `shortcode` (absent: the remote one's), as the options of useMutation. */
export function copyCustomEmojiMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/custom_emojis/{id}/copy"] as const,
    mutationFn: async ({ id, shortcode }: { id: string; shortcode?: string | undefined }): Promise<AdminCustomEmoji> => {
      const { data, error } = await api.fetch.POST("/admin/custom_emojis/{id}/copy", {
        params: { path: { id } },
        body: shortcode === undefined ? {} : { shortcode },
      });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Deletes a local emoji, as the options of useMutation. */
export function deleteCustomEmojiMutation(api: Api) {
  return {
    mutationKey: ["delete", "/admin/custom_emojis/{id}"] as const,
    mutationFn: async (id: string): Promise<void> => {
      // The middleware throws for every error status; a 204 has no body to read.
      await api.fetch.DELETE("/admin/custom_emojis/{id}", { params: { path: { id } } });
    },
  };
}

type EmojiPages = InfiniteData<{ items: AdminCustomEmoji[]; next_cursor?: string }>;

/** Marks stale what any change of the emoji changes besides the rows: the picker's emoji and the audit log. */
async function refreshAfterChange(queryClient: QueryClient): Promise<void> {
  await Promise.all([queryClient.invalidateQueries({ queryKey: ["get", "/custom_emojis"] }), refreshAuditLog(queryClient)]);
}

/** A new emoji: the lists gain it (newest first), so they are fetched again, with the picker and the audit log. */
export async function cacheCreatedCustomEmoji(queryClient: QueryClient): Promise<void> {
  await Promise.all([queryClient.invalidateQueries({ queryKey: ["get", "/admin/custom_emojis"] }), refreshAfterChange(queryClient)]);
}

/** A changed emoji: its rows show the answer at once (ADR-0045). */
export async function cacheChangedCustomEmoji(queryClient: QueryClient, emoji: AdminCustomEmoji): Promise<void> {
  queryClient.setQueriesData<EmojiPages>({ queryKey: ["get", "/admin/custom_emojis"] }, (data) =>
    data && { ...data, pages: data.pages.map((page) => ({ ...page, items: page.items.map((item) => (item.id === emoji.id ? emoji : item)) })) },
  );
  await refreshAfterChange(queryClient);
}

/** A deleted emoji: it leaves the cached lists. */
export async function cacheDeletedCustomEmoji(queryClient: QueryClient, id: string): Promise<void> {
  await refreshAfterChange(queryClient);
  queryClient.setQueriesData<EmojiPages>({ queryKey: ["get", "/admin/custom_emojis"] }, (data) =>
    data && { ...data, pages: data.pages.map((page) => ({ ...page, items: page.items.filter((item) => item.id !== id) })) },
  );
}
