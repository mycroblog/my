// SPDX-License-Identifier: AGPL-3.0-only
// The sections and words that the pickers (the post form's and the reactions') pass to the EmojiPicker of @my/ui
// (ADR-0091 §3): the emoji used often, the custom emoji by category, those without one first as the server lists them
// (ADR-0051 §4), then the Unicode emoji by group of emoji-test.txt.
import type { EmojiPickerLabels, EmojiPickerSection, PickerCustomEmoji, PickerEmoji, SkinTone } from "@my/ui";
import type { PlainClientMessageKey, Translate } from "../../../lib/i18n";
import { shortcodeOf } from "./frequentEmojis";
import type { UnicodeEmojiSet, UnicodeGroupId } from "./unicodeEmoji";

/** A custom emoji of the server, with the category it is listed under. */
type ServerEmoji = PickerCustomEmoji & { category?: string | undefined };

/** The section of the emoji used often, from their keys (lib/storage `frequentEmojisKey`). */
export function frequentSection(emojis: readonly PickerEmoji[], t: Translate): EmojiPickerSection {
  return { id: "frequent", heading: t("client:emoji.picker.frequent"), icon: "frequent", emojis };
}

/**
 * The emoji of `keys` (Unicode emoji without their skin tone, custom emoji as `:shortcode:`) that the picker offers:
 * a custom emoji the server still has, a Unicode emoji once the Unicode data is loaded.
 */
export function emojisOfKeys(
  keys: readonly string[],
  custom: readonly PickerCustomEmoji[],
  unicode: UnicodeEmojiSet | undefined,
): PickerEmoji[] {
  const byShortcode = new Map(custom.map((emoji) => [emoji.shortcode, emoji]));
  return keys.flatMap((key): PickerEmoji[] => {
    const shortcode = shortcodeOf(key);
    const emoji = shortcode === undefined ? unicode?.byEmoji.get(key) : byShortcode.get(shortcode);
    return emoji === undefined ? [] : [emoji];
  });
}

/** One section per category of `emojis`, those without a category first. */
export function customEmojiSections(emojis: readonly ServerEmoji[], t: Translate): EmojiPickerSection[] {
  const sections: EmojiPickerSection[] = [];
  const byCategory = new Map<string, ServerEmoji[]>();
  for (const emoji of emojis) {
    const category = emoji.category ?? "";
    const list = byCategory.get(category) ?? [];
    list.push(emoji);
    byCategory.set(category, list);
  }
  const uncategorized = byCategory.get("");
  if (uncategorized !== undefined) {
    sections.push({
      id: "custom:",
      heading: t("client:emoji.picker.uncategorized"),
      icon: "custom",
      emojis: uncategorized,
    });
  }
  for (const [category, list] of byCategory) {
    if (category !== "") sections.push({ id: `custom:${category}`, heading: category, icon: "custom", emojis: list });
  }
  return sections;
}

/** The heading of each group of emoji-test.txt. */
const groupHeadings: Readonly<Record<UnicodeGroupId, PlainClientMessageKey>> = {
  "smileys-emotion": "client:emoji.picker.group.smileys_emotion",
  "people-body": "client:emoji.picker.group.people_body",
  "animals-nature": "client:emoji.picker.group.animals_nature",
  "food-drink": "client:emoji.picker.group.food_drink",
  "travel-places": "client:emoji.picker.group.travel_places",
  activities: "client:emoji.picker.group.activities",
  objects: "client:emoji.picker.group.objects",
  symbols: "client:emoji.picker.group.symbols",
  flags: "client:emoji.picker.group.flags",
};

/** One section per group of the Unicode emoji, in emoji-test.txt's order. */
export function unicodeEmojiSections(unicode: UnicodeEmojiSet, t: Translate): EmojiPickerSection[] {
  return unicode.groups.map((group) => ({
    id: `unicode:${group.id}`,
    heading: t(groupHeadings[group.id]),
    icon: group.id,
    emojis: group.emojis,
  }));
}

/** The words of the picker, with the accessible name of its grid. */
export function pickerLabels(grid: string, t: Translate): EmojiPickerLabels {
  return {
    search: t("client:emoji.picker.search"),
    grid,
    categories: t("client:emoji.picker.categories"),
    results: t("client:emoji.picker.results"),
    resultCount: (count) => t("client:emoji.picker.result_count", { count }),
    empty: t("client:emoji.picker.empty"),
  };
}

/** The name of each skin tone in the menu: the default, then the CLDR names of the five modifiers. */
const skinToneNames: Readonly<Record<SkinTone, PlainClientMessageKey>> = {
  none: "client:emoji.picker.skin_tone.none",
  light: "client:emoji.picker.skin_tone.light",
  "medium-light": "client:emoji.picker.skin_tone.medium_light",
  medium: "client:emoji.picker.skin_tone.medium",
  "medium-dark": "client:emoji.picker.skin_tone.medium_dark",
  dark: "client:emoji.picker.skin_tone.dark",
};

/** The words of the skin tone menu of the picker. */
export function skinToneLabels(t: Translate): Pick<EmojiPickerLabels, "skinTone" | "skinTones"> {
  return {
    skinTone: t("client:emoji.picker.skin_tone"),
    skinTones: {
      none: t(skinToneNames.none),
      light: t(skinToneNames.light),
      "medium-light": t(skinToneNames["medium-light"]),
      medium: t(skinToneNames.medium),
      "medium-dark": t(skinToneNames["medium-dark"]),
      dark: t(skinToneNames.dark),
    },
  };
}
