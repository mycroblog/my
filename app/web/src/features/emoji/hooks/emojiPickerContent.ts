// SPDX-License-Identifier: AGPL-3.0-only
// What the emoji pickers show (ADR-0091 §3, §7): the emoji used often, the custom emoji of this server and every
// Unicode emoji, in the skin tone chosen on this browser. The picker of a text field (useEmojiPicker) and the picker of
// reactions (useReactionPicker) share it and differ only in what a picked emoji does.
import type { EmojiPickerLabels, EmojiPickerSection, EmojiPickerSelection, SkinTone } from "@my/ui";
import { useQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { useLocale, useT } from "../../../lib/i18n";
import { skinToneKey, useStorage } from "../../../lib/storage";
import { customEmojisQueryOptions } from "../api/customEmojis";
import { customEmojiKey, defaultFrequentEmojis, rememberEmoji, useFrequentEmojis } from "./frequentEmojis";
import {
  customEmojiSections,
  emojisOfKeys,
  frequentSection,
  loadingUnicodeEmojiSections,
  pickerLabels,
  skinToneLabels,
  unicodeEmojiSections,
} from "./pickerSections";
import { preloadUnicodeEmoji, useUnicodeEmoji } from "./unicodeEmoji";

export type EmojiPickerContent = {
  /**
   * `loading` until the Unicode emoji have arrived, `error` when they could not be loaded (the custom emoji are still
   * offered), `ready` otherwise.
   */
  status: "loading" | "ready" | "error";
  /** Starts loading the Unicode emoji (from the button's hover, focus and opening), unless they are loaded. */
  preload: () => void;
  /** The emoji used often, the custom emoji by category, then the Unicode emoji by group. */
  sections: readonly EmojiPickerSection[];
  labels: EmojiPickerLabels;
  skinTone: SkinTone;
  setSkinTone: (tone: SkinTone) => void;
  /** Counts the emoji of `selection` as used: a Unicode emoji without its skin tone, a custom emoji by its shortcode. */
  remember: (selection: EmojiPickerSelection) => void;
};

/** What an emoji picker shows, with `grid` as the accessible name of its grid. */
export function useEmojiPickerContent(grid: string): EmojiPickerContent {
  const api = useApi();
  const t = useT();
  const locale = useLocale();
  const { data } = useQuery(customEmojisQueryOptions(api));
  const unicode = useUnicodeEmoji(locale);
  const frequentKeys = useFrequentEmojis();
  const skinTone = useStorage(skinToneKey);
  const custom = data?.items ?? [];
  const unicodeSet = unicode.status === "ready" ? unicode.emoji : undefined;
  const frequent = emojisOfKeys(frequentKeys.length > 0 ? frequentKeys : defaultFrequentEmojis, custom, unicodeSet);
  const status = unicode.status === "ready" || unicode.status === "error" ? unicode.status : "loading";
  return {
    status,
    preload: () => {
      preloadUnicodeEmoji(locale);
    },
    sections: [
      frequentSection(frequent, t),
      ...customEmojiSections(custom, t),
      ...(unicodeSet === undefined
        ? status === "loading"
          ? loadingUnicodeEmojiSections(t)
          : []
        : unicodeEmojiSections(unicodeSet, t)),
    ],
    labels: { ...pickerLabels(grid, t), ...skinToneLabels(t) },
    skinTone,
    setSkinTone: (tone) => {
      skinToneKey.write(tone);
    },
    remember: (selection) => {
      // A Unicode emoji counts without its skin tone, so that it is one emoji whichever tone it was picked in.
      rememberEmoji(selection.kind === "custom" ? customEmojiKey(selection.emoji.shortcode) : selection.emoji.unicode);
    },
  };
}
