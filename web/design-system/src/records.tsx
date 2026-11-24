// SPDX-License-Identifier: AGPL-3.0-only

// The lists of records of the staff's screens (moderation, administration, operations; ADR-0058, NFR-MOD-7): reports,
// accounts, domain actions, jobs. A table of records is read by row and column: a record a row, its head (what names
// it, the link that opens it) the row's header, a column a kind of fact, and the column it is sorted by says so with
// its direction (aria-sort), in its header's glyph and to assistive technology. It knows its own width: wide (a
// tablet's column and wider), the columns side by side under their names; narrow (a phone, the column beside the main
// one), each record stacked: its head and its state on a line, its other facts in a line under it, each after its
// column's name, which the eye needs there and assistive technology hears from the column's header; over it, select
// all and the sort, one control naming the column and the direction. A head (a handle, a domain) never breaks inside. Rows stand apart by space alone and mark their extent with a
// surface while hovered or focused, the open record with the raised surface, as the current tab. A row may be picked
// by its checkbox for one action on all the rows picked, the actions over the table beside how many are picked. A
// state is a glyph of its own shape beside its word, never a tile, its weight and tone by whether it waits for someone.
//
// Over a list, a filter bar: a search, then the facets, a button each that opens the facet's values in a menu and,
// while one is applied, is knocked out and says it, and Clear filters at their end. Under it, the pager (pager.tsx). Where a list has nothing to show, the
// feed's notice says why (nothing yet, nothing that matches) and what to do next. A detail beside its list is the app's frame:
// the record in the main column, the list compact (its head and state columns alone) in the column beside it from
// `wide`, the open record marked, and under `wide` the record alone with the way back over it (`ScreenTitle`).
//
// Every word, figure and date is the app's but the few the parts say themselves (select all, how many are picked, the
// pages, clear filters), which are the design system's catalog's.

import {
  createContext,
  Fragment,
  use,
  useEffect,
  useId,
  useRef,
  type CSSProperties,
  type HTMLAttributes,
  type ReactNode,
  type RefCallback,
} from "react";
import { Button as AriaButton, Link } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { Button, ButtonBase, TextButton } from "./button";
import { ChoiceMark } from "./choice";
import { Icon, type IconName } from "./icons";
import { codeRuns } from "./code-text";
import { Box, Pin, Row, Spoken, Stack } from "./layout";
import { stringsIn, useReader } from "./locale";
import { MenuGroup, MenuItem, MenuSurface } from "./menu";
import { SearchField } from "./search-field";

// The state of a record.

/** The states a record of the staff's screens is in. */
export type RecordState =
  | "open"
  | "assigned"
  | "resolved"
  | "active"
  | "limited"
  | "suspended"
  | "permanent"
  | "expired"
  | "lifted"
  | "pending"
  | "ending"
  | "failed"
  | "disabled"
  | "queued"
  | "running"
  | "retrying"
  | "muted";

const stateGlyph: Record<RecordState, IconName> = {
  open: "stateOpen",
  assigned: "stateAssigned",
  resolved: "stateResolved",
  active: "stateActive",
  limited: "hidden",
  suspended: "block",
  permanent: "block",
  expired: "stateExpired",
  lifted: "stateLifted",
  pending: "statePending",
  // In force until a date it has (a cost's last payment, a subscription's period end).
  ending: "schedule",
  failed: "error",
  disabled: "stateDisabled",
  queued: "stateQueued",
  running: "stateRunning",
  retrying: "stateRetrying",
  muted: "stateMuted",
};

/**
 * How a state weighs: what asks someone to act (open, failed) and what shuts something out (suspended, and permanent,
 * which nothing lifts) in the text's color and a label's weight; what is in force or under way (assigned, active,
 * limited, a job queued, running or waiting to run again, a muted error that still counts) and what waits for a date or
 * a payment of its own (pending, ending) in the text's color; what is over (resolved, expired, lifted, disabled) muted.
 */
const stateTone = {
  open: "waits",
  pending: "holds",
  ending: "holds",
  failed: "waits",
  assigned: "holds",
  active: "holds",
  limited: "holds",
  suspended: "waits",
  permanent: "waits",
  resolved: "over",
  expired: "over",
  lifted: "over",
  disabled: "over",
  queued: "holds",
  running: "holds",
  retrying: "holds",
  muted: "holds",
} as const satisfies Record<RecordState, string>;

// The size of what holds it (a fact's value, a cell), never smaller than what it is the state of.
const stateMark = tv({
  base: "[word-break:auto-phrase]",
  variants: {
    tone: { waits: "font-label text-text", holds: "text-text", over: "text-muted-text" },
  },
});

/**
 * A record's state: its glyph, of a shape of its own, and its word, the app's ("Open", "Expired"). It stands on its
 * word's baseline, so that the word lines up with the text beside it (a row's other cells), the glyph on its middle.
 */
export function StateMark({ state, children }: { state: RecordState; children: string }) {
  return (
    <Row as="span" inline align="center" gap="entry-caption" className={stateMark({ tone: stateTone[state] })}>
      <Icon name={stateGlyph[state]} place="caption" />
      <Box as="span" self="baseline">
        {children}
      </Box>
    </Row>
  );
}

