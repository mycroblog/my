// SPDX-License-Identifier: AGPL-3.0-only
// The emoji picker of posts, profiles and reactions (ADR-0091 §3, §5, §6; ADR-0097; ADR-0051 §4; ADR-0052 §5). It holds
// no data (ADR-0031): the app passes the sections to show (the emoji used often, the custom emoji by category, the
// Unicode groups), each with its heading, its icon and its emoji, and what to do with the one picked.
//
// From the top: a search field and the skin tone menu button, a toolbar of one button per section, and one grid of all
// the sections, scrolled as a whole. The grid is one list of rows (ADR-0097 §1): per section a heading row, then its
// emoji cut into rows of as many columns as the grid's width holds. Only the rows in view and four on each side are
// rendered (TanStack Virtual), and so is the row of the active cell wherever it is, so focus is never lost to the
// virtualization (§2). The grid tells its size and each row's and cell's place with aria-rowcount / aria-colcount /
// aria-rowindex / aria-colindex, and groups the rendered rows of a section in a rowgroup named by its heading (§3).
//
// Keyboard (WAI-ARIA APG, grid pattern, with a roving tabindex; ADR-0097 §4): Tab goes from the search field to the skin
// tone button, the toolbar and the grid, where one cell takes focus. In the grid, left and right follow the writing
// direction:
//
//   Right / Left            the next / previous emoji, on to the next / previous row and section
//   Down / Up               the emoji row below / above (headings skipped), in the same column or the row's last
//   Home / End              the first / last emoji of the row
//   Control + Home / End    the first / last emoji of the grid
//   Page Down / Page Up     as many rows down / up as the grid shows
//   Enter, Space, a press   pick the emoji; with Shift held (Shift+Enter, Shift+click) the app is told to keep the
//                           picker open, and focus stays on it; a touch never does
//
// The section buttons are not APG tabs, since there are no panels to switch: a button scrolls the grid to its
// section's heading and moves focus to the section's first emoji, and the button of the section at the top of the
// grid has aria-current="true", worked out from the scroll offset (§5). The buttons scroll smoothly only when the user
// allows motion.
//
// A search ranks the emoji of every section by `searchEmojis` (exported for the completion of ADR-0092) into one
// section of results, disables the section buttons, and announces the count in a role="status" region once typing
// pauses.
//
// While the app is still loading emoji (`loading`), the picker is there at once with its search field and section
// buttons (web-client/16-look-states-and-wording.md G15): the buttons of the sections still without emoji are disabled,
// the grid is aria-busy="true" and shows skeleton rows after the emoji it has, and the status region says
// `labels.loading`.
import {
  defaultRangeExtractor,
  observeElementRect,
  type Range,
  type Virtualizer,
  type VirtualItem,
  useVirtualizer,
} from "@tanstack/react-virtual";
import { type JSX, type KeyboardEvent, useEffect, useRef, useState } from "react";
import { mergeProps, type PressEvent, useFocusRing, useHover, useLocale, usePress } from "react-aria";
import { type Selection, Toolbar } from "react-aria-components";
import { tv } from "tailwind-variants";
import { Button } from "./Button";
import { CustomEmoji, type CustomEmojiImage } from "./CustomEmoji";
import {
  ActivityEmojiIcon,
  CheckIcon,
  CustomEmojiIcon,
  EmojiIcon,
  FlagEmojiIcon,
  FoodEmojiIcon,
  FrequentEmojiIcon,
  NatureEmojiIcon,
  ObjectEmojiIcon,
  PeopleEmojiIcon,
  SymbolEmojiIcon,
  TravelEmojiIcon,
} from "./Icons";
import { Menu, MenuItem, MenuTrigger } from "./Menu";
import { Skeleton } from "./Skeleton";
import { countAnnounceDelay } from "./TextArea";
import { TextField } from "./TextField";
import { Tooltip, TooltipTrigger } from "./Tooltip";

/** A custom emoji the picker offers: its image and `:shortcode:`, and other names that the search finds it by. */
export type PickerCustomEmoji = CustomEmojiImage & {
  /** Other names that the search finds the emoji by, ranked as keywords. */
  aliases?: readonly string[] | undefined;
};

/** One skin tone variant of a Unicode emoji. */
export type PickerSkinVariant = {
  /** The fully-qualified emoji with its skin tone modifiers ("👋🏽"). */
  unicode: string;
  /** Its name in the locale shown, which names its row (CLDR annotationsDerived: "waving hand: medium skin tone"). */
  name: string;
};

/** A Unicode emoji the picker offers. */
export type PickerUnicodeEmoji = {
  /** The fully-qualified emoji without a skin tone ("👋"). */
  unicode: string;
  /** Its short name in the locale shown (CLDR), which names its row and ranks first in the search. */
  name: string;
  /** Other words that the search finds it by (the CLDR keywords, the `en` name), ranked as keywords. */
  keywords?: readonly string[] | undefined;
  /**
   * Its skin tone variants, in any order; the picker shows and picks the one whose every modifier is the chosen tone
   * (`skinToneVariant`), so an emoji of several people gets the same tone for all of them.
   */
  skins?: readonly PickerSkinVariant[] | undefined;
};

/** An emoji the picker offers: a custom emoji (it has a `shortcode`) or a Unicode emoji (it has `unicode`). */
export type PickerEmoji = PickerCustomEmoji | PickerUnicodeEmoji;

