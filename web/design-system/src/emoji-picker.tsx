// SPDX-License-Identifier: AGPL-3.0-only

// The emoji picker (ADR-0091 §3–§7): one picker for a post's reaction and the composer's emoji button. It holds no
// emoji of its own: its caller gives the sections (the reader's frequent ones, the server's custom emoji by category,
// the nine groups of Unicode emoji) with their headings, the search over them and the skin tone's words (ADR-0091 §1);
// its own words are its catalog's. From the top: the search field and the skin tone's button beside it, the row of
// categories, each a glyph that scrolls the grid to its section (the section at the top of the grid is the current
// one, on the raised surface), and the grid, one scroll of the sections each under a muted heading. While a query is
// typed, the categories rest and the grid holds the results alone, or a line that none match. The reader's own
// reaction, if any, is knocked out as its chip is.
//
// The grid is one list of rows (ADR-0091 §14): per section a heading row, then its emoji cut into rows of as many
// columns as its width holds, and the same for each group of the section. Only the rows in view and four on each side
// are drawn (TanStack Virtual, §15), and the row of the cell in the tab sequence wherever it is, so that focus is never
// lost to the virtualization. The grid tells its size and each row's and cell's place (aria-rowcount, aria-colcount,
// aria-rowindex, aria-colindex), and groups the drawn rows of a section in a rowgroup named by its heading (§16).
//
// Keyboard (APG grid, with a roving tabindex, §17): Tab goes from the search field to the skin tone's button, the
// categories and the grid, where one cell takes focus. In the grid, left and right follow the writing direction:
//
//   Right / Left            the next / previous emoji, on to the next / previous row and section
//   Down / Up               the emoji row below / above (headings skipped), in the same column or the row's last
//   Home / End              the first / last emoji of the row
//   Control + Home / End    the first / last emoji of the grid
//   Page Down / Page Up     as many rows down / up as the grid shows
//   Enter, Space, a press   pick the emoji; with Shift held (Shift+Enter, Shift+click) the caller is told to keep the
//                           picker open, and focus stays on it; a touch never does (§5)
//
// It stays fast with thousands of emoji: the layout is computed once per set of emoji and width, never on a scroll;
// every row has a fixed height, so a scroll measures nothing; a cell is memoized on its own props and takes stable
// handlers, so that a scroll draws only the rows that come in, and a move of the focus redraws only the two cells
// whose tab stop moved; and the search runs on a deferred query, off the path of the keystroke.
//
// It lays itself out by its own width, never the viewport's: the cells fill the width it is given, a cell's least
// size from the tokens (larger on a touch screen), so that the same picker sits in a popover from `tablet` and in a
// sheet on a phone, which the app chooses (emoji-picker-overlay.tsx). In a popover its grid has a most height; in a
// sheet it fills what the sheet leaves it. This module is loaded apart from the design system's entry, when a picker
// is first wanted (`LazyEmojiPicker`), together with TanStack Virtual.

import {
  defaultRangeExtractor,
  observeElementRect,
  useVirtualizer,
  type Range,
  type VirtualItem,
  type Virtualizer,
} from "@tanstack/react-virtual";
import {
  memo,
  useDeferredValue,
  useEffect,
  useId,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
  type RefObject,
} from "react";
import { mergeProps, useFocusRing, useHover, usePress, type PressEvent } from "react-aria";
import {
  Button as AriaButton,
  Menu,
  MenuItem as AriaMenuItem,
  MenuTrigger,
  Popover,
  Toolbar,
} from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { heardPointerDown } from "./button";
import { Icon, type IconName } from "./icons";
import { stringsIn, useReader } from "./locale";

/** An emoji the picker offers: a Unicode one, with its skin tones if it has them, or a custom one, as its image. */
export type PickerEmoji =
  | {
      /** What the picker tells it by: the emoji's text without a tone. */
      key: string;
      /** Its name for assistive technology (the locale's short name). */
      name: string;
      text: string;
      /** The emoji in the five skin tones, light to dark. */
      tones?: readonly string[] | undefined;
    }
  | {
      /** What the picker tells it by: `:shortcode:`. */
      key: string;
      /** Its name for assistive technology: `:shortcode:`. */
      name: string;
      image: { url: string; staticUrl: string };
    };

/** A section of the grid, and a button in the row of categories. */
export type EmojiSection = {
  id: string;
  /** Its heading, and the name of its category button. */
  label: string;
  icon: IconName;
  emojis: readonly PickerEmoji[];
  /** The groups after `emojis`, each under its own heading (the server's custom emoji by category). */
  groups?: readonly { label: string; emojis: readonly PickerEmoji[] }[] | undefined;
};

