// SPDX-License-Identifier: AGPL-3.0-only

// What the emoji picker keeps on the device (ADR-0091 §3, the head of the ADR): the emoji used often, ranked by how
// often and how lately each was used, and the skin tone chosen. They are how this device is used, like the theme, so
// they outlive the session and are shared by the accounts that sign in here (@my/platform's device values); and
// whether a picker was ever opened here, which fetches the emoji ahead for the next time. A device
// that keeps nothing forgets them with the page.

import { readDeviceValue, writeDeviceValue } from "@my/platform";
import { useSyncExternalStore } from "react";

/** One emoji used: a Unicode emoji without its skin tone, or a custom one as `:shortcode:`; its decayed count; when. */
type Use = { readonly emoji: string; readonly score: number; readonly at: number };

const frequentKey = "my:emoji-frequent";
const toneKey = "my:emoji-tone";
const openedKey = "my:emoji-picker-opened";

/**
 * Whether a picker was opened on this device before: such a reader gets the emoji ahead, once the page is idle, so
 * that their next opening does not wait for them; a reader who never opened one pays nothing for them.
 */
export const pickerOpenedBefore = () => readDeviceValue(openedKey) !== null;

/** Notes that a picker was opened on this device. */
export function notePickerOpened(): void {
  if (!pickerOpenedBefore()) writeDeviceValue(openedKey, "1");
}

/** How many emoji are kept and shown as used often. */
const maxFrequent = 32;

/** How long it takes a use to count half as much (one week): a burst of use long ago gives way to today's emoji. */
const halfLife = 7 * 24 * 60 * 60 * 1000;

/**
 * The emoji shown as used often before any is: those a reaction picker offers first (ADR-0051 §2), fully qualified,
 * the form the server counts reactions in.
 */
export const defaultFrequent: readonly string[] = [
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

const isUse = (u: unknown): u is Use =>
  typeof u === "object" &&
  u !== null &&
  typeof (u as Use).emoji === "string" &&
  typeof (u as Use).score === "number" &&
  typeof (u as Use).at === "number";

function readUses(): readonly Use[] {
  try {
    const parsed: unknown = JSON.parse(readDeviceValue(frequentKey) ?? "[]");
    return Array.isArray(parsed) ? parsed.filter(isUse) : [];
  } catch {
    return [];
  }
}

/** How much `use` counts at `now`: its count, halved for every `halfLife` since it was last used. */
const weight = (use: Use, now: number) => use.score * 2 ** (-Math.max(0, now - use.at) / halfLife);

/**
 * `uses` after a use of `emoji` at `now`: it counts one more than its decayed count, and the list is ranked by decayed
 * count at `now`, the latest first among equals. Every count decays by the same factor as time goes on, so the stored
 * order stays the order to show until the next use.
 */
function withUse(uses: readonly Use[], emoji: string, now: number): Use[] {
  const was = uses.find((u) => u.emoji === emoji);
  const used: Use = { emoji, score: (was === undefined ? 0 : weight(was, now)) + 1, at: now };
  return [used, ...uses.filter((u) => u.emoji !== emoji)]
    .toSorted((a, b) => weight(b, now) - weight(a, now) || b.at - a.at)
    .slice(0, maxFrequent);
}

const listeners = new Set<() => void>();
let frequent: readonly string[] | undefined;
let tone: number | undefined;

const changed = () => {
  frequent = undefined;
  tone = undefined;
  for (const l of listeners) l();
};

const subscribe = (listener: () => void) => {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
};

const frequentSnapshot = () => (frequent ??= readUses().map((u) => u.emoji));
const toneSnapshot = () => {
  if (tone === undefined) {
    const n = Number(readDeviceValue(toneKey) ?? "0");
    tone = Number.isInteger(n) && n >= 0 && n <= 5 ? n : 0;
  }
  return tone;
};

/** Counts a use of `emoji` (a Unicode emoji without its skin tone, or `:shortcode:`) now. */
export function rememberEmoji(emoji: string): void {
  writeDeviceValue(frequentKey, JSON.stringify(withUse(readUses(), emoji, Date.now())));
  changed();
}

/** Keeps the skin tone chosen: 0 for none, 1 to 5 light to dark. */
export function keepTone(next: number): void {
  writeDeviceValue(toneKey, String(next));
  changed();
}

/** The emoji used often on this device, the most first; empty until one is used. */
export function useFrequentEmojis(): readonly string[] {
  return useSyncExternalStore(subscribe, frequentSnapshot, frequentSnapshot);
}

/** The skin tone chosen on this device. */
export function useTone(): number {
  return useSyncExternalStore(subscribe, toneSnapshot, toneSnapshot);
}