/**
 * The icon of a section's button: `frequent` for the emoji used often, `custom` for the custom emoji, and the id of
 * the group of emoji-test.txt for the Unicode emoji of that group.
 */
export type EmojiSectionIcon =
  | "frequent"
  | "custom"
  | "smileys-emotion"
  | "people-body"
  | "animals-nature"
  | "food-drink"
  | "travel-places"
  | "activities"
  | "objects"
  | "symbols"
  | "flags";

/** One section of the grid, in the order the app lists them. */
export type EmojiPickerSection = {
  /** Unique among the sections, and the same from render to render. */
  id: string;
  /** The heading of the section in the grid, which is also the name and tooltip of its button. */
  heading: string;
  icon: EmojiSectionIcon;
  /** The emoji in the order to show them. An emoji may also be in another section (the frequent ones). */
  emojis: readonly PickerEmoji[];
};

/** The skin tones of the picker: the default yellow and the five Fitzpatrick modifiers (U+1F3FB..U+1F3FF). */
export type SkinTone = "none" | "light" | "medium-light" | "medium" | "medium-dark" | "dark";

/** The skin tones in the order the menu lists them. */
export const skinTones: readonly SkinTone[] = ["none", "light", "medium-light", "medium", "medium-dark", "dark"];

/** The modifier of each skin tone; the default has none. */
const skinToneModifiers: Readonly<Record<SkinTone, string>> = {
  none: "",
  light: "\u{1F3FB}",
  "medium-light": "\u{1F3FC}",
  medium: "\u{1F3FD}",
  "medium-dark": "\u{1F3FE}",
  dark: "\u{1F3FF}",
};

const modifierPattern = /[\u{1F3FB}-\u{1F3FF}]/gu;

/** The raised hand that the skin tone button and menu show each tone with. */
const toneHand = "✋";

/**
 * The variant of `emoji` in `tone`: the one whose every skin tone modifier is the tone's, or `undefined` for the
 * default tone and for an emoji without such a variant.
 */
export function skinToneVariant(emoji: PickerUnicodeEmoji, tone: SkinTone): PickerSkinVariant | undefined {
  if (tone === "none") return undefined;
  const modifier = skinToneModifiers[tone];
  return emoji.skins?.find((variant) => {
    const modifiers = variant.unicode.match(modifierPattern);
    return modifiers !== null && modifiers.every((found) => found === modifier);
  });
}

/** The emoji picked, and whether the app should keep the picker open (picked with Shift held, not by touch). */
export type EmojiPickerSelection =
  | { kind: "custom"; emoji: PickerCustomEmoji; keepOpen: boolean }
  | {
      kind: "unicode";
      emoji: PickerUnicodeEmoji;
      /** The emoji to insert: its variant in the chosen skin tone when it has one, or else `emoji.unicode`. */
      text: string;
      keepOpen: boolean;
    };

/** The words of the picker, which the app localizes (web-client/README.md A2). */
export type EmojiPickerLabels = {
  /** The label of the search field. */
  search: string;
  /** The accessible name of the grid of emoji. */
  grid: string;
  /** The accessible name of the toolbar of section buttons. */
  categories: string;
  /** The heading of the one section of search results. */
  results: string;
  /** What the status region says once typing pauses: the number of emoji found ("12 emoji"). */
  resultCount: (count: number) => string;
  /** What the grid says when no emoji matches the search, or there are none. */
  empty: string;
  /** What the status region says while `loading`. */
  loading?: string | undefined;
  /** The name and tooltip of the skin tone button and the name of its menu; needed for the button to show. */
  skinTone?: string | undefined;
  /** The name of each skin tone in the menu; needed for the button to show. */
  skinTones?: Readonly<Record<SkinTone, string>> | undefined;
};

export type EmojiPickerProps = {
  /**
   * The sections of the grid, in order. A section without emoji is left out, and so is its button, except while
   * `loading`, when its button shows disabled.
   */
  sections: readonly EmojiPickerSection[];
  /** Whether the app is still loading emoji: the grid shows skeleton rows after the emoji it has. */
  loading?: boolean | undefined;
  /**
   * Called with the emoji picked. Unless `keepOpen` is true, the app closes the picker (and returns focus to its
   * field); with `keepOpen`, focus stays on the emoji picked so that the user can go on picking.
   */
  onSelect: (selection: EmojiPickerSelection) => void;
  /** The skin tone the Unicode emoji with variants are shown and picked in; `none` (the default yellow) by default. */
  skinTone?: SkinTone | undefined;
  /**
   * Called with the skin tone chosen in the menu; the app keeps it and passes it back as `skinTone`. Without it, or
   * without `labels.skinTone` and `labels.skinTones`, there is no skin tone button.
   */
  onSkinToneChange?: ((tone: SkinTone) => void) | undefined;
  labels: EmojiPickerLabels;
  /** Whether animated emoji may move when the user allows motion; the default is the app's `AutoplayContext`. */
  allowMotion?: boolean | undefined;
  /** Focus the search field when the picker appears (in a popover the user just opened). */
  autoFocus?: boolean | undefined;
};