/**
 * The words of the skin tone, which the app gives (ADR-0091 §1): the picker's own words (the search, the results, the
 * row of categories) are the design system's.
 */
export type EmojiPickerLabels = {
  /** The skin tone's button and menu. */
  tone: string;
  /** The six tones' names: none (the default yellow), then light to dark. */
  tones: readonly string[];
};

export type EmojiPickerProps = {
  sections: readonly EmojiSection[];
  /** The emoji that match a query, in the order to show them. */
  search: (query: string) => readonly PickerEmoji[];
  /**
   * The emoji picked, as its text in the skin tone chosen (a custom one's `:shortcode:`); `keep` when it was picked
   * with Shift held, by the keyboard or a mouse, and focus stays on it, for the caller that lets the reader go on
   * picking (ADR-0091 §5).
   */
  onSelect: (emoji: PickerEmoji, text: string, options: { keep: boolean }) => void;
  labels: EmojiPickerLabels;
  /** The key of the emoji the reader reacted with, knocked out. */
  current?: string | undefined;
  /** The skin tone: 0 for none, 1 to 5 light to dark. */
  tone?: number | undefined;
  onToneChange?: ((tone: number) => void) | undefined;
  /** The query it opens with (a story's). */
  defaultQuery?: string | undefined;
  /** The grid fills the height its container gives it (a sheet's), instead of keeping to a popover's most height. */
  fill?: boolean | undefined;
  /** The search field takes focus when the picker appears (in a popover or sheet the reader just opened). */
  autoFocus?: boolean | undefined;
};

const picker = tv({
  slots: {
    root: "relative grid content-start gap-emoji-picker-gap font-sans text-body text-text",
    top: "flex items-center gap-emoji-picker-gap",
    searchBox: "relative min-w-0 flex-1",
    searchGlyph:
      "pointer-events-none absolute inset-y-0 start-control-timeline-inline inline-flex items-center text-muted-text",
    search: [
      "box-border h-control-form w-full rounded-control border-0 bg-surface-raised font-sans text-body text-text",
      "ps-[calc(var(--control-timeline-padding-inline)*2+var(--icon-action))] pe-control-timeline-inline",
      "outline-hidden placeholder:text-muted-text focus-visible:shadow-focus",
      "[&::-webkit-search-cancel-button]:cursor-pointer",
    ],
    tone: [
      "box-border inline-flex size-control-form flex-none cursor-pointer items-center justify-center rounded-control",
      "border-0 bg-transparent p-0 text-[length:var(--emoji-picker-glyph)] leading-none outline-hidden",
      "data-hovered:bg-surface-hover data-pressed:bg-surface-pressed data-focus-visible:shadow-focus",
    ],
    tabs: "grid auto-cols-fr grid-flow-col",
    tab: [
      "box-border inline-flex h-emoji-picker-tab min-w-0 cursor-pointer items-center justify-center rounded-control",
      "border-0 bg-transparent p-0 text-muted-text outline-hidden",
      "data-hovered:bg-surface-hover data-hovered:text-text data-focus-visible:shadow-focus",
      "aria-current:bg-surface-raised aria-current:text-text",
      "data-disabled:cursor-default data-disabled:opacity-disabled data-disabled:bg-transparent",
    ],
    // The grid is its own scroll; the rows are placed in it by the virtualizer (ADR-0091 §15), never by CSS scrolling.
    grid: "-mx-emoji-picker-gap overflow-y-auto overscroll-contain px-emoji-picker-gap outline-hidden",
    rows: "relative w-full",
    heading: [
      "box-border m-0 flex items-end px-emoji-picker-heading-gap pt-emoji-picker-gap pb-emoji-picker-heading-gap",
      "text-caption font-normal text-muted-text",
    ],
    row: "grid",
    // Copies of a heading and of the cells, never seen, that tell the sizes of the rows and the number of columns.
    probe: [
      "pointer-events-none invisible absolute inset-x-0 top-0",
      "grid grid-cols-[repeat(auto-fill,minmax(var(--emoji-picker-cell),1fr))]",
      "pointer-coarse:grid-cols-[repeat(auto-fill,minmax(var(--emoji-picker-cell-coarse),1fr))]",
    ],
    probeHeading: "col-span-full",
    cell: [
      "box-border inline-flex h-emoji-picker-cell min-w-0 cursor-pointer items-center justify-center rounded-control",
      "font-sans text-[length:var(--emoji-picker-glyph)] leading-none outline-hidden select-none",
      "pointer-coarse:h-emoji-picker-cell-coarse",
      "data-hovered:bg-surface-hover data-pressed:bg-surface-pressed data-focus-visible:shadow-focus-inset",
      // The reader's own reaction, knocked out as its chip is: the text's color as its fill.
      "data-current:bg-text data-current:data-hovered:bg-text",
    ],
    // A custom emoji's image, as large against the cell's font size as a post draws one against its text.
    image: "size-content-emoji object-contain",
    empty:
      "m-0 grid min-h-emoji-picker-cell place-items-center py-emoji-picker-gap text-center text-caption text-muted-text",
    status: "sr-only",
    menu: "box-border rounded-overlay bg-surface p-menu-padding shadow-overlay outline-hidden",
    menuList: "flex flex-col outline-hidden",
    menuItem: [
      "box-border flex h-control-form cursor-default items-center gap-menu-icon-gap rounded-control",
      "px-control-timeline-inline text-body whitespace-nowrap outline-hidden",
      "data-hovered:bg-surface-raised data-focused:bg-surface-raised data-focus-visible:shadow-focus-inset",
    ],
    menuGlyph: "w-emoji-picker-glyph text-[length:var(--emoji-picker-glyph)] leading-none",
    menuCheck: "ms-auto text-muted-text",
  },
  variants: {
    fill: {
      true: { root: "h-full grid-rows-[auto_auto_minmax(0,1fr)]", grid: "min-h-0" },
      false: { grid: "max-h-emoji-picker-height" },
    },
  },
});

