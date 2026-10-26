// SPDX-License-Identifier: AGPL-3.0-only
// The shortcodes of this server's custom emoji that the client knows (GET /custom_emojis, the picker's query): the
// highlight layer of a text field marks a `:shortcode:` only when it is one of them (ADR-0092 §4); their images show
// a `:shortcode:` that arrives alone (a reaction in a voice chat room, ADR-0100 §10).
import type { CustomEmojiImage } from "@my/ui";
import { useQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { customEmojisQueryOptions } from "../api/customEmojis";

/** The shortcodes of the local custom emoji, without colons; empty until the list arrives. */
export function useCustomEmojiShortcodes(): ReadonlySet<string> {
  const api = useApi();
  const { data } = useQuery(customEmojisQueryOptions(api));
  return new Set(data?.items.map((emoji) => emoji.shortcode));
}

/**
 * The images of the local custom emoji by shortcode (without colons), for showing a `:shortcode:` that arrives on its
 * own, such as a reaction in a voice chat room; empty until the list arrives.
 */
export function useCustomEmojiImages(): ReadonlyMap<string, CustomEmojiImage> {
  const api = useApi();
  const { data } = useQuery(customEmojisQueryOptions(api));
  return new Map(data?.items.map((emoji) => [emoji.shortcode, { shortcode: emoji.shortcode, url: emoji.url, static_url: emoji.static_url }]));
}