// The table of records.

/**
 * A column of a table of records after its head: its name; what it holds (words share the room left; figures, at its
 * end, and `fit`, a state or a date, keep their width); whether the table sorts by it, and in which direction a first
 * press sorts; and `beside`, on a narrow table at the end of the head's line (a state), not on a line of its own.
 */
export type RecordColumn<K extends string = string> = {
  key: K;
  label: string;
  kind?: "words" | "figures" | "fit" | undefined;
  sorts?: RecordDirection | undefined;
  beside?: boolean | undefined;
  /**
   * On a narrow table, its value alone in the line of facts, muted at the caption's size, without its column's name: a
   * fact that says what it is by itself (a category, a time).
   */
  bare?: boolean | undefined;
};

export type RecordDirection = "ascending" | "descending";

/** What a table of records is sorted by: a column's key, or `head`, the rows' heads; and in which direction. */
export type RecordSort<K extends string = string> = { by: K | "head"; direction: RecordDirection };

/**
 * A record: its id; its head, which names it (an account's handle, a domain), and a muted line under it (who
 * reported it, a report's number); where it opens (`href`); and its cells by column, as the app formats them.
 */
export type RecordRow<K extends string = string> = {
  id: string;
  head: string;
  caption?: string | undefined;
  /**
   * The caption is copied or compared character by character (an address, as a fact of its record's page shows it): in
   * the code's face, breaking after a slash rather than inside a word.
   */
  captionCode?: boolean | undefined;
  href?: string | undefined;
  cells: Partial<Record<K, ReactNode>>;
  /** Its head and its caption are code (a job's type, an error's message and route): in the code's face. */
  code?: boolean | undefined;
  /**
   * Its head is a domain, which names it only whole: it wraps after its dots where its column has no more room, never
   * cut short.
   */
  wraps?: boolean | undefined;
  /**
   * What the record holds past its cells, across the whole of the row under them (a person's reason, their private
   * facts, the actions on that one record): at any width a line of its own, the row's last.
   */
  detail?: ReactNode;
};

/** An action on every row picked: its words, what it does, and whether it is under way. */
export type RecordAction = { id: string; label: string; onPress: () => void; busy?: boolean | undefined };

export type RecordTableProps<K extends string> = {
  /** What the table is, for assistive technology ("Reports"). */
  label: string;
  /** The name of the rows' heads' column ("Account", "Domain"), and the direction a first press sorts by it in. */
  head: { label: string; sorts?: RecordDirection | undefined };
  columns: readonly RecordColumn<K>[];
  rows: readonly RecordRow<K>[];
  /** What it is sorted by; with `onSort`, the columns that sort are buttons. */
  sort?: RecordSort<K> | undefined;
  onSort?: ((sort: RecordSort<K>) => void) | undefined;
  /** The id of the record open beside it or in place of it, marked. */
  current?: string | undefined;
  /** A row opens by a press, where it has no `href` (a detail the screen shows in place of the list). */
  onOpen?: ((id: string) => void) | undefined;
  /**
   * The rows picked, and the actions on all of them, over the table: its rows each start with a checkbox. The selection
   * is of the rows shown: every change drops the ids of rows not in `rows`.
   */
  selection?:
    | {
        selected: ReadonlySet<string>;
        onChange: (selected: ReadonlySet<string>) => void;
        actions: readonly RecordAction[];
        /** Whether the row of `id` may be picked (one not decided yet); every row where left out. */
        selectable?: ((id: string) => boolean) | undefined;
      }
    | undefined;
  /** What it shows changes (sorted anew, the next page): its rows faint, and busy for assistive technology. */
  busy?: boolean | undefined;
  /** In place of the table, while it has no row: a `FeedNotice` of why (nothing yet, nothing matches) and what to do. */
  empty?: ReactNode;
  /**
   * The heads' column and the columns of words share the room evenly, whatever the rows hold, so that the columns stand
   * in one place from one filter or page to the next; the heads' captions wrap only at that share.
   */
  even?: boolean | undefined;
  /**
   * Its rows, and the surface of the one hovered or open, keep within its own width, for a place with no room past its
   * edges (the column beside the main one), their words set in by a row's inset; by default they reach past it by that
   * inset, so that their words line up with what stands above and below.
   */
  contained?: boolean | undefined;
  /**
   * The ref of the row at `index` (from 0), for its focusable element: the link or the button that opens it, else its
   * head, which `useAddedFocus` makes focusable by script alone. A list that grows under Show more passes `useAddedFocus().item`, so that the
   * first row added takes the focus.
   */
  itemRef?: ((index: number) => RefCallback<HTMLElement> | undefined) | undefined;
};

// A column of words keeps its longest word and shares the room left; the heads' column takes what its heads need, up
// to all the room the others leave, and cuts a head short only past that. With no column of words, the heads' column
// (`minmax(0,auto)`, which the grid stretches) takes the room left, so that the table fills its column.
const TRACK = { words: "minmax(min-content,1fr)", figures: "max-content", fit: "max-content" } as const;