const picker = tv({
  slots: {
    root: "relative flex min-w-0 flex-col gap-2",
    top: "flex min-w-0 items-end gap-2",
    tone: "text-xl",
    toneItem: "flex min-w-0 grow items-center gap-2",
    toneHand: "text-xl",
    toneCheck: "ms-auto size-4 shrink-0",
    toolbar: "flex min-w-0 flex-wrap gap-0.5",
    // The section at the top of the grid shows by a ground behind its icon, not by the color of the icon alone (WCAG 1.4.1).
    tab: "aria-current:bg-soft aria-current:text-accent",
    // The section buttons scroll the grid through the virtualizer, smoothly only when the user allows motion
    // (web-client/README.md D6); a CSS scroll-behavior would fight the virtualizer's own scrolling (ADR-0097 §2).
    grid: "max-h-64 min-w-0 overflow-y-auto overscroll-contain outline-none",
    rows: "relative w-full",
    // The rows are placed by the virtualizer; their heights are what the probes measure.
    header: "flex items-end pt-2 pb-1 text-xs font-extrabold text-fg-muted",
    row: "flex gap-1",
    // As large as a tap target needs (D4); the emoji is as high as the text of the cell.
    item: [
      "flex size-10 shrink-0 cursor-default items-center justify-center rounded-md text-2xl outline-none pointer-coarse:size-11",
      "data-focus-visible:focus-ring data-hovered:bg-surface-sunken data-pressed:bg-surface-sunken",
    ],
    // Copies of a heading and two cells, never seen, that tell the sizes of the rows and the columns (ADR-0097 §1).
    probe: "pointer-events-none invisible absolute top-0 inset-s-0 flex items-start gap-1",
    empty: "p-2 text-sm text-fg-muted",
    // The rows of the emoji still loading, in the size of the emoji rows.
    skeleton: "flex flex-col gap-1 pt-1",
    skeletonCell: "size-10 shrink-0 rounded-md pointer-coarse:size-11",
    status: "sr-only",
  },
});

/**
 * The search text as the emoji are compared with it: NFKC, lower case, katakana as hiragana, without spaces or
 * colons around it. The names and keywords of the emoji are compared in the same form.
 */
export function normalizeEmojiSearch(text: string): string {
  return text
    .normalize("NFKC")
    .toLowerCase()
    .replace(/[ァ-ヶ]/gu, (katakana) => String.fromCodePoint((katakana.codePointAt(0) ?? 0) - 0x60))
    .trim()
    .replace(/^:+|:+$/gu, "");
}

type SearchForms = { emoji: string | undefined; name: string; keywords: readonly string[] };

/** The normalized names of each emoji object, computed once: the search compares every emoji on every keystroke. */
const searchForms = new WeakMap<PickerEmoji, SearchForms>();

function searchFormsOf(emoji: PickerEmoji): SearchForms {
  let forms = searchForms.get(emoji);
  if (forms === undefined) {
    forms =
      "unicode" in emoji
        ? {
            emoji: normalizeEmojiSearch(emoji.unicode),
            name: normalizeEmojiSearch(emoji.name),
            keywords: (emoji.keywords ?? []).map(normalizeEmojiSearch),
          }
        : {
            emoji: undefined,
            name: normalizeEmojiSearch(emoji.shortcode),
            keywords: (emoji.aliases ?? []).map(normalizeEmojiSearch),
          };
    searchForms.set(emoji, forms);
  }
  return forms;
}

/**
 * How well `emoji` matches the normalized search text `query` (`normalizeEmojiSearch`), best first: 0 when its name
 * (the shortcode of a custom emoji) is the text or it is the emoji itself, 1 when its name starts with it, 2 when a
 * keyword (an alias of a custom emoji) starts with it, 3 when its name or a keyword contains it; `undefined` when it
 * does not match or the text is empty.
 */
export function emojiMatchRank(emoji: PickerEmoji, query: string): number | undefined {
  if (query === "") return undefined;
  const forms = searchFormsOf(emoji);
  if (forms.name === query || forms.emoji === query) return 0;
  if (forms.name.startsWith(query)) return 1;
  if (forms.keywords.some((keyword) => keyword.startsWith(query))) return 2;
  if (forms.name.includes(query) || forms.keywords.some((keyword) => keyword.includes(query))) return 3;
  return undefined;
}

/** The key an emoji is the same by in every section: the `:shortcode:` of a custom emoji, a Unicode emoji itself. */
function emojiKey(emoji: PickerEmoji): string {
  return "unicode" in emoji ? emoji.unicode : `:${emoji.shortcode}:`;
}

/**
 * The emoji of `emojis` that match the search text `search` (normalized here), ranked by `emojiMatchRank`, and in the
 * order given within a rank; each emoji once, at its first place. The picker's search and the completion after ":"
 * (ADR-0092) use it.
 */
export function searchEmojis(emojis: Iterable<PickerEmoji>, search: string): PickerEmoji[] {
  const query = normalizeEmojiSearch(search);
  const ranks: PickerEmoji[][] = [[], [], [], []];
  const seen = new Set<string>();
  for (const emoji of emojis) {
    const key = emojiKey(emoji);
    if (seen.has(key)) continue;
    const rank = emojiMatchRank(emoji, query);
    if (rank === undefined) continue;
    seen.add(key);
    ranks[rank]?.push(emoji);
  }
  return ranks.flat();
}

/** One emoji of the grid, under an id unique in the grid. */
type Item = { id: string; emoji: PickerEmoji };

type Group = { id: string; heading: string; items: Item[] };

