// SPDX-License-Identifier: AGPL-3.0-only
// The table of a list of records (the admin and moderation lists, web-client §14): one row per record under column
// headers, drawn from column definitions, so that the same definitions draw a second form for a narrow container.
// Below `listTableNarrowWidth` the rows become a list of cards: the row header is the card's heading and every other
// value is a `DescriptionItem` whose term is its column header, so that no value loses its header (the reason `Table`
// itself never stacks its cells). The actions of a row go below its facts, without a term.
//
// The form answers to the width of the table's own container, as a container query would (web-client/README.md D2),
// but is chosen in script: the two forms are different elements (a `table` and a `ul`), and drawing both and hiding
// one with CSS would put every control of a row twice in the page. The width is measured before the first paint
// (useLayoutEffect), so the wrong form never shows. Without a ResizeObserver (jsdom) the table form stays.
import { type ReactNode, type Ref, type RefObject, useLayoutEffect, useRef, useState } from "react";
import { mergeRefs } from "react-aria";
import { tv } from "tailwind-variants";
import { DescriptionItem, DescriptionList } from "./DescriptionList";
import { Table, TableBody, TableCell, TableColumnHeader, TableHead, TableRow, TableRowHeader } from "./Table";
import { VisuallyHidden } from "./VisuallyHidden";

/** Below this inline size of its container, in rem, a `ListTable` draws its rows as cards. */
export const listTableNarrowWidth = 40;

const listTable = tv({
  slots: {
    root: "min-w-0",
    cards: "flex flex-col divide-y divide-border",
    card: "@container flex min-w-0 flex-col gap-3 p-4",
    // The look of Heading's `subsection`; outline-none: it takes focus from script only (tabIndex={-1}).
    heading: "min-w-0 font-bold wrap-anywhere outline-none",
    actions: "flex flex-wrap items-center gap-2",
  },
});

/** One column of a `ListTable`: its header and how each record fills it. */
export type ListTableColumn<T> = {
  /** The column's key among the columns. */
  id: string;
  /** The column header; in the card form, the term of the value. An `actions` column's header is visually hidden. */
  header: string;
  /** The value of `item` in this column. A cell that calls hooks renders a component of its own. */
  cell: (item: T) => ReactNode;
  /**
   * `value` (the default): a fact of the record. `actions`: the row's buttons, the last column, whose header
   * ("Actions") is only read; in the card form they go below the facts.
   */
  kind?: "value" | "actions";
  /** Digits of equal width, for counts and amounts (G8). */
  numeric?: boolean;
  /** Wraps a long word anywhere (people's and servers' words, domains, versions). */
  wrap?: "anywhere";
  /** Lines a `dir="auto"` value up with the table's direction (G9). */
  align?: "matchParent";
};

/** The columns of a `ListTable`: the first names the record (its row header and its card's heading). */
export type ListTableColumns<T> = readonly [ListTableColumn<T>, ...ListTableColumn<T>[]];

export type ListTableProps<T> = {
  /** The records shown, one row each. */
  items: readonly T[];
  /** The columns, the record's name first; at most four (the name, a state, a date and the actions, web-client §14). */
  columns: ListTableColumns<T>;
  /** A stable id of a record: the row's key and its `data-row-id` (app/web's lib/useFocusAfterRemoval.ts). */
  getId: (item: T) => string;
  /** What the list holds. It names the table (a visually hidden caption) and the list of cards. */
  caption: string;
  /** The level of a card's heading: one below the heading of the region the list is in. */
  headingLevel: 2 | 3 | 4 | 5 | 6;
  /**
   * The element around either form. Rows carry `data-row-id`, and the element that names a row (the `th` or the
   * card's heading) carries `data-row-header` and `tabIndex={-1}`, so that focus can move to it from script.
   */
  ref?: Ref<HTMLDivElement>;
};

/** Whether the element is narrower than `listTableNarrowWidth` rem, measured before paint and on every resize. */
function useNarrow(ref: RefObject<HTMLElement | null>): boolean {
  const [narrow, setNarrow] = useState(false);
  useLayoutEffect(() => {
    const element = ref.current;
    if (element === null || typeof ResizeObserver === "undefined") return;
    const update = () => {
      const rem = Number.parseFloat(getComputedStyle(element.ownerDocument.documentElement).fontSize) || 16;
      setNarrow(element.clientWidth < listTableNarrowWidth * rem);
    };
    update();
    // The width follows the container; a change of the root font size (zoom, text size) reaches it as a resize too.
    const observer = new ResizeObserver(update);
    observer.observe(element);
    return () => {
      observer.disconnect();
    };
  }, [ref]);
  return narrow;
}

/**
 * The rows of a list as a table, or as cards in a narrow container. Every column is a `ListTableColumn`; the first is
 * the row header. Put it on the surface of the list (a `Card` with `padding="0"`).
 */
export function ListTable<T>({ items, columns, getId, caption, headingLevel, ref }: ListTableProps<T>) {
  const rootRef = useRef<HTMLDivElement>(null);
  const narrow = useNarrow(rootRef);
  const styles = listTable();
  const [name, ...rest] = columns;
  const facts = rest.filter((column) => column.kind !== "actions");
  const actions = rest.filter((column) => column.kind === "actions");
  return (
    <div ref={mergeRefs(rootRef, ref)} className={styles.root()} data-list-table={narrow ? "cards" : "table"}>
      {narrow ? (
        <ul aria-label={caption} className={styles.cards()}>
          {items.map((item) => {
            const id = getId(item);
            return (
              <li key={id} data-row-id={id} className={styles.card()}>
                {/* A `div` with the heading role rather than an `h2`–`h6`: a row header may hold blocks (a name over a
                    line of text), which a heading element may not. */}
                <div
                  role="heading"
                  aria-level={headingLevel}
                  tabIndex={-1}
                  data-row-header=""
                  className={styles.heading()}
                >
                  {name.cell(item)}
                </div>
                {facts.length > 0 && (
                  <DescriptionList columns={2}>
                    {facts.map((column) => (
                      <DescriptionItem key={column.id} term={column.header}>
                        {column.cell(item)}
                      </DescriptionItem>
                    ))}
                  </DescriptionList>
                )}
                {actions.map((column) => (
                  <div key={column.id} className={styles.actions()}>
                    {column.cell(item)}
                  </div>
                ))}
              </li>
            );
          })}
        </ul>
      ) : (
        <Table caption={caption}>
          <TableHead>
            {columns.map((column) => (
              <TableColumnHeader key={column.id}>
                {column.kind === "actions" ? <VisuallyHidden>{column.header}</VisuallyHidden> : column.header}
              </TableColumnHeader>
            ))}
          </TableHead>
          <TableBody>
            {items.map((item) => {
              const id = getId(item);
              return (
                <TableRow key={id} data-row-id={id}>
                  <TableRowHeader
                    tabIndex={-1}
                    data-row-header=""
                    {...(name.wrap !== undefined && { wrap: name.wrap })}
                    {...(name.align !== undefined && { align: name.align })}
                  >
                    {name.cell(item)}
                  </TableRowHeader>
                  {rest.map((column) => (
                    <TableCell
                      key={column.id}
                      {...(column.numeric !== undefined && { numeric: column.numeric })}
                      {...(column.wrap !== undefined && { wrap: column.wrap })}
                      {...(column.align !== undefined && { align: column.align })}
                    >
                      {column.cell(item)}
                    </TableCell>
                  ))}
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      )}
    </div>
  );
}