/**
 * The room of a table, which knows its own width: what stands over the table on a narrow one alone (select all and
 * the sort), then the table.
 */
function RecordRoom({ children }: { children: ReactNode }) {
  return <div className="@container grid grid-cols-[minmax(0,1fr)] gap-y-entry-line">{children}</div>;
}

/**
 * The table's frame: narrow (under 30rem), a record under another; wide, the tracks of its columns, which the table
 * gives (`tracks`), each row on them. It reaches past its container by a row's inset, so that what the rows hold lines
 * up with what stands above and below the table.
 */
function RecordFrame({
  tracks,
  contained,
  className,
  ...rest
}: HTMLAttributes<HTMLDivElement> & { tracks: string; contained: boolean; className: string }) {
  return (
    <div
      {...rest}
      style={{ "--record-tracks": tracks } as CSSProperties}
      className={`${className} ${contained ? "" : "-mx-screen-row-inline"} grid grid-cols-[minmax(0,1fr)] @min-[30rem]:grid-cols-[var(--record-tracks)] @min-[30rem]:gap-x-shell-edge`}
    />
  );
}

/** What stands over a narrow table alone: select all at the start, the sort at the end. */
function NarrowStrip({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-control-timeline items-center gap-screen-row-gap @min-[30rem]:hidden">{children}</div>
  );
}

/**
 * A row of the table. `head`, the row of the columns' names: narrow, its names read alone (the strip over the table
 * shows its controls); wide, on the table's tracks. A record: narrow, its checkbox, its head and what stands beside it on the
 * first line, then its facts in a line that wraps; wide, on the table's tracks, its cells on their first baseline. The
 * record is what its head's link covers.
 */
function RecordLine({
  head = false,
  className,
  ...rest
}: HTMLAttributes<HTMLDivElement> & { head?: boolean; className: string }) {
  const shown = head
    ? [
        "flex",
        "@min-[30rem]:col-span-full @min-[30rem]:grid @min-[30rem]:grid-cols-subgrid @min-[30rem]:items-center",
        "@min-[30rem]:gap-x-shell-edge @min-[30rem]:pb-screen-air",
      ].join(" ")
    : [
        // Narrow, a break after the first line (its ::after, a line of its own) sends the facts under it.
        "relative flex flex-wrap items-baseline gap-x-screen-row-gap gap-y-hair py-screen-row-block",
        "after:order-1 after:basis-full after:content-['']",
        "@min-[30rem]:col-span-full @min-[30rem]:grid @min-[30rem]:grid-cols-subgrid @min-[30rem]:gap-x-shell-edge",
        "@min-[30rem]:gap-y-0",
        "@min-[30rem]:after:hidden",
      ].join(" ");
  return <div {...rest} className={`${className} ${shown}`} />;
}

const PLACE = {
  // What a record holds across the row, under its cells: narrow, after its facts; wide, every column. It asks the
  // columns for no width of its own, so that a long line (an error, a URL) wraps inside the row rather than widening
  // the columns it spans past the table's edge and squeezing the heads' column to nothing.
  detail: "order-3 basis-full pt-entry-line [contain:inline-size] @min-[30rem]:col-span-full",
  // A row's checkbox, or select all: on the first line's height, over the link that covers the row.
  pick: "relative z-1 flex h-[1lh] flex-none items-center self-start",
  // The head of a record: narrow, the first line's room; wide, its column.
  head: "min-w-0 flex-[1_1_0%]",
  // What stands beside the head on a narrow table (a state): at the first line's end; wide, its column.
  beside: "flex-none",
  // Figures beside the head (a price): narrow, at the first line's end in a room of their own, so that what stands
  // before them keeps its place from row to row; wide, at their column's end.
  besideFigures: "min-w-[5.5rem] flex-none text-end",
  // A fact: narrow, after the first line, its column's name before it, in a line that wraps; wide, its column.
  fact: "order-2 inline-flex min-w-0 items-baseline gap-x-entry-caption @min-[30rem]:order-none @min-[30rem]:block",
  // A fact that says what it is by itself: narrow, as a fact, muted at the caption's size, with no column's name.
  bare: [
    "order-2 inline-flex min-w-0 items-baseline gap-x-entry-caption @min-[30rem]:order-none @min-[30rem]:block",
    "@max-[30rem]:text-caption @max-[30rem]:text-muted-text",
  ].join(" "),
  // A fact of figures: wide, at its column's end.
  figures: [
    "order-2 inline-flex min-w-0 items-baseline gap-x-entry-caption",
    "@min-[30rem]:order-none @min-[30rem]:block @min-[30rem]:text-end",
  ].join(" "),
  // A fact the record has none of: narrow, nothing shown, still read as its column's cell; wide, its column, empty.
  blank: "sr-only @min-[30rem]:not-sr-only",
  // The name of a column: narrow, read alone; wide, over it, at its end for a column of figures.
  name: "sr-only @min-[30rem]:not-sr-only",
  "figures-name": "sr-only @min-[30rem]:not-sr-only @min-[30rem]:text-end",
  // The name of a column the table sorts by, a button on a wide table: the same.
  sorts: "sr-only @min-[30rem]:not-sr-only",
  "figures-sorts": "sr-only @min-[30rem]:not-sr-only @min-[30rem]:justify-self-end",
  // Select all, a checkbox on a wide table.
  "pick-name":
    "sr-only @min-[30rem]:not-sr-only @min-[30rem]:relative @min-[30rem]:flex @min-[30rem]:h-[1lh] @min-[30rem]:items-center",
} as const;

