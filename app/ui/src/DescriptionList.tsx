// SPDX-License-Identifier: AGPL-3.0-only
// The facts of a record (a server's state, a job's keys, an account's history): a description list whose
// terms sit above their values and whose items flow into columns as the container widens (web-client/README.md D2).
import type { ReactNode } from "react";
import { tv } from "tailwind-variants";

const descriptionList = tv({
  slots: {
    root: "@container min-w-0",
    list: "grid grid-cols-1 gap-x-6 gap-y-3",
    item: "flex min-w-0 flex-col",
    term: "text-sm font-bold text-fg-muted",
    value: "min-w-0 wrap-anywhere",
  },
  variants: {
    /** The most columns, reached in a wide container; a narrow one always has one column. */
    columns: {
      "1": {},
      "2": { list: "@sm:grid-cols-2" },
      "3": { list: "@sm:grid-cols-2 @2xl:grid-cols-3" },
      "4": { list: "@sm:grid-cols-2 @2xl:grid-cols-4" },
    },
  },
  defaultVariants: { columns: "1" },
});

export type DescriptionListProps = {
  /** `DescriptionItem`s. */
  children: ReactNode;
  /** The most columns (1 to 4). Long values (an error, a payload) belong in a list of one column. */
  columns?: 1 | 2 | 3 | 4;
  /** Classes merged onto the root with tailwind-variants. */
  className?: string;
};

/** A `<dl>` that is its own size container, so its columns answer to the space it has. */
export function DescriptionList({ children, columns, className }: DescriptionListProps) {
  const styles = descriptionList({ columns: columns && `${columns}` });
  return (
    <div className={styles.root({ className })}>
      <dl className={styles.list()}>{children}</dl>
    </div>
  );
}

export type DescriptionItemProps = {
  /** The name of the fact ("State"). */
  term: ReactNode;
  /** Its value. Values written by people or servers wrap anywhere, so a long one never scrolls the page. */
  children: ReactNode;
};

/** One term and its value. */
export function DescriptionItem({ term, children }: DescriptionItemProps) {
  const styles = descriptionList();
  return (
    <div className={styles.item()}>
      <dt className={styles.term()}>{term}</dt>
      <dd className={styles.value()}>{children}</dd>
    </div>
  );
}
