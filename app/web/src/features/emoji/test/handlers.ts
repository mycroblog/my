// SPDX-License-Identifier: AGPL-3.0-only
// The default MSW handler of the custom emoji feature (web-client/README.md §7): the picker's emoji, none by default, so
// that the forms show no picker unless a test offers emoji with `server.use(customEmojisHandler(...))`.
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../../test/fixtures";
import { apiBase } from "../../../test/msw/http";

/** An emoji of the picker, its images at media.example. */
export function pickerEmojiFixture(
  shortcode: string,
  init: Partial<Schemas["PickerCustomEmoji"]> = {},
): Schemas["PickerCustomEmoji"] {
  return {
    shortcode,
    url: `https://media.example/emoji/${shortcode}.png`,
    static_url: `https://media.example/emoji/${shortcode}.png`,
    aliases: [],
    ...init,
  };
}

/** GET /custom_emojis answering `items`. */
export function customEmojisHandler(items: Schemas["PickerCustomEmoji"][]) {
  return http.get(`${apiBase}/custom_emojis`, () =>
    HttpResponse.json({ items } satisfies Schemas["PickerCustomEmojiList"]),
  );
}

export const handlers = [
  // operationId listCustomEmojis.
  customEmojisHandler([]),
];