/**
 * Where a cell stands in its row; a control in a cell (a link to an account) stands over the link that covers it, but
 * the head's own link, whose box past it is the cover, stays unpositioned so that the cover reaches the whole row.
 */
function RecordPlace({
  at,
  className,
  ...rest
}: { at: keyof typeof PLACE; className: string } & HTMLAttributes<HTMLElement>) {
  return <div {...rest} className={`${className} ${PLACE[at]} [&_:is(a,button):not([data-open])]:relative`} />;
}

/** What a narrow table shows alone: a fact's column's name before it, select all's words. */
function NarrowOnly({ children }: { children: ReactNode }) {
  return (
    <span aria-hidden="true" className="@min-[30rem]:hidden">
      {children}
    </span>
  );
}

/** What a wide table shows alone: a column's controls (its sort, select all), which the narrow one's strip holds. */
function WideOnly({ children }: { children: ReactNode }) {
  return <span className="hidden @min-[30rem]:contents">{children}</span>;
}

/** What a narrow table reads alone: a column's name, which its button says on a wide one. */
function NarrowName({ children }: { children: ReactNode }) {
  return <span className="@min-[30rem]:hidden">{children}</span>;
}

/**
 * A record's head, isolated in its own direction (a handle reads the same in a right-to-left page), on one line, cut
 * short with an ellipsis only where its column has no more room: a handle never breaks inside. `wraps`, a domain, it
 * wraps after its dots instead, and inside a label only where one is longer than the line. `cover`, its box past it
 * covers the whole row, so that a press anywhere opens the record.
 */
function HeadWords({
  cover = false,
  code = false,
  wraps = false,
  children,
}: {
  cover?: boolean;
  code?: boolean;
  wraps?: boolean;
  children: string;
}) {
  // Its own direction cuts it at its end in its reading order (a handle keeps its "@" and its start in a right-to-left
  // page), standing at the start of its column all the same; code reads left to right.
  return (
    <span
      dir={code ? "ltr" : "auto"}
      className={`block ${wraps ? "[overflow-wrap:anywhere]" : "truncate"} [text-align:match-parent] ${code ? "font-mono" : ""} ${cover ? "after:absolute after:inset-0 after:content-['']" : ""}`}
    >
      {wraps
        ? children.split(/(?<=\.)/).map((label, i) => (
            // A label at a time, a break allowed after each dot; a domain's labels keep their order.
            <Fragment key={i}>
              {i > 0 && <wbr />}
              {label}
            </Fragment>
          ))
        : children}
    </span>
  );
}

const table = tv({
  slots: {
    root: "text-body text-text",
    busy: "opacity-disabled",
    name: "text-caption text-muted-text [word-break:auto-phrase]",
    line: [
      "rounded-control px-screen-row-inline",
      // The record marks its extent while its link is hovered or focused (the link covers the row), the open one with
      // the raised surface.
      "has-[[data-open][data-hovered]]:bg-surface-hover has-[[data-open][data-focus-visible]]:shadow-focus",
      "has-[[data-open][data-pressed]]:translate-y-press has-[[data-open][data-pressed]]:bg-surface-pressed",
      "has-[[data-open][aria-current]]:bg-surface-raised",
      "has-[[data-open][aria-current][data-pressed]]:bg-surface-raised-pressed",
    ],
    headLine: "px-screen-row-inline",
    // A head, a link or a button, in the page's face and type, whatever element it is.
    open: [
      "cursor-pointer border-0 bg-transparent p-0 text-start font-sans text-body font-label text-text no-underline",
      "outline-hidden",
    ],
    plain: "font-label",
    // Under the head, wrapping as its column allows: it asks the column for no width of its own.
    caption: "text-caption text-muted-text [contain:inline-size] [overflow-wrap:break-word]",
    captionCode: "font-mono",
    factName: "text-caption text-muted-text whitespace-nowrap",
    fact: "[overflow-wrap:break-word]",
    figure: "tabular-nums",
    // A column's name that sorts: muted, the text's color while it is the one sorted by, its glyph after it.
    sort: [
      "box-border min-h-control-timeline cursor-pointer rounded-control border-0 bg-transparent px-control-timeline-inline",
      "font-sans text-caption whitespace-nowrap text-muted-text outline-hidden",
      "data-hovered:bg-surface-hover data-hovered:text-text data-focus-visible:shadow-focus",
      "data-pressed:translate-y-press data-pressed:bg-surface-pressed data-pressed:text-text",
    ],
    sorted: "font-label text-text",
    pick: [
      "cursor-pointer rounded-control has-[:focus-visible]:shadow-focus",
      "has-[input:enabled]:active:translate-y-press",
    ],
    // Not drawn: the mark beside it shows its state, the target its focus.
    pickInput: "cursor-pointer opacity-0",
    pickWords: "text-caption text-muted-text",
  },
});

