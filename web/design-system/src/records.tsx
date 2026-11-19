// SPDX-License-Identifier: AGPL-3.0-only

// The lists of records of the staff's screens (moderation, administration, operations; ADR-0058, NFR-MOD-7): reports,
// accounts, domain actions, jobs. A table of records is read by row and column: a record a row, its head (what names
// it, the link that opens it) the row's header, a column a kind of fact, and the column it is sorted by says so with
// its direction (aria-sort), in its header's glyph and to assistive technology. It knows its own width: wide (a
// tablet's column and wider), the columns side by side under their names; narrow (a phone, the column beside the main
// one), each record stacked: its head and its state on a line, each other fact on a line under it after its column's
// name, which the eye needs there and assistive technology hears from the column's header; the names of the columns
// the table sorts by stay in a row over it, as its sort. Rows stand apart by space alone and mark their extent with a
// surface while hovered or focused, the open record with the raised surface, as the current tab. A row may be picked
// by its checkbox for one action on all the rows picked, the actions over the table beside how many are picked. A
// state is a glyph of its own shape beside its word, never a tile, its weight and tone by whether it waits for someone.
//
// Over a list, a filter bar: a search, then the facets, a button each that opens the facet's values in a menu and,
// while one is applied, is knocked out and says it, and Clear filters at their end. Under it, the pages of a list paged
// by cursor (previous and next), or Show more for one that grows in place. Where a list has nothing to show, the
// feed's notice says why (nothing yet, nothing that matches) and what to do next. A detail beside its list is the app's frame:
// the record in the main column, the list compact (its head and state columns alone) in the column beside it from
// `wide`, the open record marked, and under `wide` the record alone with the way back over it (`ScreenTitle`).
//
// Every word, figure and date is the app's but the few the parts say themselves (select all, how many are picked, the
// pages, clear filters), which are the design system's catalog's.

import { createContext, use, useId, type CSSProperties, type HTMLAttributes, type ReactNode } from "react";
import { Button as AriaButton, CheckboxButton, CheckboxField, Link } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { Button, ButtonBase, LinkButton, TextButton } from "./button";
import { ChoiceMark } from "./choice";
import { Icon, type IconName } from "./icons";
import { Box, Row, Stack } from "./layout";
import { stringsIn, useReader } from "./locale";
import { MenuGroup, MenuItem, MenuSurface } from "./menu";
import { SearchField } from "./search-field";

// The state of a record.

/** The states a record of the staff's screens is in. */
export type RecordState =
  "open" | "assigned" | "resolved" | "active" | "expired" | "lifted" | "pending" | "failed" | "disabled";

const stateGlyph: Record<RecordState, IconName> = {
  open: "stateOpen",
  assigned: "stateAssigned",
  resolved: "stateResolved",
  active: "stateActive",
  expired: "stateExpired",
  lifted: "stateLifted",
  pending: "statePending",
  failed: "error",
  disabled: "stateDisabled",
};

/**
 * How a state weighs: what waits for someone (open, pending, failed) in the text's color and a label's weight; what is
 * in force (assigned, active) in the text's color; what is over (resolved, expired, lifted, disabled) muted.
 */
const stateTone = {
  open: "waits",
  pending: "waits",
  failed: "waits",
  assigned: "holds",
  active: "holds",
  resolved: "over",
  expired: "over",
  lifted: "over",
  disabled: "over",
} as const satisfies Record<RecordState, string>;

const stateMark = tv({
  base: "text-caption [word-break:auto-phrase]",
  variants: {
    tone: { waits: "font-label text-text", holds: "text-text", over: "text-muted-text" },
  },
});