function* emojisOf(sections: readonly EmojiPickerSection[]): Generator<PickerEmoji> {
  for (const section of sections) yield* section.emojis;
}

const sectionIcons: Readonly<Record<EmojiSectionIcon, () => JSX.Element>> = {
  frequent: FrequentEmojiIcon,
  custom: CustomEmojiIcon,
  "smileys-emotion": EmojiIcon,
  "people-body": PeopleEmojiIcon,
  "animals-nature": NatureEmojiIcon,
  "food-drink": FoodEmojiIcon,
  "travel-places": TravelEmojiIcon,
  activities: ActivityEmojiIcon,
  objects: ObjectEmojiIcon,
  symbols: SymbolEmojiIcon,
  flags: FlagEmojiIcon,
};

function SectionIcon({ icon }: { icon: EmojiSectionIcon }) {
  const Icon = sectionIcons[icon];
  return <Icon />;
}

/** The share of the grid's height at its top where a heading counts as the top of the grid. */
const topBand = 0.1;

/** How many rows the grid renders beyond each end of its view (ADR-0097 §2). */
const overscan = 4;

/** The skin tone `keys` holds, from the menu's selection. */
function toneOf(keys: Selection): SkinTone | undefined {
  if (keys === "all") return undefined;
  return skinTones.find((tone) => keys.has(tone));
}

type SkinToneButtonProps = {
  value: SkinTone;
  onChange: (tone: SkinTone) => void;
  label: string;
  names: Readonly<Record<SkinTone, string>>;
};

/**
 * The menu button of the skin tone (WAI-ARIA APG, menu button pattern): the raised hand in the tone chosen, and a menu
 * of the six tones with single selection (role="menuitemradio"), the chosen one also marked by a check.
 */
function SkinToneButton({ value, onChange, label, names }: SkinToneButtonProps) {
  const styles = picker();
  return (
    <TooltipTrigger>
      <MenuTrigger>
        <Button variant="ghost" size="compact" aria-label={label}>
          <span aria-hidden="true" className={styles.tone()}>
            {toneHand + skinToneModifiers[value]}
          </span>
        </Button>
        <Menu
          aria-label={label}
          placement="bottom end"
          selectionMode="single"
          disallowEmptySelection
          selectedKeys={[value]}
          onSelectionChange={(keys) => {
            const tone = toneOf(keys);
            if (tone !== undefined) onChange(tone);
          }}
        >
          {skinTones.map((tone) => (
            <MenuItem key={tone} id={tone} textValue={names[tone]}>
              <span className={styles.toneItem()}>
                <span aria-hidden="true" className={styles.toneHand()}>
                  {toneHand + skinToneModifiers[tone]}
                </span>
                {names[tone]}
                {tone === value && (
                  <span className={styles.toneCheck()}>
                    <CheckIcon />
                  </span>
                )}
              </span>
            </MenuItem>
          ))}
        </Menu>
      </MenuTrigger>
      <Tooltip>{label}</Tooltip>
    </TooltipTrigger>
  );
}

/** What the status region announced last: the count of the search `query`. */
type Announced = { query: string; count: number };

/** The sizes in CSS px that the rows and columns are laid out by, measured on the probes (ADR-0097 §1). */
type Metrics = {
  /** The inner width of the grid. */
  width: number;
  /** The inline and block size of a cell. */
  cell: number;
  cellBlock: number;
  /** The distance from one cell to the next: the cell and the gap after it. */
  pitch: number;
  /** The block size of a heading row. */
  header: number;
};

/**
 * What the grid is laid out by until it is measured, and where nothing is laid out (jsdom): the 22 rem popover at
 * 16 px, 40 px cells 4 px apart, and a heading of 0.75 rem text with its padding.
 */
const fallbackMetrics: Metrics = { width: 352, cell: 40, cellBlock: 40, pitch: 44, header: 28 };

/** The grid's height until it is measured: its largest (max-h-64). */
const fallbackHeight = 256;

/** `value` when it is a size something was laid out with, or else `fallback`. */
function laidOut(value: number | undefined, fallback: number): number {
  return value !== undefined && value > 0 ? value : fallback;
}

/** The metrics of the grid from its probes: a heading and two cells in a row. */
function measure(grid: HTMLElement, probe: HTMLElement): Metrics {
  const [heading, first, second] = [...probe.children].filter((child) => child instanceof HTMLElement);
  return {
    width: laidOut(grid.clientWidth, fallbackMetrics.width),
    cell: laidOut(first?.offsetWidth, fallbackMetrics.cell),
    cellBlock: laidOut(first?.offsetHeight, fallbackMetrics.cellBlock),
    pitch: laidOut(
      first === undefined || second === undefined ? undefined : Math.abs(second.offsetLeft - first.offsetLeft),
      fallbackMetrics.pitch,
    ),
    header: laidOut(heading?.offsetHeight, fallbackMetrics.header),
  };
}

function sameMetrics(first: Metrics, second: Metrics): boolean {
  return (
    first.width === second.width &&
    first.cell === second.cell &&
    first.cellBlock === second.cellBlock &&
    first.pitch === second.pitch &&
    first.header === second.header
  );
}

/** A heading row of a section, or a row of its emoji: the cells `first` to `last` of the layout, inclusive. */
type GridRow =
  | { kind: "header"; key: string; group: Group }
  | { kind: "emoji"; key: string; group: Group; first: number; last: number };