/**
 * A checkbox of the table: its mark, knocked out while checked, a dash while some of the rows are. The native checkbox
 * takes the press over the whole of its target, not drawn, the mark under it showing its state and its focus, so that
 * a press, a screen reader's pointer and a click by role all land on the control itself.
 */
function Pick({
  label,
  checked,
  mixed = false,
  onChange,
  words,
}: {
  label: string;
  checked: boolean;
  mixed?: boolean;
  onChange: (checked: boolean) => void;
  /** Its words, shown beside it on a narrow table (select all). */
  words?: string | undefined;
}) {
  const s = table();
  const input = useRef<HTMLInputElement>(null);
  // At every render: a press sets the native state to checked or not, and leaves no dash, whatever `mixed` says.
  useEffect(() => {
    if (input.current !== null) input.current.indeterminate = mixed;
  });
  return (
    <Row as="label" inline anchor align="center" gap="entry-caption" className={s.pick()}>
      <Pin
        cover
        as={
          <input
            ref={input}
            type="checkbox"
            className={s.pickInput()}
            aria-label={label}
            checked={checked}
            onChange={(event) => {
              onChange(event.target.checked);
              // The press cleared the native dash; it stays until what holds the table says otherwise.
              event.target.indeterminate = mixed;
            }}
          />
        }
      />
      <ChoiceMark multiple on={checked} mixed={mixed} />
      {words !== undefined && (
        <NarrowOnly>
          <span className={s.pickWords()}>{words}</span>
        </NarrowOnly>
      )}
    </Row>
  );
}

/** The glyph of a column that sorts: up or down for the one sorted by, both for the others. */
const sortGlyph = (direction: RecordDirection | undefined): IconName =>
  direction === "ascending" ? "sortedUp" : direction === "descending" ? "sortedDown" : "sortable";

/** The name of a column that sorts, a button: a press sorts by it, or turns its direction where it is sorted by. */
function SortButton({
  label,
  direction,
  onPress,
  end = false,
}: {
  label: string;
  direction: RecordDirection | undefined;
  onPress: () => void;
  /** It names a column of figures: its words line up with the figures' end. */
  end?: boolean;
}) {
  const s = table();
  return (
    <Row
      inline
      align="center"
      gap="entry-caption"
      {...(end ? { bleedEnd: "control-timeline-inline" as const } : { bleedStart: "control-timeline-inline" as const })}
      as={<AriaButton onPress={onPress} className={`${s.sort()} ${direction === undefined ? "" : s.sorted()}`} />}
    >
      <span>{label}</span>
      <Icon name={sortGlyph(direction)} place="caption" />
    </Row>
  );
}

/**
 * The sort of a narrow table, one control: the column it is sorted by and its glyph of the direction, read with the
 * direction's word; it opens a menu of the columns that sort and of the two directions.
 */
function SortMenu<K extends string>({
  options,
  sort,
  onSort,
}: {
  options: readonly { by: K | "head"; label: string; first: RecordDirection }[];
  sort: RecordSort<K> | undefined;
  onSort: (sort: RecordSort<K>) => void;
}) {
  const s = table();
  const { lang } = useReader(undefined);
  const { t } = stringsIn(lang);
  const now = options.find((option) => option.by === sort?.by);
  return (
    <MenuSurface
      label={t("records.sort_by")}
      placement="bottom end"
      trigger={
        <Row
          inline
          align="center"
          gap="entry-caption"
          bleedEnd="control-timeline-inline"
          as={<AriaButton className={`${s.sort()} ${now === undefined ? "" : s.sorted()}`} />}
        >
          <span>{now?.label ?? t("records.sort_by")}</span>
          {sort !== undefined && now !== undefined && (
            <>
              <Icon name={sortGlyph(sort.direction)} place="caption" />
              <Spoken>{` ${t(sort.direction === "ascending" ? "records.ascending" : "records.descending")}`}</Spoken>
            </>
          )}
        </Row>
      }
    >
      <MenuGroup
        chosen={now?.by}
        onChoose={(by) => {
          const option = options.find((o) => o.by === by);
          if (option !== undefined) onSort({ by: option.by, direction: option.first });
        }}
      >
        {options.map((option) => (
          <MenuItem key={option.by} id={option.by} icon="sortable">
            {option.label}
          </MenuItem>
        ))}
      </MenuGroup>
      {now !== undefined && sort !== undefined && (
        <MenuGroup
          chosen={sort.direction}
          onChoose={(direction) => {
            onSort({ by: now.by, direction: direction === "ascending" ? "ascending" : "descending" });
          }}
        >
          <MenuItem id="ascending" icon="sortedUp">
            {t("records.ascending")}
          </MenuItem>
          <MenuItem id="descending" icon="sortedDown">
            {t("records.descending")}
          </MenuItem>
        </MenuGroup>
      )}
    </MenuSurface>
  );
}