/** A record's state: its glyph, of a shape of its own, and its word, the app's ("Open", "Expired"). */
export function StateMark({ state, children }: { state: RecordState; children: string }) {
  return (
    <Row as="span" inline align="center" gap="entry-caption" className={stateMark({ tone: stateTone[state] })}>
      <Icon name={stateGlyph[state]} place="caption" />
      <span>{children}</span>
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
  href?: string | undefined;
  cells: Partial<Record<K, ReactNode>>;
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
  /** The rows picked, and the actions on all of them, over the table: its rows each start with a checkbox. */
  selection?:
    | {
        selected: ReadonlySet<string>;
        onChange: (selected: ReadonlySet<string>) => void;
        actions: readonly RecordAction[];
      }
    | undefined;
  /** What it shows changes (sorted anew, the next page): its rows faint, and busy for assistive technology. */
  busy?: boolean | undefined;
  /** In place of the table, while it has no row: a `FeedNotice` of why (nothing yet, nothing matches) and what to do. */
  empty?: ReactNode;
};

const TRACK = { words: "minmax(0,1fr)", figures: "max-content", fit: "max-content" } as const;

/**
 * The table's frame, which knows its own width: narrow (under 30rem), a record under another; wide, the tracks of its
 * columns, which the table gives (`tracks`), each row on them. It reaches past its container by a row's inset, so
 * that what the rows hold lines up with what stands above and below the table.
 */
function RecordFrame({
  tracks,
  className,
  ...rest
}: HTMLAttributes<HTMLDivElement> & { tracks: string; className: string }) {
  return (
    <div className="@container">
      <div
        {...rest}
        style={{ "--record-tracks": tracks } as CSSProperties}
        className={`${className} -mx-screen-row-inline grid grid-cols-[minmax(0,1fr)] @min-[30rem]:grid-cols-[var(--record-tracks)] @min-[30rem]:gap-x-entry-avatar`}
      />
    </div>
  );
}

/**
 * A row of the table. `head`, the row of the columns' names: narrow, a row that wraps of those the table sorts by
 * (and select all), the others read alone; wide, on the table's tracks. A record: narrow, its checkbox, its head and
 * what stands beside it on the first line, then its facts in a line that wraps; wide, on the table's tracks, its cells
 * on their first baseline. The record is what its head's link covers.
 */
function RecordLine({
  head = false,
  className,
  ...rest
}: HTMLAttributes<HTMLDivElement> & { head?: boolean; className: string }) {
  const shown = head
    ? [
        "flex flex-wrap items-center gap-screen-tab-gap pb-screen-air",
        "@min-[30rem]:col-span-full @min-[30rem]:grid @min-[30rem]:grid-cols-subgrid @min-[30rem]:gap-x-entry-avatar",
      ].join(" ")
    : [
        // Narrow, a break after the first line (its ::after, a line of its own) sends the facts under it.
        "relative flex flex-wrap items-baseline gap-x-screen-row-gap gap-y-hair py-screen-row-block",
        "after:order-1 after:basis-full after:content-['']",
        "@min-[30rem]:col-span-full @min-[30rem]:grid @min-[30rem]:grid-cols-subgrid @min-[30rem]:gap-x-entry-avatar",
        "@min-[30rem]:gap-y-0",
        "@min-[30rem]:after:hidden",
      ].join(" ");
  return <div {...rest} className={`${className} ${shown}`} />;
}

const PLACE = {
  // A row's checkbox, or select all: on the first line's height, over the link that covers the row.
  pick: "relative z-1 flex h-[1lh] flex-none items-center self-start",
  // The head of a record: narrow, the first line's room; wide, its column.
  head: "min-w-0 flex-[1_1_0%]",
  // What stands beside the head on a narrow table (a state): at the first line's end; wide, its column.
  beside: "flex-none",
  // A fact: narrow, after the first line, its column's name before it, in a line that wraps; wide, its column.
  fact: "order-2 inline-flex min-w-0 items-baseline gap-x-entry-caption @min-[30rem]:order-none @min-[30rem]:block",
  // A fact of figures: wide, at its column's end.
  figures: [
    "order-2 inline-flex min-w-0 items-baseline gap-x-entry-caption",
    "@min-[30rem]:order-none @min-[30rem]:block @min-[30rem]:text-end",
  ].join(" "),
  // A fact the record has none of: narrow, nothing shown, still read as its column's cell; wide, its column, empty.
  blank: "sr-only @min-[30rem]:not-sr-only",
  // The name of a column the table does not sort by: narrow, read alone; wide, over its column.
  name: "sr-only @min-[30rem]:not-sr-only",
  // The name of a column of figures: wide, at its column's end.
  "figures-name": "sr-only @min-[30rem]:not-sr-only @min-[30rem]:text-end",
  // The name of a column the table sorts by, a button: narrow in the row over the table; wide, over its column.
  sorts: "flex-none",
  "figures-sorts": "flex-none @min-[30rem]:justify-self-end",
} as const;

/** Where a cell stands in its row; a control in a cell (a link to an account) stands over the link that covers it. */
function RecordPlace({
  at,
  className,
  ...rest
}: { at: keyof typeof PLACE; className: string } & HTMLAttributes<HTMLElement>) {
  return <div {...rest} className={`${className} ${PLACE[at]} [&_:is(a,button)]:relative`} />;
}

/** What a narrow table shows alone: a fact's column's name before it, select all's words. */
function NarrowOnly({ children }: { children: ReactNode }) {
  return (
    <span aria-hidden="true" className="@min-[30rem]:hidden">
      {children}
    </span>
  );
}

/** The words of a record's head, whose box past them covers the whole row, so that a press anywhere opens it. */
function OpenPlace({ children }: { children: ReactNode }) {
  return <span className="after:absolute after:inset-0 after:content-['']">{children}</span>;
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
      "has-[[data-open][aria-current]]:bg-surface-raised",
    ],
    headLine: "px-screen-row-inline",
    open: "cursor-pointer text-text no-underline outline-hidden font-label [overflow-wrap:anywhere]",
    plain: "font-label [overflow-wrap:anywhere]",
    caption: "text-caption text-muted-text [overflow-wrap:anywhere]",
    factName: "text-caption text-muted-text whitespace-nowrap",
    fact: "[overflow-wrap:anywhere]",
    figure: "tabular-nums",
    // A column's name that sorts: muted, the text's color while it is the one sorted by, its glyph after it.
    sort: [
      "box-border min-h-control-timeline cursor-pointer rounded-control border-0 bg-transparent px-control-timeline-inline",
      "font-sans text-caption whitespace-nowrap text-muted-text outline-hidden",
      "data-hovered:bg-surface-hover data-hovered:text-text data-focus-visible:shadow-focus",
    ],
    sorted: "font-label text-text",
    pick: "cursor-pointer rounded-control outline-hidden data-focus-visible:shadow-focus",
    pickWords: "text-caption text-muted-text",
  },
});

