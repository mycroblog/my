// SPDX-License-Identifier: AGPL-3.0-only

// The finance views (ADR-0067, FR-902, FR-905, FR-916, FR-917): a table of figures that a screen reader reads cell by
// cell, each cell with its row's head and its column's name. One part serves every view: the totals per period (a
// week, a month or a year a row; costs, income and the balance its columns), and a period's breakdown (a provider,
// a category and a currency a row, a provider named once over its categories). Totals are per currency and never
// converted (ADR-0067 §6): a cell holds a figure a currency, one under another. A figure from an estimate carries the
// estimate's glyph on its own line, read as the app's word for it, which a line under the table says once beside the
// glyph; the table's rules (how its periods are cut, how a cost longer than a month counts) stand there too, from the
// screen. Where a bar draws a comparison the figures do not show at a glance (a period's costs against its income, in
// one currency), a bar under each figure draws its share of the table's largest: a chart that adds nothing the figures
// do not say, hidden from assistive technology. The app gives `share` only there.
//
// The app formats every amount (`Intl.NumberFormat` in its currency) and every period (`Intl.DateTimeFormat`,
// `formatRange`), and gives every word: these parts lay them out. A provider's name is the operator's (`dir="auto"`).
// A choice of the period's length, or of how a cost longer than a month is counted, is a `SegmentChoice` (choice.tsx).

import { createContext, use, useId, type HTMLAttributes, type ReactNode } from "react";
import { tv } from "tailwind-variants/lite";
import { ShareBar } from "./choice";
import { CodeText } from "./code-text";
import { Icon } from "./icons";
import { Row, Spoken, Stack } from "./layout";

/** A column of a table: its name, and whether it holds figures, which stand at its end. */
export type FinanceColumn = { label: string; figures?: boolean | undefined };

/** A figure, as the app formats it in its currency; marked where it comes from an estimate. */
export type FinanceFigure = string | { shown: string; estimated?: boolean | undefined };

/**
 * A cell of figures: an amount a currency; `estimated`, all of them come from an estimate; their bar's share (0 to
 * 1) of the table's largest figure, where a bar compares what the figures do not (costs against income, one currency);
 * a cell of more than one figure draws none.
 */
export type FinanceFigures = {
  figures: readonly FinanceFigure[];
  estimated?: boolean | undefined;
  share?: number | undefined;
};

/** A cell: words (a category), or figures. */
export type FinanceCell = string | FinanceFigures;

/**
 * The tracks of the table, by how many columns follow the rows' heads, which know the table's own width. Narrow (a
 * phone): those columns alike, the head of a row on a line of its own over its cells. Wide (a tablet's column and
 * wider): the heads down the start taking the room left, then the columns alike, each as wide as a few figures or as
 * its widest, so that the bars of every column compare true.
 */
const TRACKS = {
  1: "grid-cols-[minmax(0,1fr)] @min-[16rem]:grid-cols-[minmax(0,1fr)_minmax(max-content,7.5rem)]",
  2: "grid-cols-[repeat(2,minmax(0,1fr))] @min-[20rem]:grid-cols-[minmax(0,1fr)_repeat(2,minmax(max-content,6.5rem))]",
  3: "grid-cols-[repeat(3,minmax(0,1fr))] @min-[30rem]:grid-cols-[minmax(max-content,1fr)_repeat(3,minmax(max-content,7rem))]",
  4: "grid-cols-[repeat(4,minmax(0,1fr))] @min-[34rem]:grid-cols-[minmax(max-content,1fr)_repeat(4,minmax(max-content,6.5rem))]",
} as const;

type Count = keyof typeof TRACKS;

/**
 * Where a table of so many columns sets its rows side by side, its heads down the start: as soon as its columns fit
 * beside a head (a phone holds one or two), as a class each, written out for Tailwind to find.
 */
const WIDE = {
  head: {
    1: "col-span-full min-w-0 @min-[16rem]:col-span-1",
    2: "col-span-full min-w-0 @min-[20rem]:col-span-1",
    3: "col-span-full min-w-0 @min-[30rem]:col-span-1",
    4: "col-span-full min-w-0 @min-[34rem]:col-span-1",
  },
  line: {
    1: "@min-[16rem]:gap-y-0",
    2: "@min-[20rem]:gap-y-0",
    3: "@min-[30rem]:gap-y-0",
    4: "@min-[34rem]:gap-y-0",
  },
} as const satisfies Record<string, Record<Count, string>>;

const countOf = (columns: number): Count => Math.min(4, Math.max(1, columns)) as Count;

