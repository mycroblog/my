// SPDX-License-Identifier: AGPL-3.0-only
// The emoji used often on this browser (ADR-0091 §3): the Unicode emoji (without their skin tone) and the custom emoji
// of this server (`:shortcode:`) in one list, ranked by how often and how lately each was used. They are how this
// browser is used, like the theme, so they are kept in localStorage (lib/storage `frequentEmojisKey`), outlive the
// session and are shared by the accounts that sign in here. The picker shows those it still offers; the completion
// after ":" (ADR-0092 §2) ranks them first.
import { frequentEmojisKey, type StoredEmojiUse, useStorage } from "../../../lib/storage";

/** How many emoji are kept and shown as used often. */
const maxFrequentEmojis = 32;

/** How long it takes a use to count half as much (one week): a burst of use long ago gives way to the emoji of today. */
const halfLife = 7 * 24 * 60 * 60 * 1000;

/**
 * The emoji shown as used often before any is used: those that the picker of reactions offers by default (ADR-0052
 * §2), the ones Misskey offers and a few more. Each is fully qualified, the form the server counts reactions in.
 */
export const defaultFrequentEmojis: readonly string[] = [
  "👍",
  "❤️",
  "😆",
  "🤔",
  "😮",
  "🎉",
  "😢",
  "😇",
  "🙏",
  "👀",
  "🔥",
  "💯",
];

/** The key of a custom emoji in the list: its `:shortcode:`, which no Unicode emoji starts with. */
export function customEmojiKey(shortcode: string): string {
  return `:${shortcode}:`;
}

/** The shortcode of the custom emoji of `key`, or `undefined` for a Unicode emoji. */
export function shortcodeOf(key: string): string | undefined {
  return key.length > 2 && key.startsWith(":") && key.endsWith(":") ? key.slice(1, -1) : undefined;
}

/** How much `use` counts at `now`: its count, halved for every `halfLife` since it was used last. */
function weight(use: StoredEmojiUse, now: number): number {
  return use.score * 2 ** (-Math.max(0, now - use.at) / halfLife);
}

/**
 * The list `uses` after a use of `emoji` at `now`: it counts one more than its decayed count, and the list is ranked
 * by decayed count at `now`, the most recent first among equals. Every count decays by the same factor as time goes
 * on, so the order stays right until the next use, and the stored order is the order to show.
 */
export function withUse(uses: readonly StoredEmojiUse[], emoji: string, now: number): StoredEmojiUse[] {
  const previous = uses.find((use) => use.emoji === emoji);
  const used: StoredEmojiUse = { emoji, score: (previous === undefined ? 0 : weight(previous, now)) + 1, at: now };
  return [used, ...uses.filter((use) => use.emoji !== emoji)]
    .toSorted((first, second) => weight(second, now) - weight(first, now) || second.at - first.at)
    .slice(0, maxFrequentEmojis);
}

/** Counts a use of `emoji` (a Unicode emoji without its skin tone, or `customEmojiKey(shortcode)`) now. */
export function rememberEmoji(emoji: string): void {
  frequentEmojisKey.write(withUse(frequentEmojisKey.read(), emoji, Date.now()));
}

/**
 * The emoji used often, the most first (Unicode emoji without their skin tone, custom emoji as `:shortcode:`), rendering
 * again when they change; empty until one is used.
 */
export function useFrequentEmojis(): readonly string[] {
  return useStorage(frequentEmojisKey).map((use) => use.emoji);
}