/** A checkbox of the table: its mark, knocked out while checked, a dash while some of the rows are. */
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
  return (
    <CheckboxField aria-label={label} isSelected={checked} isIndeterminate={mixed} onChange={onChange}>
      <Row
        inline
        align="center"
        gap="entry-caption"
        as={
          <CheckboxButton className={s.pick()}>
            {({ isSelected, isIndeterminate }) => (
              <>
                <ChoiceMark multiple on={isSelected} mixed={isIndeterminate} />
                {words !== undefined && (
                  <NarrowOnly>
                    <span className={s.pickWords()}>{words}</span>
                  </NarrowOnly>
                )}
              </>
            )}
          </CheckboxButton>
        }
      />
    </CheckboxField>
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
}: RecordTableProps<K>) {
  const s = table();
  const { lang } = useReader(undefined);
  const { t } = stringsIn(lang);
  if (rows.length === 0 && empty !== undefined) return <>{empty}</>;
  const picks = selection !== undefined;
  const picked = selection?.selected;
  const count = rows.filter((row) => picked?.has(row.id) === true).length;
  const tracks = [picks ? "max-content" : "", "minmax(0,2fr)", ...columns.map((c) => TRACK[c.kind ?? "words"])]
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
      <RecordFrame
        role="table"
        aria-label={label}
        aria-busy={busy || undefined}
        tracks={tracks}
        className={`${s.root()} ${busy ? s.busy() : ""}`}
      >
        <RecordLine role="row" head className={s.headLine()}>
          {selection !== undefined && (
            <RecordPlace at="pick" role="columnheader" className="">
              <Pick
                label={t("records.select_all")}
                words={t("records.select_all")}
                checked={count > 0 && count === rows.length}
                mixed={count > 0 && count < rows.length}
                onChange={(checked) => {
                  selection.onChange(new Set(checked ? rows.map((row) => row.id) : []));
                }}
              />
            </RecordPlace>
          )}
          <RecordPlace
            at={sortable && head.sorts !== undefined ? "sorts" : "name"}
            role="columnheader"
            aria-sort={ariaSort("head")}
            className={s.name()}
          >
            {sortable && head.sorts !== undefined ? (
              <SortButton label={head.label} direction={directionOf("head")} onPress={press("head", head.sorts)} />
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
                  <SortButton
                    label={column.label}
                    direction={directionOf(column.key)}
                    onPress={press(column.key, column.sorts)}
                    end={figures}
                  />
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
              const next = new Set(selection.selected);
              if (checked) next.add(id);
              else next.delete(id);
              selection.onChange(next);
            },
            onOpen,
            current,
          }}
        >
          {rows.map((row) => (
            <Record key={row.id} row={row} columns={columns} picks={picks} />
          ))}
        </RowsContext>
      </RecordFrame>
    </Stack>
  );
}