/**
 * The table's frame, which knows its own width: its rows one under another, on its tracks, within its container, the
 * surface of its total too, what the rows hold set in by a row's inset.
 */
function FinanceFrame({
  columns,
  className,
  ...rest
}: HTMLAttributes<HTMLDivElement> & { columns: number; className: string }) {
  const tracks = TRACKS[countOf(columns)];
  return (
    <div className="@container">
      <div {...rest} className={`${className} grid gap-x-entry-avatar ${tracks}`} />
    </div>
  );
}

/**
 * A row of the table, on the table's tracks, a little air above and below it. `head`, the row of the columns' names;
 * `continued`, a row of the same head as the one above (a provider's next category), close under it.
 */
function FinanceLine({
  head = false,
  continued = false,
  className,
  ...rest
}: HTMLAttributes<HTMLDivElement> & { head?: boolean; continued?: boolean; className: string }) {
  const wide = WIDE.line[countOf(use(TableContext).columns.length)];
  const shown = head
    ? "pb-screen-air"
    : continued
      ? `pb-screen-row-block ${wide}`
      : `gap-y-entry-line py-screen-row-block ${wide}`;
  return <div {...rest} className={`${className} col-span-full grid grid-cols-subgrid items-start ${shown}`} />;
}

const PLACE = {
  // The head of a row, and the name of the heads' column: narrow, a line of its own; wide, the first column (WIDE).
  head: "",
  corner: "",
  // A cell of figures, at its column's end.
  figures: "min-w-0 justify-self-stretch text-end",
  // A cell of words, from its column's start.
  words: "min-w-0 text-start",
} as const;

/** Where a cell stands in its row. */
function FinancePlace({
  at,
  className,
  ...rest
}: { at: keyof typeof PLACE; className: string } & HTMLAttributes<HTMLElement>) {
  const wide = at === "head" || at === "corner" ? WIDE.head[countOf(use(TableContext).columns.length)] : "";
  return <div {...rest} className={`${className} ${PLACE[at]} ${wide}`} />;
}

const table = tv({
  slots: {
    root: "text-body text-text",
    column: "text-caption text-muted-text [word-break:auto-phrase]",
    head: "[word-break:auto-phrase]",
    caption: "text-caption text-muted-text [word-break:auto-phrase]",
    figure: "tabular-nums whitespace-nowrap",
    glyph: "text-muted-text",
    note: "text-caption text-muted-text [word-break:auto-phrase]",
    // The row that sums the others: on the raised surface, and on the page's where it stands on a raised one.
    total: [
      "rounded-control bg-surface-raised px-screen-row-inline font-strong",
      "[[data-surface-raised]_&]:bg-surface",
    ],
    line: "px-screen-row-inline",
  },
});

/** What the table gives its rows: its columns, and the word that marks an estimate. */
const TableContext = createContext<{ readonly columns: readonly FinanceColumn[]; readonly estimated: string }>({
  columns: [],
  estimated: "",
});

/** The glyph that marks a figure from an estimate. */
function EstimateMark() {
  const s = table();
  return (
    <Row as="span" inline rigid className={s.glyph()}>
      <Icon name="estimate" place="caption" />
    </Row>
  );
}

/** Whether a row is the one that sums the others. */
const TotalContext = createContext(false);

export type FinanceTableProps = {
  /** What the table is, for assistive technology ("Totals by month"). */
  label: string;
  /** The name of the rows' heads' column ("Month", "Provider"), over the columns' names on a narrow table. */
  head: string;
  /** The columns after the heads, one to four: "Costs", "Income", "Balance"; "Category", "Amount". */
  columns: readonly FinanceColumn[];
  /**
   * The word read with a figure from an estimate ("Estimate"), said once under the table beside the estimate's glyph:
   * give it where the table holds an estimate.
   */
  estimated?: string | undefined;
  /**
   * The table's rules, short, under it and describing it: how its periods are cut (dates with no time zone, the week
   * from Monday, each provider's billing period), how a cost longer than a month counts.
   */
  about?: string | undefined;
  /** The `FinanceRow`s, in the order of their periods, the latest first, or of the breakdown. */
  children: ReactNode;
  /** The row that sums the others (a `FinanceRow`), last, on the raised surface. */
  total?: ReactNode;
};

/**
 * A table of figures, named `label`: a row a period (or a line of a breakdown), its head the row's header; a column
 * a kind of figure. For assistive technology it is a table whose every cell is read with its row's head and its
 * column's name ("November 2026, Costs, ¥12,300, US$45.67, Estimate"); its first row the columns' names.
 */