/** One emoji of the layout: its row (an index of `rows`) and its column, from 0. */
type Cell = { item: Item; row: number; column: number };

type Layout = {
  rows: GridRow[];
  cells: Cell[];
  /** The block size of each row and where it starts, in px; all rows have fixed sizes. */
  sizes: number[];
  starts: number[];
  total: number;
  /** The indexes of the heading rows, in order. */
  headerRows: number[];
  /** The heading row and the first cell of each section, by its id. */
  headerRowOf: Map<string, number>;
  firstCellOf: Map<string, number>;
};

/** The rows of `groups` (ADR-0097 §1): per group a heading row, then its emoji in rows of `columns`. */
function layOut(groups: readonly Group[], columns: number, rowSize: number, headerSize: number): Layout {
  const layout: Layout = {
    rows: [],
    cells: [],
    sizes: [],
    starts: [],
    total: 0,
    headerRows: [],
    headerRowOf: new Map(),
    firstCellOf: new Map(),
  };
  const add = (row: GridRow, size: number) => {
    layout.rows.push(row);
    layout.sizes.push(size);
    layout.starts.push(layout.total);
    layout.total += size;
  };
  for (const group of groups) {
    layout.headerRows.push(layout.rows.length);
    layout.headerRowOf.set(group.id, layout.rows.length);
    layout.firstCellOf.set(group.id, layout.cells.length);
    add({ kind: "header", key: group.id, group }, headerSize);
    for (let start = 0; start < group.items.length; start += columns) {
      const row = layout.rows.length;
      const first = layout.cells.length;
      for (const [column, item] of group.items.slice(start, start + columns).entries())
        layout.cells.push({ item, row, column });
      add(
        { kind: "emoji", key: `${group.id}\n${String(start / columns)}`, group, first, last: layout.cells.length - 1 },
        rowSize,
      );
    }
  }
  return layout;
}

/**
 * The cell to move to in row `toRow` (clamped to the grid) from `from`, in the same column or the row's last: a heading
 * row gives way to the next emoji row in `step`'s direction, or the other way at the edge of the grid.
 */
function cellInRow(layout: Layout, from: Cell, toRow: number, step: 1 | -1): number | undefined {
  let index = Math.max(0, Math.min(toRow, layout.rows.length - 1));
  if (layout.rows[index]?.kind === "header")
    index = layout.rows[index + step] === undefined ? index - step : index + step;
  const row = layout.rows[index];
  if (row?.kind !== "emoji") return undefined;
  return Math.min(row.first + from.column, row.last);
}

/** Reports the size of the grid only once it is laid out, so that a grid not laid out (jsdom) keeps `initialRect`. */
const observeLaidOutRect: typeof observeElementRect = (instance, callback) =>
  observeElementRect(instance, (rect) => {
    if (rect.width > 0 && rect.height > 0) callback(rect);
  });

type VirtualRowsOptions = {
  count: number;
  sizes: readonly number[];
  getScrollElement: () => HTMLElement | null;
  getItemKey: (index: number) => string;
  rangeExtractor: (range: Range) => number[];
};

type VirtualRows = {
  virtualItems: VirtualItem[];
  totalSize: number;
  scrollOffset: number;
  viewportHeight: number;
  /** Scrolls row `index` into view (`auto`) or to the top (`start`); called in event handlers only. */
  scrollToIndex: (index: number, align: "auto" | "start", smooth: boolean) => void;
};

/**
 * The virtualizer of the grid's rows, read into plain values. As in `Feed` (web-client/README.md R4), the virtualizer
 * is one mutable object whose methods give new results after a scroll, which a memoized read would miss, so this hook
 * alone opts out of React Compiler; its caller sees plain values and is compiled as usual. The rows have fixed sizes,
 * so they are never measured, and `getItemKey` is wrapped anew on every render so that new sizes are always read.
 */
function useVirtualRows(options: VirtualRowsOptions): VirtualRows {
  // eslint-disable-next-line restrict/syntax -- R4: the virtualizer is a mutable object React Compiler would read stale (see above).
  "use no memo";
  // eslint-disable-next-line react/incompatible-library -- R4: this hook opts out of the compiler for this library (see above); Oxlint's port of the rule reports inside "use no memo" too.
  const virtualizer: Virtualizer<HTMLElement, HTMLElement> = useVirtualizer<HTMLElement, HTMLElement>({
    count: options.count,
    getScrollElement: options.getScrollElement,
    estimateSize: (index) => options.sizes[index] ?? 0,
    getItemKey: (index) => options.getItemKey(index),
    rangeExtractor: options.rangeExtractor,
    overscan,
    initialRect: { width: fallbackMetrics.width, height: fallbackHeight },
    observeElementRect: observeLaidOutRect,
  });
  return {
    virtualItems: virtualizer.getVirtualItems(),
    totalSize: virtualizer.getTotalSize(),
    scrollOffset: virtualizer.scrollOffset ?? 0,
    viewportHeight: virtualizer.scrollRect?.height ?? fallbackHeight,
    scrollToIndex: (index, align, smooth) => {
      virtualizer.scrollToIndex(index, { align, behavior: smooth ? "smooth" : "auto" });
    },
  };
}

/** Whether the user allows motion, read when a section button is pressed. */
function allowsMotion(): boolean {
  return (
    typeof window.matchMedia === "function" && window.matchMedia("(prefers-reduced-motion: no-preference)").matches
  );
}

