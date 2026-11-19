// SPDX-License-Identifier: AGPL-3.0-only

// The finance views (ADR-0067, FR-902, FR-905, FR-916, FR-917): a table of figures that a screen reader reads cell by
// cell, each cell with its row's head and its column's name. One part serves every view: the totals per period (a
// week, a month or a year a row; costs, income and the balance its columns), and a period's breakdown (a provider,
// a category and a currency a row). Totals are per currency and never converted (ADR-0067 §6): a cell holds a figure a
// currency, one under another. A figure from an estimate is marked by the app's word for it, read with it. Where the
// figures of a table share one currency, a bar under each draws its share of the largest: a chart that adds nothing
// the figures do not say, hidden from assistive technology.
//
// The app formats every amount (`Intl.NumberFormat` in its currency) and every period (`Intl.DateTimeFormat`,
// `formatRange`), and gives every word: these parts lay them out. A provider's name is the operator's (`dir="auto"`).
// A choice of the period's length, or of how a cost longer than a month is counted, is a `SegmentChoice` (choice.tsx).

import { createContext, use, type HTMLAttributes, type ReactNode } from "react";
import { tv } from "tailwind-variants/lite";
import { ShareBar } from "./choice";
import { Stack } from "./layout";

/** A column of a table: its name, and whether it holds figures, which stand at its end. */
export type FinanceColumn = { label: string; figures?: boolean | undefined };

/** A cell of figures: an amount a currency, as the app formats them; whether they come from an estimate; their bar. */
export type FinanceFigures = {
  figures: readonly string[];
  estimated?: boolean | undefined;
  /** The share (0 to 1) of the table's largest figure, where the table's figures share one currency. */
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
  1: "grid-cols-[minmax(0,1fr)] @min-[30rem]:grid-cols-[minmax(0,1fr)_minmax(max-content,7.5rem)]",
  2: "grid-cols-[repeat(2,minmax(0,1fr))] @min-[30rem]:grid-cols-[minmax(0,1fr)_repeat(2,minmax(max-content,7.5rem))]",
  3: "grid-cols-[repeat(3,minmax(0,1fr))] @min-[30rem]:grid-cols-[minmax(0,1fr)_repeat(3,minmax(max-content,7.5rem))]",
  4: "grid-cols-[repeat(4,minmax(0,1fr))] @min-[30rem]:grid-cols-[minmax(0,1fr)_repeat(4,minmax(max-content,7.5rem))]",
} as const;

/**
 * The table's frame, which knows its own width: its rows one under another, on its tracks, reaching past its
 * container by a row's inset, so that what the rows hold lines up with what stands above and below the table, and the
 * surface of its total reaches past it.
 */
function FinanceFrame({
  columns,
  className,
  ...rest
}: HTMLAttributes<HTMLDivElement> & { columns: number; className: string }) {
  const tracks = TRACKS[Math.min(4, Math.max(1, columns)) as keyof typeof TRACKS];
  return (
    <div className="@container">
      <div {...rest} className={`${className} -mx-screen-row-inline grid gap-x-entry-avatar ${tracks}`} />
    </div>
  );
}

/**
 * A row of the table, on the table's tracks, a little air above and below it. `head`, the row of the columns' names.
 * `total`, the row that sums the others, on the raised surface.
 */
function FinanceLine({
  head = false,
  className,
  ...rest
}: HTMLAttributes<HTMLDivElement> & { head?: boolean; className: string }) {
  const shown = head ? "pb-screen-air" : "gap-y-entry-line py-screen-row-block @min-[30rem]:gap-y-0";
  return <div {...rest} className={`${className} col-span-full grid grid-cols-subgrid items-start ${shown}`} />;
}

const PLACE = {
  // The head of a row: narrow, a line of its own; wide, the first column.
  head: "col-span-full min-w-0 @min-[30rem]:col-span-1",
  // The name of the heads' column, which a narrow table says to assistive technology alone.
  corner: "sr-only @min-[30rem]:not-sr-only",
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
  return <div {...rest} className={`${className} ${PLACE[at]}`} />;
}

const table = tv({
  slots: {
    root: "text-body text-text",
    column: "text-caption text-muted-text [word-break:auto-phrase]",
    head: "[word-break:auto-phrase]",
    caption: "text-caption text-muted-text [word-break:auto-phrase]",
    figure: "tabular-nums whitespace-nowrap",
    estimated: "text-caption text-muted-text",
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

/** Whether a row is the one that sums the others. */
const TotalContext = createContext(false);

export type FinanceTableProps = {
  /** What the table is, for assistive technology ("Totals by month"). */
  label: string;
  /** The name of the rows' heads' column ("Month", "Provider"), which a narrow table says to assistive technology alone. */
  head: string;
  /** The columns after the heads, one to four: "Costs", "Income", "Balance"; "Category", "Amount". */
  columns: readonly FinanceColumn[];
  /** The word read with a figure from an estimate ("Estimate"). */
  estimated: string;
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
export function FinanceTable({ label, head, columns, estimated, children, total }: FinanceTableProps) {
  const s = table();
  return (
    <FinanceFrame role="table" aria-label={label} columns={columns.length} className={s.root()}>
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
      <TableContext value={{ columns, estimated }}>
        {children}
        {total !== undefined && <TotalContext value>{total}</TotalContext>}
      </TableContext>
    </FinanceFrame>
  );
}

export type FinanceRowProps = {
  /** The row's head: its period, as the app formats it ("November 2026", "2026-W47"), or a provider's name. */
  head: string;
  /** A line under the head, muted: the period's dates ("Nov 16 – 22"), "So far" for the period of today. */
  caption?: string | undefined;
  /** Its cells, in the columns' order. */
  cells: readonly FinanceCell[];
};

/**
 * A row of a `FinanceTable`: its head, with a line under it; then its cells, a figure a currency, one under another,
 * their bar right under them, so that the bars of a row line up, and the word of an estimate last.
 */
export function FinanceRow({ head, caption, cells }: FinanceRowProps) {
  const s = table();
  const { columns, estimated } = use(TableContext);
  const total = use(TotalContext);
  return (
    <FinanceLine role="row" className={total ? s.total() : s.line()}>
      <FinancePlace at="head" role="rowheader" className={s.head()}>
        <Stack as="span" gap="hair">
          <span dir="auto">{head}</span>
          {caption !== undefined && <span className={s.caption()}>{caption}</span>}
        </Stack>
      </FinancePlace>
      {cells.map((cell, i) => {
        const figures = columns[i]?.figures === true;
        return (
          <FinancePlace key={columns[i]?.label ?? i} at={figures ? "figures" : "words"} role="cell" className="">
            {typeof cell === "string" ? (
              <span dir="auto">{cell}</span>
            ) : (
              <Stack as="span" gap="hair">
                {cell.figures.map((figure) => (
                  <span key={figure} className={s.figure()}>
                    {figure}
                  </span>
                ))}
                {cell.share !== undefined && <ShareBar share={Math.min(1, Math.max(0, cell.share))} />}
                {cell.estimated === true && <span className={s.estimated()}>{estimated}</span>}
              </Stack>
            )}
          </FinancePlace>
        );
      })}
    </FinanceLine>
  );
}