/** What the rows of a table know of it: whether they are picked, and what opens one. */
const RowsContext = createContext<{
  picked: ReadonlySet<string> | undefined;
  onPick: (id: string, checked: boolean) => void;
  onOpen: ((id: string) => void) | undefined;
  current: string | undefined;
}>({ picked: undefined, onPick: () => undefined, onOpen: undefined, current: undefined });

/**
 * A table of records, named `label`, of the rows' heads' column and `columns`: for assistive technology a table whose
 * every cell is read with its row's head and its column's name ("@spam@bad.example, State, Open"), the column it is
 * sorted by with its direction. With `selection`, the actions on the rows picked over it, beside how many are picked
 * (said as it changes), and select all over the rows' checkboxes. With no row, `empty` in its place.
 */
export function RecordTable<K extends string>({
  label,
  head,
  columns,
  rows,
  sort,
  onSort,
  current,
  onOpen,
  selection,
  busy = false,
  empty,
  even = false,
  contained = false,
  itemRef,
}: RecordTableProps<K>) {
  const s = table();
  const { lang } = useReader(undefined);
  const { t } = stringsIn(lang);
  if (rows.length === 0 && empty !== undefined) return <>{empty}</>;
  const picks = selection !== undefined;
  const picked = selection?.selected;
  // The rows that may be picked, which select all checks and counts: a row that may not is no part of it.
  const pickable = rows.filter((row) => selection?.selectable?.(row.id) ?? true);
  const count = pickable.filter((row) => picked?.has(row.id) === true).length;
  const share = "minmax(0,1fr)";
  const tracks = [
    picks ? "max-content" : "",
    even ? share : "minmax(0,auto)",
    ...columns.map((c) => (even && (c.kind ?? "words") === "words" ? share : TRACK[c.kind ?? "words"])),
  ]
    .filter((track) => track !== "")
    .join(" ");
  const sortable = onSort !== undefined;
  const directionOf = (by: K | "head") => (sort?.by === by ? sort.direction : undefined);
  const press = (by: K | "head", first: RecordDirection | undefined) => () => {
    const now = directionOf(by);
    onSort?.({
      by,
      direction: now === undefined ? (first ?? "ascending") : now === "ascending" ? "descending" : "ascending",
    });
  };
  const ariaSort = (by: K | "head") => directionOf(by);
  // The selection is of the rows shown: an id of a row no longer shown (filtered out, another page) leaves it with
  // every change, so that the count and the actions always speak of the same rows.
  const shown = new Set(pickable.map((row) => row.id));
  const selectAll = (checked: boolean) => {
    selection?.onChange(new Set(checked ? shown : []));
  };
  // The columns that sort, the heads' first, for the sort of a narrow table.
  const options = sortable
    ? [
        ...(head.sorts === undefined ? [] : [{ by: "head" as const, label: head.label, first: head.sorts }]),
        ...columns.flatMap((c) => (c.sorts === undefined ? [] : [{ by: c.key, label: c.label, first: c.sorts }])),
      ]
    : [];
  return (
    <Stack gap="entry-line">
      {selection !== undefined && (
        <Row wrap align="center" gapX="entry-avatar" gapY="screen-tab-gap">
          <Box grow as="p" role="status" text="caption" tone="muted">
            {t("records.selected", { count })}
          </Box>
          <Row wrap align="center" gap="screen-tab-gap">
            {selection.actions.map((action) => (
              <Button
                key={action.id}
                kind="secondary"
                size="timeline"
                isDisabled={count === 0}
                busy={action.busy === true}
                onPress={action.onPress}
              >
                {action.label}
              </Button>
            ))}
          </Row>
        </Row>
      )}
      <RecordRoom>
        {(selection !== undefined || options.length > 0) && (
          <NarrowStrip>
            {selection !== undefined && (
              <Pick
                label={t("records.select_all")}
                words={t("records.select_all")}
                checked={count > 0 && count === pickable.length}
                mixed={count > 0 && count < pickable.length}
                onChange={selectAll}
              />
            )}
            {options.length > 0 && onSort !== undefined && (
              <Box push="end">
                <SortMenu options={options} sort={sort} onSort={onSort} />
              </Box>
            )}
          </NarrowStrip>
        )}
        <RecordFrame
          role="table"
          aria-label={label}
          aria-busy={busy || undefined}
          tracks={tracks}
          contained={contained}
          className={`${s.root()} ${busy ? s.busy() : ""}`}
        >
          <RecordLine role="row" head className={s.headLine()}>
            {selection !== undefined && (
              <RecordPlace at="pick-name" role="columnheader" className="">
                <WideOnly>
                  <Pick
                    label={t("records.select_all")}
                    checked={count > 0 && count === pickable.length}
                    mixed={count > 0 && count < pickable.length}
                    onChange={selectAll}
                  />
                </WideOnly>
                <NarrowName>{t("records.select_all")}</NarrowName>
              </RecordPlace>
            )}
            <RecordPlace
              at={sortable && head.sorts !== undefined ? "sorts" : "name"}
              role="columnheader"
              aria-sort={ariaSort("head")}
              className={s.name()}
            >
              {sortable && head.sorts !== undefined ? (
                <>
                  <WideOnly>
                    <SortButton
                      label={head.label}
                      direction={directionOf("head")}
                      onPress={press("head", head.sorts)}
                    />
                  </WideOnly>
                  <NarrowName>{head.label}</NarrowName>
                </>
              ) : (
                head.label
              )}
            </RecordPlace>
            {columns.map((column) => {
              const sorts = sortable && column.sorts !== undefined;
              const figures = column.kind === "figures";
              return (
                <RecordPlace
                  key={column.key}
                  at={sorts ? (figures ? "figures-sorts" : "sorts") : figures ? "figures-name" : "name"}
                  role="columnheader"
                  aria-sort={ariaSort(column.key)}
                  className={s.name()}
                >
                  {sorts ? (
                    <>
                      <WideOnly>
                        <SortButton
                          label={column.label}
                          direction={directionOf(column.key)}
                          onPress={press(column.key, column.sorts)}
                          end={figures}
                        />
                      </WideOnly>
                      <NarrowName>{column.label}</NarrowName>
                    </>
                  ) : (
                    column.label
                  )}
                </RecordPlace>
              );
            })}
          </RecordLine>
          <RowsContext
            value={{
              picked,
              onPick: (id, checked) => {
                if (selection === undefined) return;
                const next = new Set([...selection.selected].filter((kept) => shown.has(kept)));
                if (checked) next.add(id);
                else next.delete(id);
                selection.onChange(next);
              },
              onOpen,
              current,
            }}
          >
            {rows.map((row, index) => (
              <Record
                key={row.id}
                row={row}
                focusRef={itemRef?.(index)}
                columns={columns}
                picks={picks}
                pickable={selection?.selectable?.(row.id) ?? true}
              />
            ))}
          </RowsContext>
        </RecordFrame>
      </RecordRoom>
    </Stack>
  );
}

