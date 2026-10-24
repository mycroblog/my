// SPDX-License-Identifier: AGPL-3.0-only
// The Unicode emoji of the picker in the locale shown (ADR-0091 §1): the base data and the names are loaded once per
// locale, when the picker is first opened or its button is hovered or focused, and kept for the life of the page. The
// emoji objects are built once, so the picker's search computes the normalized names of each only once.
import type { PickerUnicodeEmoji } from "@my/ui";
import { useSyncExternalStore } from "react";
import {
  loadUnicodeEmoji,
  loadUnicodeEmojiNames,
  type UnicodeEmojiData,
  type UnicodeEmojiLocale,
  type UnicodeEmojiNames,
  unicodeEmojiLocale,
} from "../unicode";

/** The id of a group of emoji-test.txt, also the icon of its section (@my/ui `EmojiSectionIcon`). */
export type UnicodeGroupId = UnicodeEmojiData["groups"][number]["id"];

/** The Unicode emoji of one group, in emoji-test.txt's order. */
type UnicodeEmojiGroup = { id: UnicodeGroupId; emojis: readonly PickerUnicodeEmoji[] };

/** Every Unicode emoji of the picker, by group, and by the emoji without its skin tone. */
export type UnicodeEmojiSet = { groups: readonly UnicodeEmojiGroup[]; byEmoji: ReadonlyMap<string, PickerUnicodeEmoji> };

/** The loading of the Unicode emoji of a locale: not asked for yet (`idle`), on its way, done, or failed. */
export type UnicodeEmojiState =
  | { status: "idle" }
  | { status: "loading" }
  | { status: "ready"; emoji: UnicodeEmojiSet }
  | { status: "error" };

const idle: UnicodeEmojiState = { status: "idle" };
const loading: UnicodeEmojiState = { status: "loading" };
const failed: UnicodeEmojiState = { status: "error" };

const states = new Map<UnicodeEmojiLocale, UnicodeEmojiState>();
const listeners = new Set<() => void>();

function setState(locale: UnicodeEmojiLocale, state: UnicodeEmojiState): void {
  states.set(locale, state);
  for (const listener of listeners) listener();
}

/**
 * The emoji of `data` named in `locale`. The search finds an emoji by its name and keywords in the locale and, in
 * another locale than `en`, by its `en` name and keywords too (ADR-0091 §3), so the `en` ones join its keywords.
 */
function buildSet(data: UnicodeEmojiData, names: UnicodeEmojiNames, en: UnicodeEmojiNames | undefined): UnicodeEmojiSet {
  const byEmoji = new Map<string, PickerUnicodeEmoji>();
  const groups = data.groups.map((group) => ({
    id: group.id,
    emojis: group.emoji.map(([unicode, , skins]) => {
      const [name, keywords] = names.emoji[unicode] ?? [unicode, []];
      const english = en?.emoji[unicode];
      const emoji: PickerUnicodeEmoji = {
        unicode,
        name,
        keywords: english === undefined ? keywords : [...keywords, english[0], ...english[1]],
        ...(skins === undefined ? {} : { skins: skins.map(([variant]) => ({ unicode: variant, name: names.skins[variant] ?? name })) }),
      };
      byEmoji.set(unicode, emoji);
      return emoji;
    }),
  }));
  return { groups, byEmoji };
}

async function load(locale: UnicodeEmojiLocale): Promise<UnicodeEmojiSet> {
  const [data, names, en] = await Promise.all([
    loadUnicodeEmoji(),
    loadUnicodeEmojiNames(locale),
    locale === "en" ? undefined : loadUnicodeEmojiNames("en"),
  ]);
  return buildSet(data, names, en);
}

/**
 * Starts loading the Unicode emoji named in the locale closest to `locale`, unless they are loaded or on their way; a
 * failed load starts again. Called from the picker button's events (hover, focus, opening), never while rendering.
 */
export function preloadUnicodeEmoji(locale: string): void {
  loadUnicodeEmojiSet(locale).catch(() => undefined);
}

/** The loads on their way or done, by locale; a failed one is started again. */
const loads = new Map<UnicodeEmojiLocale, Promise<UnicodeEmojiSet>>();

/**
 * The Unicode emoji named in the locale closest to `locale`, loading them unless they are loaded or on their way (the
 * completion after ":", ADR-0092 §2, waits for them); a failed load starts again.
 */
export function loadUnicodeEmojiSet(locale: string): Promise<UnicodeEmojiSet> {
  const key = unicodeEmojiLocale(locale);
  const state = states.get(key);
  if (state?.status === "ready") return Promise.resolve(state.emoji);
  const pending = loads.get(key);
  if (pending !== undefined && state?.status === "loading") return pending;
  setState(key, loading);
  const promise = load(key);
  loads.set(key, promise);
  promise.then(
    (emoji) => {
      setState(key, { status: "ready", emoji });
    },
    () => {
      setState(key, failed);
    },
  );
  return promise;
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/** The loading of the Unicode emoji named in the locale closest to `locale`, rendering again as it moves on. */
export function useUnicodeEmoji(locale: string): UnicodeEmojiState {
  const key = unicodeEmojiLocale(locale);
  const snapshot = () => states.get(key) ?? idle;
  return useSyncExternalStore(subscribe, snapshot, snapshot);
}