export function FinanceTable({ label, head, columns, estimated, about, children, total }: FinanceTableProps) {
  const s = table();
  const aboutId = useId();
  return (
    <Stack gap="entry-line">
      <FinanceFrame
        role="table"
        aria-label={label}
        {...(about === undefined ? {} : { "aria-describedby": aboutId })}
        columns={columns.length}
        className={s.root()}
      >
        <TableContext value={{ columns, estimated: estimated ?? "" }}>
          <FinanceLine role="row" head className={s.line()}>
            <FinancePlace at="corner" role="columnheader" className={s.column()}>
              {head}
            </FinancePlace>
            {columns.map((column) => (
              <FinancePlace
                key={column.label}
                at={column.figures === true ? "figures" : "words"}
                role="columnheader"
                className={s.column()}
              >
                {column.label}
              </FinancePlace>
            ))}
          </FinanceLine>
          {children}
          {total !== undefined && <TotalContext value>{total}</TotalContext>}
        </TableContext>
      </FinanceFrame>
      {(estimated !== undefined || about !== undefined) && (
        <Stack gap="hair">
          {estimated !== undefined && (
            // What the glyph means, for the eye: a screen reader hears the word with each figure.
            <Row align="center" gap="hair" className={s.note()} aria-hidden="true">
              <EstimateMark />
              <span>{estimated}</span>
            </Row>
          )}
          {about !== undefined && (
            <p id={aboutId} className={s.note()}>
              {about}
            </p>
          )}
        </Stack>
      )}
    </Stack>
  );
}

export type FinanceRowProps = {
  /** The row's head: its period, as the app formats it ("November 2026", "2026-W47"), or a provider's name. */
  head: string;
  /** A line under the head, muted: the period's dates ("Nov 16 – 22"), "So far" for the period of today. */
  caption?: string | undefined;
  /** Its cells, in the columns' order. */
  cells: readonly FinanceCell[];
  /**
   * It has the head of the row above (a provider's next category): the head is said to assistive technology alone,
   * and the row stands close under that one.
   */
  continued?: boolean | undefined;
  /** The head is code (a route, an action's ID): in the code's face, left to right. */
  code?: boolean | undefined;
};

/**
 * A row of a `FinanceTable`: its head, with a line under it; then its cells, a figure a currency, one under another,
 * a figure from an estimate with the estimate's glyph before it on its line, and their bar right under them.
 */
export function FinanceRow({ head, caption, cells, continued = false, code = false }: FinanceRowProps) {
  const s = table();
  const { columns, estimated } = use(TableContext);
  const total = use(TotalContext);
  return (
    <FinanceLine role="row" continued={continued} className={total ? s.total() : s.line()}>
      <FinancePlace at="head" role="rowheader" className={s.head()}>
        {continued ? (
          <Spoken>{head}</Spoken>
        ) : (
          // Its line on the head's own line where it fits, never beside a figure of the row's next currency.
          <Row as="span" wrap align="baseline" gapX="entry-caption">
            {code ? <CodeText>{head}</CodeText> : <span dir="auto">{head}</span>}
            {caption !== undefined && <span className={s.caption()}>{caption}</span>}
          </Row>
        )}
      </FinancePlace>
      {cells.map((cell, i) => {
        const figures = columns[i]?.figures === true;
        return (
          <FinancePlace key={columns[i]?.label ?? i} at={figures ? "figures" : "words"} role="cell" className="">
            {typeof cell === "string" ? (
              <span dir="auto">{cell}</span>
            ) : (
              <Stack as="span" gap="hair">
                {cell.figures.map((figure) => {
                  const shown = typeof figure === "string" ? figure : figure.shown;
                  const marked = cell.estimated === true || (typeof figure !== "string" && figure.estimated === true);
                  return (
                    <Row key={shown} as="span" justify="end" align="center" gap="hair">
                      {marked && <EstimateMark />}
                      <span className={s.figure()}>{shown}</span>
                      {marked && <Spoken>{` ${estimated}`}</Spoken>}
                    </Row>
                  );
                })}
                {/* Amounts in two currencies do not compare: their cell draws no bar, whatever its share. */}
                {cell.share !== undefined && cell.figures.length === 1 && (
                  <ShareBar share={Math.min(1, Math.max(0, cell.share))} />
                )}
              </Stack>
            )}
          </FinancePlace>
        );
      })}
    </FinanceLine>
  );
}
