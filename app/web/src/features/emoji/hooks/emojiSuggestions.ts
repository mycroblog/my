// SPDX-License-Identifier: AGPL-3.0-only
// The emoji that the completion after ":" offers (ADR-0092 §2): from the data the client has, never asking the server
// for each word: the custom emoji of the picker (GET /custom_emojis, matched by shortcode and alias) and the Unicode
// emoji (matched by their CLDR names and keywords in the locale shown and in `en`), ranked as the picker's search ranks
// them (prefix before substring), with the emoji used often on this browser first, at most `maxEmojiSuggestions`.
// A Unicode emoji goes in as itself, in the skin tone chosen in the picker (ADR-0091 §3); a custom emoji as `:shortcode:`.
import { type PickerCustomEmoji, type PickerEmoji, type PickerUnicodeEmoji, searchEmojis, skinToneVariant } from "@my/ui";
import { useQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { useLocale } from "../../../lib/i18n";
import { skinToneKey, useStorage } from "../../../lib/storage";
import { customEmojisQueryOptions } from "../api/customEmojis";
import { customEmojiKey, useFrequentEmojis } from "./frequentEmojis";
import { loadUnicodeEmojiSet, type UnicodeEmojiSet } from "./unicodeEmoji";

/** How many emoji the completion offers (ADR-0092 §2). */
const maxEmojiSuggestions = 8;

/** An emoji the completion offers: its key, the text it inserts, and the emoji to show. */
export type EmojiSuggestion =
  | { kind: "custom"; key: string; value: string; emoji: PickerCustomEmoji }
  | { kind: "unicode"; key: string; value: string; emoji: PickerUnicodeEmoji };

/** Every Unicode emoji of a set in emoji-test.txt's order, listed once per set. */
const flattened = new WeakMap<UnicodeEmojiSet, readonly PickerUnicodeEmoji[]>();

function allUnicode(set: UnicodeEmojiSet): readonly PickerUnicodeEmoji[] {
  let list = flattened.get(set);
  if (list === undefined) {
    list = set.groups.flatMap((group) => group.emojis);
    flattened.set(set, list);
  }
  return list;
}

/** The key an emoji is counted by in the emoji used often: a custom emoji's `:shortcode:`, a Unicode emoji itself. */
function keyOf(emoji: PickerEmoji): string {
  return "unicode" in emoji ? emoji.unicode : customEmojiKey(emoji.shortcode);
}

/**
 * The emoji of `custom` and `unicode` that match `query`, ranked by the picker's search, those of `frequent` first
 * (in their order of use), at most `maxEmojiSuggestions`.
 */
function rankEmojiSuggestions(
  query: string,
  custom: readonly PickerCustomEmoji[],
  unicode: readonly PickerUnicodeEmoji[],
  frequent: readonly string[],
): PickerEmoji[] {
  const found = searchEmojis([...custom, ...unicode], query);
  const usage = new Map(frequent.map((key, index) => [key, index]));
  const often = found.filter((emoji) => usage.has(keyOf(emoji))).toSorted((a, b) => (usage.get(keyOf(a)) ?? 0) - (usage.get(keyOf(b)) ?? 0));
  const rest = found.filter((emoji) => !usage.has(keyOf(emoji)));
  return [...often, ...rest].slice(0, maxEmojiSuggestions);
}

/**
 * The function that gives the emoji suggestions for the query after ":". It waits for the Unicode emoji the first time
 * (and offers the custom emoji alone when they cannot be loaded).
 */
export function useEmojiSuggestions(): (query: string) => Promise<EmojiSuggestion[]> {
  const api = useApi();
  const locale = useLocale();
  const { data } = useQuery(customEmojisQueryOptions(api));
  const frequent = useFrequentEmojis();
  const skinTone = useStorage(skinToneKey);
  const custom = data?.items ?? [];
  return async (query) => {
    const unicode = await loadUnicodeEmojiSet(locale).then(allUnicode, () => []);
    return rankEmojiSuggestions(query, custom, unicode, frequent).map((emoji): EmojiSuggestion =>
      "unicode" in emoji
        ? { kind: "unicode", key: emoji.unicode, value: skinToneVariant(emoji, skinTone)?.unicode ?? emoji.unicode, emoji }
        : { kind: "custom", key: customEmojiKey(emoji.shortcode), value: customEmojiKey(emoji.shortcode), emoji },
    );
  };
}
