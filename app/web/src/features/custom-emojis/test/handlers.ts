// SPDX-License-Identifier: AGPL-3.0-only
// The default MSW handlers of the user's custom emoji feature (web-client/README.md §7): no emoji and no requests, without a
// limit; a new request is answered pending, a withdrawn one withdrawn, and a changed emoji as sent. The default
// viewer has no permissions, so the page's tests start with `server.use(operatorHandler(["create_custom_emojis"]))`.
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../../test/fixtures";
import { apiBase, withViewer } from "../../../test/msw/http";

/** An emoji that the user made, at 2026-10-20T00:00:00Z, with `overrides`. */
export function ownCustomEmojiFixture(
  id: string,
  shortcode: string,
  overrides: Partial<Schemas["OwnCustomEmoji"]> = {},
): Schemas["OwnCustomEmoji"] {
  return {
    id,
    shortcode,
    url: `https://my.example/media/${id}/r/medium.webp`,
    static_url: `https://my.example/media/${id}/r/medium.webp`,
    enabled: true,
    visible_in_picker: true,
    aliases: [],
    created_at: "2026-10-20T00:00:00Z",
    updated_at: "2026-10-20T00:00:00Z",
    ...overrides,
  };
}

/** A pending request of the user for a new emoji, made at 2026-10-21T00:00:00Z, with `overrides`. */
export function customEmojiRequestFixture(
  id: string,
  shortcode: string,
  overrides: Partial<Schemas["CustomEmojiRequest"]> = {},
): Schemas["CustomEmojiRequest"] {
  return {
    id,
    kind: "create",
    state: "pending",
    shortcode,
    url: `https://my.example/media/${id}/r/medium.webp`,
    static_url: `https://my.example/media/${id}/r/medium.webp`,
    created_at: "2026-10-21T00:00:00Z",
    ...overrides,
  };
}

/** The ID the default POST /custom_emoji_requests handler gives a new request. */
const createdRequestId = "0199f0e0-0000-7000-8000-00000000e101";

export const handlers = [
  // operationIds listOwnCustomEmojis, updateOwnCustomEmoji, deleteOwnCustomEmoji.
  http.get(`${apiBase}/custom_emojis/mine`, ({ request }) =>
    withViewer(request, () =>
      HttpResponse.json({
        items: [],
        used: 0,
        pending: 0,
        resets_at: "2026-11-01T00:00:00Z",
      } satisfies Schemas["OwnCustomEmojiPage"]),
    ),
  ),
  http.patch(`${apiBase}/custom_emojis/mine/:id`, async ({ request, params }) => {
    const body = (await request.clone().json()) as Schemas["UpdateOwnCustomEmojiRequest"];
    return withViewer(request, () => HttpResponse.json(ownCustomEmojiFixture(String(params["id"]), "blobcat", body)));
  }),
  http.delete(`${apiBase}/custom_emojis/mine/:id`, ({ request }) =>
    withViewer(request, () => new HttpResponse(null, { status: 204 })),
  ),
  // operationIds listCustomEmojiRequests, createCustomEmojiRequest, withdrawCustomEmojiRequest.
  http.get(`${apiBase}/custom_emoji_requests`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: [] } satisfies Schemas["CustomEmojiRequestPage"])),
  ),
  http.post(`${apiBase}/custom_emoji_requests`, async ({ request }) => {
    const body = (await request.clone().json()) as Schemas["NewCustomEmojiRequest"];
    return withViewer(request, () =>
      HttpResponse.json(
        customEmojiRequestFixture(createdRequestId, body.shortcode ?? "blobcat", {
          kind: body.kind,
          ...(body.emoji_id === undefined ? {} : { emoji_id: body.emoji_id }),
        }),
      ),
    );
  }),
  http.post(`${apiBase}/custom_emoji_requests/:id/withdraw`, ({ request, params }) =>
    withViewer(request, () =>
      HttpResponse.json(
        customEmojiRequestFixture(String(params["id"]), "blobcat", {
          state: "withdrawn",
          decided_at: "2026-10-22T00:00:00Z",
        }),
      ),
    ),
  ),
];
