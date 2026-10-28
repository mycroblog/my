// SPDX-License-Identifier: AGPL-3.0-only
// A table to read: rows of values under column headers, each row named by its row header. It is the native `table`,
// which assistive technology already reads by row and column; a table people select or sort in is React Aria's Table,
// which the kit does not wrap yet (ADR-0031). The page never scrolls sideways: values wrap where `wrap="anywhere"` allows,
// and a table of more columns than the width holds scrolls inside its own focusable group (a data table is exempt from
// WCAG 1.4.10, and stacking its cells would leave them without their column headers).
import { type ComponentPropsWithRef, createContext, type ReactNode, use, useId } from "react";
import { tv } from "tailwind-variants";

const table = tv({
  slots: {
    // relative: the visually hidden parts of the table are absolute and would else escape the scroller and widen the page.
    scroller: "relative overflow-x-auto outline-none focus-visible:focus-ring",
    root: "w-full",
    caption: "sr-only",
    headRow: "border-b",
    body: "",
    row: "",
    columnHeader: "px-3 py-2 text-start text-xs font-extrabold text-fg-muted wrap-anywhere",
    rowHeader: "min-w-48 p-3 text-start font-normal",
    cell: "p-3",
  },
  variants: {
    density: {
      // Totals of a few columns at body size, a rule under every row.
      regular: {
        root: "text-start",
        headRow: "border-border",
        row: "border-b border-border align-top last:border-b-0",
      },
      // Many short values at `text-sm` in fixed columns, rules between the rows.
      compact: {
        root: "table-fixed border-collapse text-sm",
        headRow: "border-border-strong",
        body: "divide-y divide-border",
        rowHeader: "align-top",
        cell: "align-top",
      },
    },
    width: { narrow: { columnHeader: "w-16" }, wide: { columnHeader: "w-2/5" } },
    numeric: { true: { cell: "tabular-nums" } },
    wrap: { anywhere: { cell: "wrap-anywhere", rowHeader: "wrap-anywhere" } },
    align: { matchParent: { cell: "align-match-parent", rowHeader: "align-match-parent" } },
  },
  defaultVariants: { density: "regular" },
});

type Density = "regular" | "compact";

const DensityContext = createContext<Density>("regular");

export type TableProps = Omit<ComponentPropsWithRef<"table">, "className" | "style"> & {
  /** `TableHead` and `TableBody`. */
  children: ReactNode;
  /**
   * What the table holds. It names the table and is visually hidden, as the heading or the controls around a table
   * already say it; leave it out when `aria-labelledby` names the table by a visible heading.
   */
  caption?: ReactNode;
  /** `regular` (totals at body size, a rule under every row) or `compact` (`text-sm` in fixed columns, rules between rows). */
  density?: Density;
};

/** A table to read. Its rows are `TableRow`s of a `TableRowHeader` and `TableCell`s, under one `TableHead` row. */
export function Table({ children, caption, density = "regular", ...props }: TableProps) {
  const captionId = useId();
  const styles = table({ density });
  const labelledBy = caption === undefined ? props["aria-labelledby"] : captionId;
  return (
    <DensityContext value={density}>
      <div role="group" aria-labelledby={labelledBy} tabIndex={0} className={styles.scroller()}>
        <table aria-labelledby={labelledBy} {...props} className={styles.root()}>
          {caption !== undefined && (
            <caption id={captionId} className={styles.caption()}>
              {caption}
            </caption>
          )}
          {children}
        </table>
      </div>
    </DensityContext>
  );
}

export type TableHeadProps = {
  /** The `TableColumnHeader`s of the one header row. */
  children: ReactNode;
};

/** The header row (`thead`), with a rule under it. */
export function TableHead({ children }: TableHeadProps) {
  const styles = table({ density: use(DensityContext) });
  return (
    <thead>
      <tr className={styles.headRow()}>{children}</tr>
    </thead>
  );
}

export type TableBodyProps = {
  /** The `TableRow`s. */
  children: ReactNode;
};

/** The rows of values (`tbody`). */
export function TableBody({ children }: TableBodyProps) {
  const styles = table({ density: use(DensityContext) });
  return <tbody className={styles.body()}>{children}</tbody>;
}

export type TableRowProps = Omit<ComponentPropsWithRef<"tr">, "className" | "style">;

/** A row of values, its row header first. */
export function TableRow(props: TableRowProps) {
  const styles = table({ density: use(DensityContext) });
  return <tr {...props} className={styles.row()} />;
}

export type TableColumnHeaderProps = Omit<ComponentPropsWithRef<"th">, "className" | "style" | "scope"> & {
  /** A fixed share of a `compact` table: `narrow` (`w-16`, a line number) or `wide` (`w-2/5`, a sentence). */
  width?: "narrow" | "wide";
};

/** The header of a column (`th scope="col"`). */
export function TableColumnHeader({ width, ...props }: TableColumnHeaderProps) {
  const styles = table({ density: use(DensityContext), width });
  return <th scope="col" {...props} className={styles.columnHeader()} />;
}

type CellLook = {
  /** Wraps a long word anywhere (people's and servers' words, amounts in many currencies). */
  wrap?: "anywhere";
  /** Lines a `dir="auto"` value up with the table's direction (G9). */
  align?: "matchParent";
};

export type TableRowHeaderProps = Omit<ComponentPropsWithRef<"th">, "className" | "style" | "scope" | "align"> &
  CellLook;

/** The header of a row (`th scope="row"`): the name of what the row is about, at body weight. */
export function TableRowHeader({ wrap, align, ...props }: TableRowHeaderProps) {
  const styles = table({ density: use(DensityContext), wrap, align });
  return <th scope="row" {...props} className={styles.rowHeader()} />;
}

export type TableCellProps = Omit<ComponentPropsWithRef<"td">, "className" | "style" | "align"> &
  CellLook & {
    /** Digits of equal width, for counts and amounts (G8). */
    numeric?: boolean;
  };

/** A value (`td`). */
export function TableCell({ numeric, wrap, align, ...props }: TableCellProps) {
  const styles = table({ density: use(DensityContext), numeric, wrap, align });
  return <td {...props} className={styles.cell()} />;
}
