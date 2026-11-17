// SPDX-License-Identifier: AGPL-3.0-only

// What the emoji picker of @my/design-system shows (ADR-0091 §1, §3, §7): the emoji used often on this device, the
// server's custom emoji (those without a category first, then a group per category), and every Unicode emoji by the
// nine groups of emoji-test.txt, named in the reader's locale, in the skin tone chosen here; and the search over them.
// The Unicode emoji, with the index their search runs over, are a chunk of their own (unicode/set.ts and its data),
// loaded once per locale when the button that opens a picker is first hovered, focused or pressed (`preload`), or once
// the page is idle for a reader who opened a picker on this device before (`ahead`), and kept for the life of the page; until they arrive the picker offers the frequent and the custom ones. Each part of what
// the picker is given is built once for what it is made of, so that the picker lays out its grid again only when one
// of those changes. The Unicode data comes from the server's EMOJI_VERSION, so the server accepts every emoji it
// offers; the custom emoji of other servers are not offered.

import {
  preloadEmojiPickerWhenIdle,
  type EmojiPickerLabels,
  type EmojiSection,
  type IconName,
  type PickerEmoji,
} from "@my/design-system";
import { useCustomEmojis, type PickerCustomEmoji } from "@my/app-data";
import { startTransition, useEffect, useMemo, useState } from "react";
import {
  defaultFrequent,
  keepTone,
  notePickerOpened,
  pickerOpenedBefore,
  rememberEmoji,
  useFrequentEmojis,
  useTone,
} from "./device";
import { emojiGroups, emojiStrings, type EmojiGroup } from "./strings";
import { loadUnicodeEmoji, loadUnicodeEmojiNames, unicodeEmojiLocale, type UnicodeEmojiLocale } from "./unicode";
import type { UnicodeSet } from "./unicode/set";

/** What the picker is given, but where it opens and what a pick does. */
export type PickerData = {
  sections: readonly EmojiSection[];
  /** The emoji that match `query`, best first; within a match as good, those `first` picks first. */
  search: (query: string, first?: (emoji: PickerEmoji) => boolean) => readonly PickerEmoji[];
  labels: EmojiPickerLabels;
  tone: number;
  onToneChange: (tone: number) => void;
  /**
   * Starts loading the Unicode emoji, unless they are loaded or on their way: before the picker first opens, from the
   * press (its pointerdown), the hover or the focus of the button that opens it.
   */
  preload: () => void;
  /** Notes that the picker opened on this device. */
  onOpen: () => void;
  /**
   * Fetches what a picker at hand needs once the page is idle: its code, and its emoji too for a reader who opened a
   * picker on this device before; returns what cancels it. For an effect of a screen that offers a picker.
   */
  ahead: () => () => void;
};

/** The icon of each group's category, until the Unicode emoji arrive with theirs. */
const groupIcons: Readonly<Record<EmojiGroup, IconName>> = {
  smileys: "emojiSmileys",
  people: "emojiPeople",
  nature: "emojiNature",
  food: "emojiFood",
  travel: "emojiTravel",
  activities: "emojiActivities",
  objects: "emojiObjects",
  symbols: "emojiSymbols",
  flags: "emojiFlags",
};

/** The emoji without its skin tone modifiers: how it counts as used, whichever tone it was picked in. */
const withoutTone = (text: string) => text.replace(/[\u{1F3FB}-\u{1F3FF}]/gu, "");

/** The Unicode emoji and the search's code, once loaded. */
type Lib = typeof import("./unicode/set");

/** The loading of the Unicode emoji of a locale: on its way, done, or failed. */
type Loaded = { status: "loading" } | { status: "ready"; set: UnicodeSet; lib: Lib } | { status: "error" };

const loaded = new Map<UnicodeEmojiLocale, Loaded>();
const listeners = new Set<() => void>();

/** Starts loading the Unicode emoji of the locale closest to `locale`, unless they are loaded or on their way. */
function load(locale: string): void {
  const key = unicodeEmojiLocale(locale);
  const now = loaded.get(key);
  if (now !== undefined && now.status !== "error") return;
  // Only what the picker shows tells the screens: the emoji arrived (or failed, to be asked for again), never that
  // they are on their way, so that the press that asks for them redraws nothing.
  loaded.set(key, { status: "loading" });
  const set = (state: Loaded) => {
    loaded.set(key, state);
    for (const l of listeners) l();
  };
  Promise.all([
    import("./unicode/set"),
    loadUnicodeEmoji(),
    loadUnicodeEmojiNames(key),
    key === "en" ? undefined : loadUnicodeEmojiNames("en"),
  ])
    .then(([lib, data, names, en]) => {
      set({ status: "ready", set: lib.buildSet(data, names, en), lib });
    })
    .catch(() => {
      set({ status: "error" });
    });
}