/**
 * A head with a chance to break before each @ and . inside it (@deals4u@promo.example, a domain), so that a handle or
 * a domain too long for its column wraps between its parts; its text is unchanged for copying and assistive technology.
 */
function breakable(head: string): ReactNode {
  const parts = head.split(/(?<=.)(?=[@.])/u);
  if (parts.length === 1) return head;
  return parts.flatMap((part, i) => (i === 0 ? [part] : [<wbr key={i} />, part]));
}

/** A record: its checkbox, its head (the link that opens it) over its caption, and its cells. */
function Record<K extends string>({
  row,
  columns,
  picks,
}: {
  row: RecordRow<K>;
  columns: readonly RecordColumn<K>[];
  picks: boolean;
}) {
  const s = table();
  const { lang } = useReader(undefined);
  const { t } = stringsIn(lang);
  const { picked, onPick, onOpen, current } = use(RowsContext);
  const marked = row.id === current ? { "aria-current": "page" as const } : {};
  const words = <OpenPlace>{breakable(row.head)}</OpenPlace>;
  return (
    <RecordLine role="row" className={s.line()}>
      {picks && (
        <RecordPlace at="pick" role="cell" className="">
          <Pick
            label={t("records.select_row", { name: row.head })}
            checked={picked?.has(row.id) === true}
            onChange={(checked) => {
              onPick(row.id, checked);
            }}
          />
        </RecordPlace>
      )}
      <RecordPlace at="head" role="rowheader" className="">
        <Stack as="span" gap="hair">
          {row.href !== undefined ? (
            <Link href={row.href} data-open="" {...marked} className={s.open()} dir="auto">
              {words}
            </Link>
          ) : onOpen !== undefined ? (
            <AriaButton
              data-open=""
              dir="auto"
              {...marked}
              onPress={() => {
                onOpen(row.id);
              }}
              className={`${s.open()} border-0 bg-transparent p-0 text-start text-body`}
            >
              {words}
            </AriaButton>
          ) : (
            <span className={s.plain()} dir="auto">
              {breakable(row.head)}
            </span>
          )}
          {row.caption !== undefined && (
            <span className={s.caption()} dir="auto">
              {row.caption}
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
            at={column.beside === true ? "beside" : figures ? "figures" : "fact"}
            role="cell"
            className=""
          >
            {column.beside !== true && (
              <NarrowOnly>
                <span className={s.factName()}>{column.label}</span>
              </NarrowOnly>
            )}
            <span className={figures ? `${s.fact()} ${s.figure()}` : s.fact()}>{value}</span>
          </RecordPlace>
        );
      })}
    </RecordLine>
  );
}

// The pages of a list.

/** Where a page leads: an address, or what the screen does to show it. */
export type PageTarget = { href: string } | { onPress: () => void };