// The classes of a cell, once: a grid draws hundreds of cells, each without computing its own.
const base = picker();
const cellClass = base.cell();
const imageClass = base.image();

/** The hand that shows a tone, and the emoji in it. */
const hands = ["✋", "✋🏻", "✋🏼", "✋🏽", "✋🏾", "✋🏿"] as const;

/** The emoji's text in `tone`, where it has tones. */
const textIn = (emoji: PickerEmoji, tone: number) =>
  "text" in emoji ? ((tone > 0 ? emoji.tones?.[tone - 1] : undefined) ?? emoji.text) : emoji.key;

/** How long a search rests before its count is announced (ADR-0091 §3). */
const announceDelay = 500;

/** The share of the grid's height at its top where a heading counts as the top of the grid (ADR-0091 §18). */
const topBand = 0.1;

/** How many rows the grid draws beyond each end of its view (ADR-0091 §15). */
const overscan = 4;

/** The size in CSS px a custom emoji's image is drawn at in a cell (the glyph's size), so that it takes no relayout. */
const imageSize = 24;

/** A heading row, or a row of emoji (the cells `first` to `last` of the layout, inclusive), of a section. */
type GridRow =
  | { kind: "heading"; key: string; section: string; sectionLabel: string; label: string }
  | { kind: "emoji"; key: string; section: string; sectionLabel: string; first: number; last: number };

/** One emoji of the layout, under an id unique in the grid (an emoji may be in two sections: the frequent ones). */
type Cell = { id: string; emoji: PickerEmoji; row: number; column: number };

type Layout = {
  rows: GridRow[];
  cells: Cell[];
  /** The block size of each row and where it starts, in px: every row of a kind has the same. */
  sizes: number[];
  starts: number[];
  keys: string[];
  total: number;
  /** The heading row and the first cell of each section, by its id. */
  headingOf: Map<string, number>;
  firstCellOf: Map<string, number>;
  /** Each cell's index, by its id. */
  indexOf: Map<string, number>;
};

/** A part of the grid: its heading and its emoji, in a section (a section's own emoji, or one of its groups). */
type Part = { section: string; sectionLabel: string; label: string; emojis: readonly PickerEmoji[] };