/** A record: its checkbox, its head (the link that opens it) over its caption, and its cells. */
function Record<K extends string>({
  row,
  focusRef,
  columns,
  picks,
  pickable,
}: {
  row: RecordRow<K>;
  /** The ref of its focusable element (`itemRef`). */
  focusRef: RefCallback<HTMLElement> | undefined;
  columns: readonly RecordColumn<K>[];
  picks: boolean;
  /** Its checkbox is there: it may be picked. */
  pickable: boolean;
}) {
  const s = table();
  const { lang } = useReader(undefined);
  const { t } = stringsIn(lang);
  const { picked, onPick, onOpen, current } = use(RowsContext);
  // The open record: the page it leads to, for a link; the one shown, for a button.
  const open = row.id === current;
  const code = row.code === true;
  const words = (
    <HeadWords cover code={code} wraps={row.wraps === true}>
      {row.head}
    </HeadWords>
  );
  return (
    <RecordLine role="row" className={s.line()}>
      {picks && (
        <RecordPlace at="pick" role="cell" className="">
          {pickable && (
            <Pick
              label={t("records.select_row", { name: row.head })}
              checked={picked?.has(row.id) === true}
              onChange={(checked) => {
                onPick(row.id, checked);
              }}
            />
          )}
        </RecordPlace>
      )}
      <RecordPlace at="head" role="rowheader" className="">
        <Stack as="span" gap="hair">
          {row.href !== undefined ? (
            <Link
              ref={focusRef}
              href={row.href}
              data-open=""
              {...(open ? { "aria-current": "page" as const } : {})}
              className={s.open()}
            >
              {words}
            </Link>
          ) : onOpen !== undefined ? (
            <AriaButton
              ref={focusRef}
              data-open=""
              {...(open ? { "aria-current": "true" as const } : {})}
              onPress={() => {
                onOpen(row.id);
              }}
              className={s.open()}
            >
              {words}
            </AriaButton>
          ) : (
            <span ref={focusRef} className={s.plain()}>
              <HeadWords code={code} wraps={row.wraps === true}>
                {row.head}
              </HeadWords>
            </span>
          )}
          {row.caption !== undefined && (
            <span className={s.caption()}>
              {code || row.captionCode === true ? (
                <bdi dir="ltr" className={s.captionCode()}>
                  {codeRuns(row.caption)}
                </bdi>
              ) : (
                <bdi dir="auto">{row.caption}</bdi>
              )}
            </span>
          )}
        </Stack>
      </RecordPlace>
      {columns.map((column) => {
        const value = row.cells[column.key];
        const figures = column.kind === "figures";
        if (value === undefined || value === "")
          return <RecordPlace key={column.key} at="blank" role="cell" className="" />;
        return (
          <RecordPlace
            key={column.key}
            at={
              column.beside === true
                ? figures
                  ? "besideFigures"
                  : "beside"
                : figures
                  ? "figures"
                  : column.bare === true
                    ? "bare"
                    : "fact"
            }
            role="cell"
            className=""
          >
            {column.beside !== true && column.bare !== true && (
              <NarrowOnly>
                <span className={s.factName()}>{column.label}</span>
              </NarrowOnly>
            )}
            <span className={figures ? `${s.fact()} ${s.figure()}` : s.fact()}>{value}</span>
          </RecordPlace>
        );
      })}
      {row.detail !== undefined && (
        <RecordPlace at="detail" role="cell" aria-colspan={columns.length + (picks ? 2 : 1)} className="">
          {row.detail}
        </RecordPlace>
      )}
    </RecordLine>
  );
}