/** The loaded emoji of `key`, if they arrived. */
const readyOf = (key: UnicodeEmojiLocale) => {
  const state = loaded.get(key);
  return state?.status === "ready" ? state : undefined;
};

/**
 * The Unicode emoji of `locale` and the search's code; undefined until they are asked for (`load`) and arrive. Their
 * arrival redraws the screens as a transition, which yields to the browser, so that it never holds back the frame that
 * answers the press that asked for them (the opening of a picker).
 */
function useUnicode(locale: string): { set: UnicodeSet; lib: Lib } | undefined {
  const key = unicodeEmojiLocale(locale);
  const [shown, setShown] = useState(() => ({ key, ready: readyOf(key) }));
  useEffect(() => {
    const listener = () => {
      startTransition(() => {
        setShown({ key, ready: readyOf(key) });
      });
    };
    listeners.add(listener);
    // What arrived between the first render and the subscription.
    if (readyOf(key) !== undefined) listener();
    return () => {
      listeners.delete(listener);
    };
  }, [key]);
  return shown.key === key ? shown.ready : readyOf(key);
}

/** Whether the Unicode emoji of `locale` arrived: the search runs over every emoji; until then it finds none. */
export const useUnicodeReady = (locale: string): boolean => useUnicode(locale) !== undefined;

/** A custom emoji as the picker offers it, keyed by `:shortcode:`. */
const customOf = (e: PickerCustomEmoji): PickerEmoji => ({
  key: `:${e.shortcode}:`,
  name: `:${e.shortcode}:`,
  image: { url: e.url, staticUrl: e.static_url },
});

/**
 * Counts the emoji of a reaction as used on this device: its `name` as a reaction takes it (a Unicode emoji, in a skin
 * tone or not, or a custom emoji's shortcode); a custom emoji of another server (`shortcode@domain`) is not offered.
 */
export function rememberReaction(name: string): void {
  if (name.includes("@")) return;
  rememberEmoji(/^\w+$/u.test(name) ? `:${name}:` : withoutTone(name));
}

const noTones = ["", "", "", "", ""];

/** What the emoji picker shows in `locale`. */
export function usePickerData(locale: string): PickerData {
  const custom = useCustomEmojis();
  const unicode = useUnicode(locale);
  const used = useFrequentEmojis();
  const tone = useTone();
  const set = unicode?.set;
  const lib = unicode?.lib;
  const strings = useMemo(() => emojiStrings(locale, set?.tones ?? noTones), [locale, set]);
  const customEmojis = useMemo(() => custom.map(customOf), [custom]);
  const sections = useMemo((): EmojiSection[] => {
    const byShortcode = new Map(customEmojis.map((e) => [e.key, e]));
    const frequent = (used.length > 0 ? used : defaultFrequent).flatMap((key) => {
      const emoji = byShortcode.get(key) ?? set?.byKey.get(key);
      return emoji === undefined ? [] : [emoji];
    });
    const categories = [...new Set(custom.flatMap((e) => (e.category === undefined ? [] : [e.category])))];
    const of = (category: string | undefined) => customEmojis.filter((_, i) => custom[i]?.category === category);
    return [
      { id: "frequent", label: strings.frequent, icon: "time", emojis: frequent },
      ...(custom.length === 0
        ? []
        : [
            {
              id: "custom",
              label: strings.custom,
              icon: "emojiCustom" as const,
              emojis: of(undefined),
              groups: categories.map((c) => ({ label: c, emojis: of(c) })),
            },
          ]),
      ...(set?.groups ?? emojiGroups.map((id) => ({ id, icon: groupIcons[id], emojis: [] }))).map((g) => ({
        id: g.id,
        label: strings.groups[g.id],
        icon: g.icon,
        emojis: g.emojis,
      })),
    ];
  }, [custom, customEmojis, set, used, strings]);
  // The search runs over the index built with the Unicode emoji, the custom emoji first; until they arrive it has
  // nothing to run over.
  const customIndex = useMemo(
    () =>
      lib === undefined
        ? []
        : custom.map((e, i) => lib.entryOf(customEmojis[i] ?? customOf(e), { name: e.shortcode, keywords: e.aliases })),
    [custom, customEmojis, lib],
  );
  const unicodeIndex = set?.index;
  const search = (query: string, first?: (emoji: PickerEmoji) => boolean): readonly PickerEmoji[] =>
    lib === undefined || unicodeIndex === undefined ? [] : lib.searchIndex([customIndex, unicodeIndex()], query, first);
  return {
    sections,
    search,
    labels: strings.labels,
    tone,
    onToneChange: keepTone,
    preload: () => {
      load(locale);
    },
    onOpen: notePickerOpened,
    ahead: () =>
      preloadEmojiPickerWhenIdle(
        pickerOpenedBefore()
          ? () => {
              load(locale);
            }
          : undefined,
      ),
  };
}
