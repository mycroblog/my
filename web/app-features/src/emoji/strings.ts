// SPDX-License-Identifier: AGPL-3.0-only

// The words of the emoji picker that are the app's (ADR-0091 §1): the headings of its sections (the reader's frequent
// ones, the server's custom emoji, and Unicode's nine groups, in the order of emoji-test.txt) and the skin tone's
// button and its first choice, and the title and close button of the sheet it opens in on a phone, from the features'
// catalog. The five tones are named by the data generated from CLDR, which the caller passes; the picker's own words
// (its search, its results) are the design system's.

import type { EmojiPickerLabels } from "@my/design-system";
import { stringsIn } from "../locale";

/** Unicode's nine groups of emoji, in the order of emoji-test.txt. */
export const emojiGroups = [
  "smileys",
  "people",
  "nature",
  "food",
  "travel",
  "activities",
  "objects",
  "symbols",
  "flags",
] as const;

export type EmojiGroup = (typeof emojiGroups)[number];

/** The headings of the picker's sections in `locale`, and its skin tone's words with CLDR's five `toneNames`. */
export function emojiStrings(locale: string, toneNames: readonly string[]) {
  const { t } = stringsIn(locale);
  const groups = {
    smileys: t("emoji.group_smileys"),
    people: t("emoji.group_people"),
    nature: t("emoji.group_nature"),
    food: t("emoji.group_food"),
    travel: t("emoji.group_travel"),
    activities: t("emoji.group_activities"),
    objects: t("emoji.group_objects"),
    symbols: t("emoji.group_symbols"),
    flags: t("emoji.group_flags"),
  } satisfies Record<EmojiGroup, string>;
  const labels: EmojiPickerLabels = { tone: t("emoji.tone"), tones: [t("emoji.tone_default"), ...toneNames] };
  const sheet = { title: t("emoji.react"), close: t("emoji.close") };
  return { frequent: t("emoji.frequent"), custom: t("emoji.custom"), groups, labels, sheet };
}