/** The rows of `parts` (ADR-0091 §14): per part a heading row, then its emoji in rows of `columns`. */
function layOut(parts: readonly Part[], columns: number, rowSize: number, headingSize: number): Layout {
  const layout: Layout = {
    rows: [],
    cells: [],
    sizes: [],
    starts: [],
    keys: [],
    total: 0,
    headingOf: new Map(),
    firstCellOf: new Map(),
    indexOf: new Map(),
  };
  const add = (row: GridRow, size: number) => {
    layout.rows.push(row);
    layout.keys.push(row.key);
    layout.sizes.push(size);
    layout.starts.push(layout.total);
    layout.total += size;
  };
  parts.forEach((part, p) => {
    const { section, sectionLabel } = part;
    if (!layout.headingOf.has(section)) layout.headingOf.set(section, layout.rows.length);
    add({ kind: "heading", key: `${String(p)}\n`, section, sectionLabel, label: part.label }, headingSize);
    if (!layout.firstCellOf.has(section) && part.emojis.length > 0)
      layout.firstCellOf.set(section, layout.cells.length);
    for (let start = 0; start < part.emojis.length; start += columns) {
      const row = layout.rows.length;
      const first = layout.cells.length;
      part.emojis.slice(start, start + columns).forEach((emoji, column) => {
        const id = `${section}\n${emoji.key}`;
        layout.indexOf.set(id, layout.cells.length);
        layout.cells.push({ id, emoji, row, column });
      });
      add(
        {
          kind: "emoji",
          key: `${String(p)}\n${String(start)}`,
          section,
          sectionLabel,
          first,
          last: layout.cells.length - 1,
        },
        rowSize,
      );
    }
  });
  return layout;
}

/**
 * The cell to move to in row `toRow` (kept in the grid) from `from`, in the same column or the row's last: a heading
 * row gives way to the next emoji row in `step`'s direction, or the other way at the edge of the grid.
 */
function cellInRow(layout: Layout, from: Cell, toRow: number, step: 1 | -1): number | undefined {
  let index = Math.max(0, Math.min(toRow, layout.rows.length - 1));
  while (layout.rows[index]?.kind === "heading" && layout.rows[index + step] !== undefined) index += step;
  while (layout.rows[index]?.kind === "heading" && layout.rows[index - step] !== undefined) index -= step;
  const row = layout.rows[index];
  if (row?.kind !== "emoji") return undefined;
  return Math.min(row.first + from.column, row.last);
}

/** The sizes in CSS px the rows and the columns are laid out by, measured on the probe (ADR-0091 §14). */
type Metrics = { columns: number; row: number; heading: number };

/**
 * What the grid is laid out by until it is measured, and where nothing is laid out (jsdom, the server): the eight
 * columns of the popover's width, 40 px cells, and a heading of a caption with its padding.
 */
const fallbackMetrics: Metrics = { columns: 8, row: 40, heading: 28 };

/** The grid's height until it is measured: a popover's most. */
const fallbackHeight = 256;

/** `value` when it is a size something was laid out with, or else `fallback`. */
const laidOut = (value: number | undefined, fallback: number) => (value !== undefined && value > 0 ? value : fallback);

/** The metrics from the probe: a heading over two cells, in the columns its width holds. */
function measure(probe: HTMLElement): Metrics {
  const [heading, first, second] = [...probe.children].filter((child) => child instanceof HTMLElement);
  const pitch = first === undefined || second === undefined ? 0 : Math.abs(second.offsetLeft - first.offsetLeft);
  return {
    columns: pitch > 0 ? Math.max(1, Math.round(probe.clientWidth / pitch)) : fallbackMetrics.columns,
    row: laidOut(first?.offsetHeight, fallbackMetrics.row),
    heading: laidOut(heading?.offsetHeight, fallbackMetrics.heading),
  };
}

/** Reports the grid's size only once it is laid out, so that a grid not laid out (jsdom) keeps `initialRect`. */
const observeLaidOutRect: typeof observeElementRect = (instance, callback) =>
  observeElementRect(instance, (rect) => {
    if (rect.width > 0 && rect.height > 0) callback(rect);
  });

type VirtualRows = {
  items: VirtualItem[];
  total: number;
  offset: number;
  height: number;
  /** Scrolls row `index` into view (`auto`) or to the top (`start`); called in event handlers only. */
  scrollTo: (index: number, align: "auto" | "start", smooth: boolean) => void;
};

/**
 * The virtualizer of the grid's rows, read into plain values. The virtualizer is one mutable object whose methods give
 * new results after a scroll, which a memoized read would miss, so this hook alone opts out of React Compiler
 * (ADR-0091 §15); its caller reads plain values and is compiled as usual. The rows have fixed sizes, never measured.
 */
