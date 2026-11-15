// SPDX-License-Identifier: AGPL-3.0-only

// The Unicode emoji of the picker as it shows them, and the search over every emoji it offers (ADR-0091 §1, §3): built
// once per locale from the generated data, together with the index the search runs over, the words of each emoji in
// the form a query is compared in. It is a chunk of its own with the data, loaded when a picker is first wanted
// (../picker.ts), so that neither weighs on the app's entry.

import type { IconName, PickerEmoji } from "@my/design-system";
import type { EmojiGroup } from "../strings";
import type { UnicodeEmojiData, UnicodeEmojiNames } from ".";

/** The words an emoji is searched by, besides itself: its name, and its keywords (a custom emoji's aliases). */
export type Words = { name: string; keywords: readonly string[] };

/** An emoji of the search's index: its words in the form a query is compared in, computed once. */
export type Entry = { emoji: PickerEmoji; text: string | undefined; name: string; keywords: readonly string[] };

/**
 * The Unicode emoji named in a locale, by group, the index of their words, and the five tones' names. The index is
 * built at the first search and kept, so that opening the picker does not wait for it.
 */
export type UnicodeSet = {
  groups: readonly { id: EmojiGroup; icon: IconName; emojis: readonly PickerEmoji[] }[];
  byKey: ReadonlyMap<string, PickerEmoji>;
  index: () => readonly Entry[];
  tones: readonly string[];
};

/** The group of emoji-test.txt of each of the catalog's groups, in the same order, and its category's icon. */
const dataGroups: Readonly<Record<UnicodeEmojiData["groups"][number]["id"], [EmojiGroup, IconName]>> = {
  "smileys-emotion": ["smileys", "emojiSmileys"],
  "people-body": ["people", "emojiPeople"],
  "animals-nature": ["nature", "emojiNature"],
  "food-drink": ["food", "emojiFood"],
  "travel-places": ["travel", "emojiTravel"],
  activities: ["activities", "emojiActivities"],
  objects: ["objects", "emojiObjects"],
  symbols: ["symbols", "emojiSymbols"],
  flags: ["flags", "emojiFlags"],
};

/** The five skin tone modifiers, light to dark. */
const modifiers = ["\u{1F3FB}", "\u{1F3FC}", "\u{1F3FD}", "\u{1F3FE}", "\u{1F3FF}"] as const;
const modifierPattern = /[\u{1F3FB}-\u{1F3FF}]/gu;

/**
 * The text as emoji are compared with it (ADR-0091 §3): NFKC, lower case, katakana as hiragana, without spaces or the
 * colons around a shortcode. The words of the emoji are compared in the same form.
 */
export function normalizeQuery(text: string): string {
  const form = text
    .normalize("NFKC")
    .toLowerCase()
    .replace(/[ァ-ヶ]/gu, (k) => String.fromCodePoint((k.codePointAt(0) ?? 0) - 0x60))
    .trim();
  // The colons are cut by index, not by /:+$/, which takes quadratic time on a text of many colons not at its end.
  let start = 0;
  let end = form.length;
  while (start < end && form[start] === ":") start++;
  while (end > start && form[end - 1] === ":") end--;
  return form.slice(start, end);
}

/** The entry of the index of `emoji`, with its words. */
export const entryOf = (emoji: PickerEmoji, words: Words): Entry => ({
  emoji,
  text: "text" in emoji ? normalizeQuery(emoji.text) : undefined,
  name: normalizeQuery(words.name),
  keywords: words.keywords.map(normalizeQuery),
});

/**
 * The variant of each tone among `skins`: the one whose every modifier is the tone's, so that an emoji of several
 * people has the same tone for all of them (ADR-0091 §3); none when a tone has no such variant.
 */
function tonesOf(skins: readonly (readonly [string, string])[] | undefined): readonly string[] | undefined {
  if (skins === undefined) return undefined;
  const found = modifiers.map(
    (m) => skins.find(([variant]) => (variant.match(modifierPattern) ?? []).every((x) => x === m))?.[0],
  );
  return found.every((v) => v !== undefined) ? found : undefined;
}

/** The emoji of `data` named in `names`, `en`'s names and keywords joining the keywords in another locale. */
export function buildSet(
  data: UnicodeEmojiData,
  names: UnicodeEmojiNames,
  en: UnicodeEmojiNames | undefined,
): UnicodeSet {
  const byKey = new Map<string, PickerEmoji>();
  const groups = data.groups.map((group) => {
    const [id, icon] = dataGroups[group.id];
    return {
      id,
      icon,
      emojis: group.emoji.map(([text, , skins]): PickerEmoji => {
        const [name] = names.emoji[text] ?? [text];
        const tones = tonesOf(skins);
        const emoji: PickerEmoji = { key: text, name, text, ...(tones === undefined ? {} : { tones }) };
        byKey.set(text, emoji);
        return emoji;
      }),
    };
  });
  let index: Entry[] | undefined;
  const indexOf = () =>
    (index ??= groups.flatMap((g) =>
      g.emojis.map((emoji) => {
        const [name, keywords] = names.emoji[emoji.key] ?? [emoji.key, []];
        const english = en?.emoji[emoji.key];
        return entryOf(emoji, {
          name,
          keywords: english === undefined ? keywords : [...keywords, english[0], ...english[1]],
        });
      }),
    ));
  return { groups, byKey, index: indexOf, tones: modifiers.map((m) => names.tones[m] ?? "") };
}

/**
 * How well an entry matches `query`, best first (ADR-0091 §3): 0 when its name (a custom emoji's shortcode) is the
 * query or it is the emoji itself, 1 when its name starts with it, 2 when a keyword (an alias) does, 3 when its name
 * or a keyword holds it; undefined when it does not match.
 */
function rank(e: Entry, query: string): number | undefined {
  if (e.name === query || e.text === query) return 0;
  if (e.name.startsWith(query)) return 1;
  if (e.keywords.some((k) => k.startsWith(query))) return 2;
  if (e.name.includes(query) || e.keywords.some((k) => k.includes(query))) return 3;
  return undefined;
}

/**
 * The emoji of `indexes` that match `text`, by rank, in the order given within one (the custom emoji first); within a
 * rank, those that `first` picks (the emoji used often, for the composer's suggestions) before the rest.
 */
export function searchIndex(
  indexes: readonly (readonly Entry[])[],
  text: string,
  first?: (emoji: PickerEmoji) => boolean,
): PickerEmoji[] {
  const query = normalizeQuery(text);
  if (query === "") return [];
  const ranks: PickerEmoji[][] = [[], [], [], []];
  for (const index of indexes)
    for (const e of index) {
      const r = rank(e, query);
      if (r !== undefined) ranks[r]?.push(e.emoji);
    }
  if (first === undefined) return ranks.flat();
  return ranks.flatMap((r) => [...r.filter(first), ...r.filter((e) => !first(e))]);
}