// The filters over a list.

/** A value of a facet: its id, its words and its glyph, as the menu shows it. */
export type FilterValue = { id: string; label: string; icon: IconName };

export type FilterFacetProps = {
  /** What it filters by ("State", "Category"), which names its button and its menu. */
  label: string;
  /** The value of no filter, first in its menu ("Any state"). */
  any: string;
  values: readonly FilterValue[];
  /** The id of the value applied; none, nothing is filtered by it. */
  chosen: string | undefined;
  onChoose: (id: string | undefined) => void;
};

const ANY = "\u0000any";

/**
 * A facet of the filters: a chip that opens its values in a menu, the one applied checked; while a value is applied,
 * the chip is on, knocked out as a chip that is on, and says it ("State: Open"), as the menu button of boost is on.
 */
export function FilterFacet({ label, any, values, chosen, onChoose }: FilterFacetProps) {
  const { lang } = useReader(undefined);
  const { t } = stringsIn(lang);
  const value = values.find((v) => v.id === chosen);
  return (
    <MenuSurface
      label={label}
      placement="bottom start"
      trigger={
        <ButtonBase kind="chip" size="chip" pressed={value !== undefined}>
          <span>{value === undefined ? label : t("records.facet", { facet: label, value: value.label })}</span>
          <Icon name="expand" place="caption" />
        </ButtonBase>
      }
    >
      <MenuGroup
        chosen={value?.id ?? ANY}
        onChoose={(id) => {
          onChoose(id === ANY ? undefined : id);
        }}
      >
        <MenuItem id={ANY} icon="filter">
          {any}
        </MenuItem>
        {values.map((v) => (
          <MenuItem key={v.id} id={v.id} icon={v.icon}>
            {v.label}
          </MenuItem>
        ))}
      </MenuGroup>
    </MenuSurface>
  );
}

/**
 * A facet that is on or off ("Assigned to me"): a toggle on the chip's surface, knocked out while on, its words where
 * a reaction's chip has a glyph and a count.
 */
export function FilterToggle({ label, on, onChange }: { label: string; on: boolean; onChange: (on: boolean) => void }) {
  return (
    <ButtonBase
      kind="chip"
      size="chip"
      pressed={on}
      onPress={() => {
        onChange(!on);
      }}
    >
      {label}
    </ButtonBase>
  );
}

export type FilterBarProps = {
  /** What it filters, for assistive technology ("Filter reports"). */
  label: string;
  /** Its search: the field's label, which stands in it as its placeholder, its words, and the name of its clear. */
  search?:
    | {
        label: string;
        value: string;
        onChange: (value: string) => void;
        clearLabel: string;
        /** What is wrong with its words (the server refused them as what it searches by), under it. */
        error?: string | undefined;
      }
    | undefined;
  /** Its facets: `FilterFacet`s and `FilterToggle`s, in a row that wraps. */
  children: ReactNode;
  /** A filter or the search is applied: Clear filters ends the facets' row. */
  applied: boolean;
  /** Removes every filter and empties the search. */
  onClear: () => void;
  /** How many the list holds as filtered, as the app counts them ("12 reports"), under the facets and said as it changes. */
  summary?: string | undefined;
};

/**
 * The filters over a list: its search, the facets under it in a row that wraps, Clear filters at their end while any is
 * applied, and how many the list holds, a search landmark named `label`.
 */
export function FilterBar({ label, search, children, applied, onClear, summary }: FilterBarProps) {
  const { lang } = useReader(undefined);
  const { t } = stringsIn(lang);
  const id = useId();
  const bar = useId();
  // Clear filters leaves as it acts: the focus goes to the search, else to the first facet, so that it is not lost.
  const clear = () => {
    onClear();
    const root = document.getElementById(bar);
    const to = root?.querySelector<HTMLElement>("input[type=search]") ?? root?.querySelector<HTMLElement>("button");
    to?.focus();
  };
  return (
    <Stack id={bar} role="search" gap="screen-field-gap" aria-label={label}>
      {search !== undefined && (
        <SearchField
          id={id}
          label={search.label}
          labelHidden
          value={search.value}
          onChange={search.onChange}
          clearLabel={search.clearLabel}
          error={search.error}
        />
      )}
      <Row wrap align="center" gap="chip-spacing">
        {children}
        {applied && (
          // In the look of a list's notice's action, so that Clear filters reads alike wherever it stands.
          <Box as="span" push="end" text="caption" tone="muted" nowrap>
            <TextButton onPress={clear}>{t("records.clear_filters")}</TextButton>
          </Box>
        )}
      </Row>
      {summary !== undefined && (
        <Box as="p" role="status" text="caption" tone="muted">
          {summary}
        </Box>
      )}
    </Stack>
  );
}