function useVirtualRows(options: {
  sizes: readonly number[];
  keys: readonly string[];
  scroller: RefObject<HTMLDivElement | null>;
  rangeExtractor: (range: Range) => number[];
}): VirtualRows {
  "use no memo";
  const virtualizer: Virtualizer<HTMLDivElement, Element> = useVirtualizer({
    count: options.sizes.length,
    getScrollElement: () => options.scroller.current,
    estimateSize: (index) => options.sizes[index] ?? 0,
    getItemKey: (index) => options.keys[index] ?? index,
    rangeExtractor: options.rangeExtractor,
    overscan,
    initialRect: { width: 0, height: fallbackHeight },
    observeElementRect: observeLaidOutRect,
  });
  return {
    items: virtualizer.getVirtualItems(),
    total: virtualizer.getTotalSize(),
    offset: virtualizer.scrollOffset ?? 0,
    height: virtualizer.scrollRect?.height ?? fallbackHeight,
    scrollTo: (index, align, smooth) => {
      virtualizer.scrollToIndex(index, { align, behavior: smooth ? "smooth" : "auto" });
    },
  };
}

/** Whether the reader allows motion, read when a category is pressed. */
const allowsMotion = () =>
  typeof window.matchMedia === "function" && window.matchMedia("(prefers-reduced-motion: no-preference)").matches;

type CellProps = {
  id: string;
  emoji: PickerEmoji;
  column: number;
  /** Its name for assistive technology: the emoji's, or that it is the reader's own reaction. */
  label: string;
  tabbable: boolean;
  current: boolean;
  /** The emoji's text as the cell shows it, in the skin tone chosen. */
  shown: string;
  /** Stable for the life of the picker, so that a cell redraws only when its own props change. */
  onPick: (id: string, event: PressEvent) => void;
  onFocusCell: (id: string) => void;
};

/**
 * One emoji of the grid (role="gridcell"). Memoized: its props are plain values and stable handlers, so a scroll, a
 * search's result or a move of the tab stop redraws only the cells they change.
 */
const EmojiCell = memo(function EmojiCell({
  id,
  emoji,
  column,
  label,
  tabbable,
  current,
  shown,
  onPick,
  onFocusCell,
}: CellProps) {
  // React Aria's press: Enter, Space, a click or a tap, with whether Shift was held and whether it was a touch.
  const { pressProps, isPressed } = usePress({
    onPress: (e) => {
      onPick(id, e);
    },
  });
  const { focusProps, isFocusVisible } = useFocusRing();
  const { hoverProps, isHovered } = useHover({});
  return (
    <div
      {...mergeProps(pressProps, focusProps, hoverProps, {
        onFocus: () => {
          onFocusCell(id);
        },
      })}
      role="gridcell"
      aria-colindex={column + 1}
      aria-label={label}
      tabIndex={tabbable ? 0 : -1}
      className={cellClass}
      data-cell={id}
      data-current={current || undefined}
      data-focus-visible={isFocusVisible || undefined}
      data-hovered={isHovered || undefined}
      data-pressed={isPressed || undefined}
    >
      {"text" in emoji ? (
        <span aria-hidden="true">{shown}</span>
      ) : (
        <img
          className={imageClass}
          src={emoji.image.staticUrl}
          alt=""
          width={imageSize}
          height={imageSize}
          loading="lazy"
          decoding="async"
          draggable={false}
        />
      )}
    </div>
  );
});

function ToneButton({
  tone,
  labels,
  onChange,
}: {
  tone: number;
  labels: EmojiPickerLabels;
  onChange: (tone: number) => void;
}) {
  const s = picker();
  const id = useId();
  return (
    <MenuTrigger>
      <AriaButton className={s.tone()} aria-label={`${labels.tone}: ${labels.tones[tone] ?? ""}`}>
        <span aria-hidden="true">{hands[tone]}</span>
      </AriaButton>
      <Popover placement="bottom end" offset={4} className="outline-hidden">
        <div className={s.menu()}>
          {/* React Aria names the menu by its button, whose name says the tone in use too: its own label names it. */}
          <span id={id} hidden>
            {labels.tone}
          </span>
          <Menu
            aria-label={labels.tone}
            aria-labelledby={id}
            className={s.menuList()}
            selectionMode="single"
            selectedKeys={[String(tone)]}
            onAction={(key) => {
              onChange(Number(key));
            }}
          >
            {hands.map((hand, i) => (
              <AriaMenuItem key={hand} id={String(i)} textValue={labels.tones[i] ?? ""} className={s.menuItem()}>
                {({ isSelected }) => (
                  <>
                    <span aria-hidden="true" className={s.menuGlyph()}>
                      {hand}
                    </span>
                    <span>{labels.tones[i]}</span>
                    {isSelected && (
                      <span className={s.menuCheck()}>
                        <Icon name="read" place="caption" />
                      </span>
                    )}
                  </>
                )}
              </AriaMenuItem>
            ))}
          </Menu>
        </div>
      </Popover>
    </MenuTrigger>
  );
}