type EmojiCellProps = {
  item: Item;
  column: number;
  /** Whether the cell is the one of the grid in the tab sequence (a roving tabindex). */
  isActive: boolean;
  skinTone: SkinTone;
  allowMotion: boolean | undefined;
  onPick: (item: Item, event: PressEvent) => void;
  onFocusCell: (id: string) => void;
  cellRef: (node: HTMLDivElement | null) => (() => void) | undefined;
};

/** One emoji of the grid (role="gridcell"), named by its CLDR name or `:shortcode:` (ADR-0091 §6). */
function EmojiCell({ item, column, isActive, skinTone, allowMotion, onPick, onFocusCell, cellRef }: EmojiCellProps) {
  const styles = picker();
  // React Aria's press: Enter, Space, a click or a tap, with whether Shift was held and whether it was a touch.
  const { pressProps, isPressed } = usePress({
    onPress: (event) => {
      onPick(item, event);
    },
  });
  const { focusProps, isFocusVisible } = useFocusRing();
  const { hoverProps, isHovered } = useHover({});
  const { emoji } = item;
  const variant = "unicode" in emoji ? skinToneVariant(emoji, skinTone) : undefined;
  const name = "unicode" in emoji ? (variant?.name ?? emoji.name) : `:${emoji.shortcode}:`;
  return (
    <div
      {...mergeProps(pressProps, focusProps, hoverProps, {
        onFocus: () => {
          onFocusCell(item.id);
        },
      })}
      ref={cellRef}
      role="gridcell"
      aria-colindex={column + 1}
      aria-label={name}
      tabIndex={isActive ? 0 : -1}
      data-focus-visible={isFocusVisible || undefined}
      data-hovered={isHovered || undefined}
      data-pressed={isPressed || undefined}
      className={styles.item()}
    >
      {"unicode" in emoji ? (
        (variant?.unicode ?? emoji.unicode)
      ) : (
        <CustomEmoji emoji={emoji} allowMotion={allowMotion} />
      )}
    </div>
  );
}

/** Where the user moved with a section button: the section, and the scroll offset that move ends at. */
type Jump = { section: string; offset: number };

/**
 * A search field, a skin tone menu, a toolbar of section buttons and a grid of the emoji of `sections`; picking one
 * calls `onSelect`. See `EmojiPickerProps` for the props, and the comment at the top of this file for the keyboard.
 */