export type RecordPagesProps = {
  /** The page before, if any (the first page has none). */
  previous?: PageTarget | undefined;
  /** The page after, if any (the last page has none). */
  next?: PageTarget | undefined;
  /** The page asked for is on its way: its button busy. */
  busy?: "previous" | "next" | undefined;
};

function PageButton({
  target,
  busy,
  icon,
  end,
  children,
}: {
  target: PageTarget;
  busy: boolean;
  icon: IconName;
  end: boolean;
  children: string;
}) {
  const label = (
    <Row as="span" inline align="center" gap="entry-caption">
      {!end && <Icon name={icon} place="caption" />}
      <span>{children}</span>
      {end && <Icon name={icon} place="caption" />}
    </Row>
  );
  if ("href" in target)
    return (
      <LinkButton href={target.href} size="target">
        {label}
      </LinkButton>
    );
  return (
    <Button size="target" busy={busy} onPress={target.onPress}>
      {label}
    </Button>
  );
}

/**
 * The pages of a list paged by cursor, under it: Previous at the start, Next at the end, each only where there is such
 * a page, so that the reader never meets a button that does nothing. A navigation named for assistive technology.
 */
export function RecordPages({ previous, next, busy }: RecordPagesProps) {
  const { lang } = useReader(undefined);
  const { t } = stringsIn(lang);
  if (previous === undefined && next === undefined) return null;
  return (
    <Row as="nav" align="center" gap="screen-tabs-gap" aria-label={t("records.pages")}>
      {previous !== undefined && (
        <PageButton target={previous} busy={busy === "previous"} icon="previous" end={false}>
          {t("records.previous")}
        </PageButton>
      )}
      {next !== undefined && (
        <Box push="end">
          <PageButton target={next} busy={busy === "next"} icon="onward" end>
            {t("records.next")}
          </PageButton>
        </Box>
      )}
    </Row>
  );
}

/**
 * Show more, under a list that grows in place: busy while the next records are on their way. The screen moves the
 * focus to the first record added once they have come, so that the reader goes on from there.
 */
export function MoreRecords({ onPress, busy = false }: { onPress: () => void; busy?: boolean | undefined }) {
  const { lang } = useReader(undefined);
  const { t } = stringsIn(lang);
  return (
    <Row justify="center">
      <Button size="target" busy={busy} onPress={onPress}>
        {t("records.more")}
      </Button>
    </Row>
  );
}

// The filters over a list.

const facet = tv({
  base: [
    "box-border h-chip cursor-pointer rounded-control border-0 px-chip-inline font-sans text-caption whitespace-nowrap",
    "outline-hidden data-focus-visible:shadow-focus",
  ],
  variants: {
    // Applied, it is knocked out of the text's color, with no hue, as a chip that is on.
    applied: {
      false: "bg-surface-raised text-text data-hovered:bg-surface-raised-hover data-pressed:bg-surface-raised-pressed",
      true: "bg-text text-surface",
    },
  },
});

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
 * A facet of the filters: a button that opens its values in a menu, the one applied checked; while a value is applied,
 * the button is knocked out and says it ("State: Open").
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
        <Row
          inline
          align="center"
          gap="chip-gap"
          as={<AriaButton className={facet({ applied: value !== undefined })} />}
        >
          <span>{value === undefined ? label : t("records.facet", { facet: label, value: value.label })}</span>
          <Icon name="expand" place="caption" />
        </Row>
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

/** A facet that is on or off ("Assigned to me"): a toggle, knocked out while on. */
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
  search?: { label: string; value: string; onChange: (value: string) => void; clearLabel: string } | undefined;
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
  return (
    <Stack role="search" gap="screen-field-gap" aria-label={label}>
      {search !== undefined && (
        <SearchField
          id={id}
          label={search.label}
          labelHidden
          value={search.value}
          onChange={search.onChange}
          clearLabel={search.clearLabel}
        />
      )}
      <Row wrap align="center" gap="chip-spacing">
        {children}
        {applied && (
          <Box as="span" push="end" text="caption">
            <TextButton onPress={onClear}>{t("records.clear_filters")}</TextButton>
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