/** Where the reader moved with a category: the section, and the scroll offset that move ends at. */
type Jump = { section: string; offset: number };

export function EmojiPicker({
  sections,
  search,
  onSelect,
  labels,
  current,
  tone: givenTone,
  onToneChange,
  defaultQuery = "",
  fill = false,
  autoFocus = false,
}: EmojiPickerProps) {
  const s = picker({ fill });
  const reader = useReader(undefined);
  const { direction } = reader;
  const { t, lang } = stringsIn(reader.lang);
  const [query, setQuery] = useState(defaultQuery);
  const [ownTone, setOwnTone] = useState(0);
  const tone = givenTone ?? ownTone;
  const [announced, setAnnounced] = useState<{ query: string; count: number } | undefined>(undefined);
  // The cell in the tab sequence, by its id: the one that last had focus, or the first emoji (ADR-0091 §17).
  const [activeId, setActiveId] = useState<string | undefined>(undefined);
  const [jump, setJump] = useState<Jump | undefined>(undefined);
  const [metrics, setMetrics] = useState<Metrics>(fallbackMetrics);
  const scroller = useRef<HTMLDivElement>(null);
  const probe = useRef<HTMLDivElement>(null);
  // The cell to focus once it is drawn.
  const pendingFocus = useRef<string | undefined>(undefined);

  // The search follows the field at the pace the reader's device allows: the field takes every keystroke at once,
  // and the grid shows the results of the latest query it had the time for.
  const searched = useDeferredValue(query.trim());
  const searching = searched !== "";
  // The categories follow the field itself, not the deferred query, so that the tab order never changes while a
  // search settles: off while anything is written, on again as soon as the field is empty.
  const typed = query.trim() !== "";
  const results = useMemo(() => (searched === "" ? [] : search(searched)), [search, searched]);
  const resultsLabel = t("emoji_picker.results");
  // The layout follows the emoji and the width alone: React Compiler keeps it across the renders of a scroll.
  const parts: Part[] = searching
    ? results.length > 0
      ? [{ section: "results", sectionLabel: resultsLabel, label: resultsLabel, emojis: results }]
      : []
    : sections.flatMap((section) => [
        { section: section.id, sectionLabel: section.label, label: section.label, emojis: section.emojis },
        ...(section.groups ?? []).map((group) => ({
          section: section.id,
          sectionLabel: section.label,
          label: group.label,
          emojis: group.emojis,
        })),
      ]);
  const layout = layOut(parts, metrics.columns, metrics.row, metrics.heading);
  const { columns } = metrics;
  // A cell no longer in the grid (the query changed) gives way to the first emoji, without an effect to set it.
  const activeIndex = activeId === undefined ? 0 : (layout.indexOf.get(activeId) ?? 0);
  const active = layout.cells[activeIndex];
  const activeRow = active?.row ?? -1;

  const rows = useVirtualRows({
    sizes: layout.sizes,
    keys: layout.keys,
    scroller,
    // The row of the active cell stays drawn wherever it is, so focus is never lost to the virtualization.
    rangeExtractor: (range) => {
      const indexes = defaultRangeExtractor(range);
      if (activeRow < 0 || indexes.includes(activeRow)) return indexes;
      return [...indexes, activeRow].toSorted((a, b) => a - b);
    },
  });

  // The section at the top of the grid: the last whose heading is above the band at its top (ADR-0091 §18), unless the
  // grid is where a category took it, which shows that section even when the grid cannot scroll that far.
  let top = sections[0]?.id;
  for (const [section, row] of layout.headingOf) {
    if ((layout.starts[row] ?? 0) > rows.offset + rows.height * topBand) break;
    top = section;
  }
  const shown = searching
    ? undefined
    : jump !== undefined && Math.abs(rows.offset - jump.offset) < 1
      ? jump.section
      : top;

  // The count of the results, once typing rests (ADR-0091 §3).
  const count = results.length;
  useEffect(() => {
    if (searched === "") return undefined;
    const timer = setTimeout(() => {
      setAnnounced({ query: searched, count });
    }, announceDelay);
    return () => {
      clearTimeout(timer);
    };
  }, [searched, count]);

  // The probe's sizes, measured whenever it changes size (ADR-0091 §14): never on a scroll, never per row.
  useEffect(() => {
    const node = probe.current;
    if (node === null || typeof ResizeObserver === "undefined") return undefined;
    const observer = new ResizeObserver(() => {
      const next = measure(node);
      setMetrics((was) =>
        was.columns === next.columns && was.row === next.row && was.heading === next.heading ? was : next,
      );
    });
    observer.observe(node);
    for (const child of node.children) observer.observe(child);
    return () => {
      observer.disconnect();
    };
  }, []);

  // A cell asked for focus before its row was drawn takes it once it is.
  useLayoutEffect(() => {
    const id = pendingFocus.current;
    if (id === undefined) return;
    const element = scroller.current?.querySelector<HTMLElement>(`[data-cell="${CSS.escape(id)}"]`);
    if (element === null || element === undefined) return;
    pendingFocus.current = undefined;
    element.focus({ preventScroll: true });
  });

  /** Makes cell `index` the active one and focuses it, and scrolls row `scrollRow` (its own by default) into view. */
  const focusCell = (index: number, scrollRow?: number, align: "auto" | "start" = "auto", smooth = false) => {
    const cell = layout.cells[index];
    if (cell === undefined) return;
    setActiveId(cell.id);
    const element = scroller.current?.querySelector<HTMLElement>(`[data-cell="${CSS.escape(cell.id)}"]`);
    // Not drawn: the range now takes its row (rangeExtractor), and the layout effect focuses it once it is.
    if (element === null || element === undefined) pendingFocus.current = cell.id;
    else element.focus({ preventScroll: true });
    rows.scrollTo(scrollRow ?? cell.row, align, smooth);
  };

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (active === undefined || e.altKey || e.metaKey) return;
    if (!(e.target instanceof HTMLElement) || e.target.getAttribute("role") !== "gridcell") return;
    const row = layout.rows[active.row];
    if (row?.kind !== "emoji") return;
    const forward = direction === "rtl" ? -1 : 1;
    const page = Math.max(1, Math.floor(rows.height / metrics.row));
    let target: number | undefined;
    switch (e.key) {
      case "ArrowRight":
        target = Math.min(layout.cells.length - 1, Math.max(0, activeIndex + forward));
        break;
      case "ArrowLeft":
        target = Math.min(layout.cells.length - 1, Math.max(0, activeIndex - forward));
        break;
      case "ArrowDown":
        target = cellInRow(layout, active, active.row + 1, 1);
        break;
      case "ArrowUp":
        target = cellInRow(layout, active, active.row - 1, -1);
        break;
      case "PageDown":
        target = cellInRow(layout, active, active.row + page, 1);
        break;
      case "PageUp":
        target = cellInRow(layout, active, active.row - page, -1);
        break;
      case "Home":
        target = e.ctrlKey ? 0 : row.first;
        break;
      case "End":
        target = e.ctrlKey ? layout.cells.length - 1 : row.last;
        break;
      default:
        return;
    }
    e.preventDefault();
    const cell = target === undefined ? undefined : layout.cells[target];
    if (target === undefined || cell === undefined || target === activeIndex) return;
    // Up into the first row of a section shows its heading too.
    const above = cell.row - 1;
    focusCell(target, cell.row < active.row && layout.rows[above]?.kind === "heading" ? above : undefined);
  };

  // What a press picks reads the latest props through a ref, so that the handler the cells take never changes.
  const picked = useRef<(id: string, e: PressEvent) => void>(() => undefined);
  useLayoutEffect(() => {
    picked.current = (id, e) => {
      const cell = layout.cells[layout.indexOf.get(id) ?? -1];
      if (cell === undefined) return;
      // Shift keeps the picker open for the keyboard and the mouse; a touch has no Shift to hold (ADR-0091 §5).
      const keep = e.shiftKey && (e.pointerType === "keyboard" || e.pointerType === "mouse");
      onSelect(cell.emoji, textIn(cell.emoji, tone), { keep });
    };
  });
  const [onPick] = useState(() => (id: string, e: PressEvent) => {
    picked.current(id, e);
  });

  const go = (section: string) => {
    const heading = layout.headingOf.get(section);
    const first = layout.firstCellOf.get(section);
    if (heading === undefined || first === undefined) return;
    // The heading goes to the top of the grid, or as far as the grid scrolls.
    setJump({ section, offset: Math.min(layout.starts[heading] ?? 0, Math.max(0, layout.total - rows.height)) });
    focusCell(first, heading, "start", allowsMotion());
  };

  // The drawn rows, in a rowgroup per section named by its heading (ADR-0091 §16), keyed by the section, so that the
  // row of the active cell, drawn apart from the rows in view, never moves to another element.
  const groups: { section: string; label: string; items: VirtualItem[] }[] = [];
  for (const item of rows.items) {
    const row = layout.rows[item.index];
    if (row === undefined) continue;
    const last = groups.at(-1);
    if (last?.section === row.section) last.items.push(item);
    else groups.push({ section: row.section, label: row.sectionLabel, items: [item] });
  }

  return (
    <div className={s.root()} lang={lang} dir={direction}>
      <div className={s.top()}>
        <div className={s.searchBox()}>
          <span className={s.searchGlyph()}>
            <Icon name="search" place="action" />
          </span>
          <input
            className={s.search()}
            type="search"
            value={query}
            placeholder={t("emoji_picker.search")}
            aria-label={t("emoji_picker.search")}
            dir="auto"
            autoFocus={autoFocus}
            onChange={(e) => {
              setQuery(e.target.value);
              setJump(undefined);
              rows.scrollTo(0, "start", false);
            }}
          />
        </div>
        <ToneButton
          tone={tone}
          labels={labels}
          onChange={(next) => {
            setOwnTone(next);
            onToneChange?.(next);
          }}
        />
      </div>
      <Toolbar aria-label={t("emoji_picker.categories")} className={s.tabs()}>
        {sections.map((section) => (
          <AriaButton
            key={section.id}
            className={s.tab()}
            aria-label={section.label}
            {...(shown === section.id ? { "aria-current": "true" as const } : {})}
            isDisabled={typed || (!searching && !layout.firstCellOf.has(section.id))}
            render={(props) => <button {...props} onPointerDown={heardPointerDown(props)} />}
            onPress={() => {
              go(section.id);
            }}
          >
            <Icon name={section.icon} place="action" />
          </AriaButton>
        ))}
      </Toolbar>
      <div
        ref={scroller}
        role="grid"
        aria-label={t("emoji_picker.grid")}
        aria-rowcount={Math.max(1, layout.rows.length)}
        aria-colcount={columns}
        className={s.grid()}
        onKeyDown={onKeyDown}
      >
        <div
          role="presentation"
          className={s.rows()}
          // The line that none match flows in it; the rows are placed in the height they need together.
          style={layout.rows.length === 0 ? undefined : { blockSize: rows.total }}
        >
          <div ref={probe} aria-hidden="true" className={s.probe()}>
            <div className={`${s.heading()} ${s.probeHeading()}`}>&nbsp;</div>
            <div className={cellClass} />
            <div className={cellClass} />
          </div>
          {layout.rows.length === 0 ? (
            <div role="row" aria-rowindex={1}>
              <div role="gridcell" aria-colspan={columns} className={s.empty()}>
                {t("emoji_picker.empty", { query: searched })}
              </div>
            </div>
          ) : (
            groups.map((group) => (
              <div key={group.section} role="rowgroup" aria-label={group.label}>
                {group.items.map((item) => {
                  const row = layout.rows[item.index];
                  if (row === undefined) return null;
                  const style = {
                    position: "absolute",
                    insetBlockStart: 0,
                    insetInline: 0,
                    blockSize: item.size,
                    transform: `translateY(${String(item.start)}px)`,
                  } as const;
                  if (row.kind === "heading")
                    return (
                      <div key={row.key} role="row" aria-rowindex={item.index + 1} style={style}>
                        <div role="rowheader" aria-colspan={columns} className={s.heading()}>
                          {row.label}
                        </div>
                      </div>
                    );
                  return (
                    <div
                      key={row.key}
                      role="row"
                      aria-rowindex={item.index + 1}
                      className={s.row()}
                      style={{ ...style, gridTemplateColumns: `repeat(${String(columns)}, minmax(0, 1fr))` }}
                    >
                      {layout.cells.slice(row.first, row.last + 1).map((cell, offset) => {
                        const own = cell.emoji.key === current;
                        return (
                          <EmojiCell
                            key={cell.id}
                            id={cell.id}
                            emoji={cell.emoji}
                            column={cell.column}
                            label={own ? t("emoji_picker.current", { name: cell.emoji.name }) : cell.emoji.name}
                            tabbable={row.first + offset === activeIndex}
                            current={own}
                            shown={textIn(cell.emoji, tone)}
                            onPick={onPick}
                            onFocusCell={setActiveId}
                          />
                        );
                      })}
                    </div>
                  );
                })}
              </div>
            ))
          )}
        </div>
      </div>
      <div role="status" className={s.status()}>
        {searching && announced?.query === searched ? t("emoji_picker.count", { count: announced.count }) : ""}
      </div>
    </div>
  );
}