export function EmojiPicker({
  sections,
  loading = false,
  onSelect,
  skinTone = "none",
  onSkinToneChange,
  labels,
  allowMotion,
  autoFocus = false,
}: EmojiPickerProps) {
  const styles = picker();
  const { direction } = useLocale();
  const [search, setSearch] = useState("");
  const [announced, setAnnounced] = useState<Announced | null>(null);
  // The cell in the tab sequence, by its item id: the one that last had focus, or the first emoji (ADR-0097 §4).
  const [activeId, setActiveId] = useState<string | undefined>(undefined);
  const [jump, setJump] = useState<Jump | null>(null);
  const [metrics, setMetrics] = useState<Metrics>(fallbackMetrics);
  const gridRef = useRef<HTMLDivElement | null>(null);
  const probeRef = useRef<HTMLDivElement | null>(null);
  // The rendered cells by item id, and the cell to focus once it is rendered.
  const cellsRef = useRef(new Map<string, HTMLElement>());
  const pendingFocusRef = useRef<string | null>(null);

  const shown = sections.filter((section) => section.emojis.length > 0);
  const query = normalizeEmojiSearch(search);
  const searching = query !== "";
  const results = searching ? searchEmojis(emojisOf(shown), search) : [];
  const groups: Group[] = searching
    ? results.length > 0
      ? [
          {
            id: "search",
            heading: labels.results,
            items: results.map((emoji) => ({ id: `search\n${emojiKey(emoji)}`, emoji })),
          },
        ]
      : []
    : shown.map((section) => ({
        id: section.id,
        heading: section.heading,
        items: section.emojis.map((emoji) => ({ id: `${section.id}\n${emojiKey(emoji)}`, emoji })),
      }));
  const count = results.length;

  const columns = Math.max(1, Math.floor((metrics.width + metrics.pitch - metrics.cell) / metrics.pitch));
  const rowSize = metrics.cellBlock + metrics.pitch - metrics.cell;
  const layout = layOut(groups, columns, rowSize, metrics.header);
  // A cell no longer in the grid (the search changed) gives way to the first emoji.
  const activeIndex = Math.max(
    0,
    layout.cells.findIndex((cell) => cell.item.id === activeId),
  );
  const active = layout.cells[activeIndex];
  const activeRow = active?.row ?? -1;

  const { virtualItems, totalSize, scrollOffset, viewportHeight, scrollToIndex } = useVirtualRows({
    count: layout.rows.length,
    sizes: layout.sizes,
    getScrollElement: () => gridRef.current,
    getItemKey: (index) => layout.rows[index]?.key ?? String(index),
    // The row of the active cell stays rendered wherever it is, so focus is never lost to the virtualization.
    rangeExtractor: (range) => {
      const indexes = defaultRangeExtractor(range);
      if (activeRow < 0 || indexes.includes(activeRow)) return indexes;
      return [...indexes, activeRow].toSorted((first, second) => first - second);
    },
  });

  // The section at the top of the grid: the last heading at or above the band at its top (ADR-0097 §5), unless the
  // grid is where a section button took it, which shows that section even when the grid cannot scroll that far.
  let topSection = shown[0]?.id;
  for (const row of layout.headerRows) {
    if ((layout.starts[row] ?? 0) > scrollOffset + viewportHeight * topBand) break;
    topSection = layout.rows[row]?.group.id;
  }
  const currentSection = searching
    ? undefined
    : jump !== null && Math.abs(scrollOffset - jump.offset) < 1
      ? jump.section
      : topSection;

  // Announce the number of results once typing pauses (web-client/README.md Y7, like TextArea's count).
  useEffect(() => {
    if (query === "") return;
    const timer = setTimeout(() => {
      setAnnounced({ query, count });
    }, countAnnounceDelay);
    return () => {
      clearTimeout(timer);
    };
  }, [query, count]);

  // Measure the grid's width and the probes whenever they change size (ADR-0097 §1). The probes come before the grid,
  // so their ref is set when the grid's runs.
  function observeGrid(grid: HTMLDivElement) {
    gridRef.current = grid;
    const probe = probeRef.current;
    if (probe === null || typeof ResizeObserver === "undefined") {
      return () => {
        gridRef.current = null;
      };
    }
    const observer = new ResizeObserver(() => {
      const next = measure(grid, probe);
      setMetrics((previous) => (sameMetrics(previous, next) ? previous : next));
    });
    observer.observe(grid);
    for (const child of probe.children) observer.observe(child);
    return () => {
      observer.disconnect();
      gridRef.current = null;
    };
  }

  function cellRef(id: string) {
    return (node: HTMLDivElement | null) => {
      if (node === null) return undefined;
      cellsRef.current.set(id, node);
      if (pendingFocusRef.current === id) {
        pendingFocusRef.current = null;
        node.focus({ preventScroll: true });
      }
      return () => {
        if (cellsRef.current.get(id) === node) cellsRef.current.delete(id);
      };
    };
  }

  /** Makes cell `index` the active one and focuses it, and scrolls row `scrollRow` (its own by default) into view. */
  function focusCell(index: number, scrollRow?: number, align: "auto" | "start" = "auto", smooth = false) {
    const cell = layout.cells[index];
    if (cell === undefined) return;
    setActiveId(cell.item.id);
    const element = cellsRef.current.get(cell.item.id);
    if (element === undefined) {
      // Not rendered: the range now takes its row (rangeExtractor), and its ref focuses it once it is.
      pendingFocusRef.current = cell.item.id;
    } else {
      element.focus({ preventScroll: true });
    }
    scrollToIndex(scrollRow ?? cell.row, align, smooth);
  }

  function onGridKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (active === undefined || event.altKey || event.metaKey) return;
    if (!(event.target instanceof HTMLElement) || event.target.getAttribute("role") !== "gridcell") return;
    const row = layout.rows[active.row];
    if (row?.kind !== "emoji") return;
    const forward = direction === "rtl" ? -1 : 1;
    const pageRows = Math.max(1, Math.floor(viewportHeight / rowSize));
    let target: number | undefined;
    switch (event.key) {
      case "ArrowRight":
        target = activeIndex + forward;
        break;
      case "ArrowLeft":
        target = activeIndex - forward;
        break;
      case "ArrowDown":
        target = cellInRow(layout, active, active.row + 1, 1);
        break;
      case "ArrowUp":
        target = cellInRow(layout, active, active.row - 1, -1);
        break;
      case "PageDown":
        target = cellInRow(layout, active, active.row + pageRows, 1);
        break;
      case "PageUp":
        target = cellInRow(layout, active, active.row - pageRows, -1);
        break;
      case "Home":
        target = event.ctrlKey ? 0 : row.first;
        break;
      case "End":
        target = event.ctrlKey ? layout.cells.length - 1 : row.last;
        break;
      default:
        return;
    }
    event.preventDefault();
    const cell = target === undefined ? undefined : layout.cells[target];
    if (target === undefined || cell === undefined || target === activeIndex) return;
    // Up into the first row of a section shows its heading too.
    const above = cell.row - 1;
    focusCell(target, cell.row < active.row && layout.rows[above]?.kind === "header" ? above : undefined);
  }

  function pick(item: Item, event: PressEvent) {
    // Shift keeps the picker open for the keyboard and the mouse; a touch has no Shift to hold (ADR-0091 §5).
    const keepOpen = event.shiftKey && (event.pointerType === "keyboard" || event.pointerType === "mouse");
    const { emoji } = item;
    if ("unicode" in emoji)
      onSelect({ kind: "unicode", emoji, text: skinToneVariant(emoji, skinTone)?.unicode ?? emoji.unicode, keepOpen });
    else onSelect({ kind: "custom", emoji, keepOpen });
  }

  function moveTo(section: EmojiPickerSection) {
    const headerRow = layout.headerRowOf.get(section.id);
    const first = layout.firstCellOf.get(section.id);
    if (headerRow === undefined || first === undefined) return;
    // The heading goes to the top of the grid, or as far as the grid scrolls.
    setJump({
      section: section.id,
      offset: Math.min(layout.starts[headerRow] ?? 0, Math.max(0, layout.total - viewportHeight)),
    });
    focusCell(first, headerRow, "start", allowsMotion());
  }

  // The rendered rows, in a rowgroup per section named by its heading (ADR-0097 §3). A section's rowgroup is keyed by
  // the section, so the row of the active cell, rendered apart from the rows in view, never moves to another element.
  const rowGroups: { group: Group; items: VirtualItem[] }[] = [];
  for (const virtualItem of virtualItems) {
    const row = layout.rows[virtualItem.index];
    if (row === undefined) continue;
    const last = rowGroups.at(-1);
    if (last?.group.id === row.group.id) last.items.push(virtualItem);
    else rowGroups.push({ group: row.group, items: [virtualItem] });
  }

  const status = loading
    ? (labels.loading ?? "")
    : announced !== null && announced.query === query
      ? labels.resultCount(announced.count)
      : "";
  // While loading, the sections still without emoji keep their (disabled) buttons.
  const buttons = loading ? sections : shown;
  const skeletonRows = loading ? (
    <div aria-hidden="true" className={styles.skeleton()}>
      {[0, 1, 2, 3].map((row) => (
        <div key={row} className={styles.row()}>
          {Array.from({ length: columns }, (_, column) => (
            <Skeleton key={column} shape="rect" className={styles.skeletonCell()} />
          ))}
        </div>
      ))}
    </div>
  ) : null;

  return (
    <div className={styles.root()}>
      <div className={styles.top()}>
        <TextField
          type="search"
          label={labels.search}
          value={search}
          onChange={(value) => {
            setSearch(value);
            setJump(null);
            scrollToIndex(0, "start", false);
          }}
          autoFocus={autoFocus}
          grow
          shrink
        />
        {onSkinToneChange !== undefined && labels.skinTone !== undefined && labels.skinTones !== undefined && (
          <SkinToneButton
            value={skinTone}
            onChange={onSkinToneChange}
            label={labels.skinTone}
            names={labels.skinTones}
          />
        )}
      </div>
      {buttons.length > 1 && (
        <Toolbar aria-label={labels.categories} className={styles.toolbar()}>
          {buttons.map((section) => (
            <TooltipTrigger key={section.id}>
              <Button
                variant="ghost"
                size="icon"
                aria-label={section.heading}
                {...(section.id === currentSection ? { "aria-current": "true" } : {})}
                isDisabled={searching || section.emojis.length === 0}
                className={styles.tab()}
                onPress={() => {
                  moveTo(section);
                }}
              >
                <SectionIcon icon={section.icon} />
              </Button>
              <Tooltip>{section.heading}</Tooltip>
            </TooltipTrigger>
          ))}
        </Toolbar>
      )}
      <div ref={probeRef} aria-hidden="true" className={styles.probe()}>
        <div className={styles.header()}>
          <span className="block h-lh" />
        </div>
        <div className={styles.item()} />
        <div className={styles.item()} />
      </div>
      <div
        ref={observeGrid}
        role="grid"
        aria-label={labels.grid}
        aria-rowcount={Math.max(1, layout.rows.length)}
        aria-colcount={columns}
        {...(loading ? { "aria-busy": "true" } : {})}
        className={styles.grid()}
        // eslint-disable-next-line restrict/syntax -- ADR-0097 §4: GridList cannot leave rows unrendered, so the virtualized grid handles its own keys (APG grid)
        onKeyDown={onGridKeyDown}
      >
        {layout.rows.length === 0 && loading ? null : layout.rows.length === 0 ? (
          <div role="row" aria-rowindex={1}>
            <div role="gridcell" aria-colspan={columns} className={styles.empty()}>
              {labels.empty}
            </div>
          </div>
        ) : (
          <div role="presentation" className={styles.rows()} style={{ blockSize: totalSize }}>
            {rowGroups.map(({ group, items }) => (
              <div key={group.id} role="rowgroup" aria-label={group.heading}>
                {items.map((virtualItem) => {
                  const row = layout.rows[virtualItem.index];
                  if (row === undefined) return null;
                  const style = {
                    position: "absolute",
                    insetBlockStart: 0,
                    insetInline: 0,
                    blockSize: virtualItem.size,
                    transform: `translateY(${String(virtualItem.start)}px)`,
                  } as const;
                  if (row.kind === "header") {
                    return (
                      <div
                        key={row.key}
                        role="row"
                        aria-rowindex={virtualItem.index + 1}
                        style={style}
                        className={styles.header()}
                      >
                        <div role="rowheader" aria-colspan={columns}>
                          {row.group.heading}
                        </div>
                      </div>
                    );
                  }
                  return (
                    <div
                      key={row.key}
                      role="row"
                      aria-rowindex={virtualItem.index + 1}
                      style={style}
                      className={styles.row()}
                    >
                      {layout.cells.slice(row.first, row.last + 1).map((cell, offset) => (
                        <EmojiCell
                          key={cell.item.id}
                          item={cell.item}
                          column={cell.column}
                          isActive={row.first + offset === activeIndex}
                          skinTone={skinTone}
                          allowMotion={allowMotion}
                          onPick={pick}
                          onFocusCell={setActiveId}
                          cellRef={cellRef(cell.item.id)}
                        />
                      ))}
                    </div>
                  );
                })}
              </div>
            ))}
          </div>
        )}
        {skeletonRows}
      </div>
      <span role="status" className={styles.status()}>
        {status}
      </span>
    </div>
  );
}
