// SPDX-License-Identifier: AGPL-3.0-only
// The head of a region or a row: its heading and the actions on it. In a narrow container the actions go under the
// heading; from @md they sit at the inline end of the same line (web-client/README.md D2).
import type { ReactNode } from "react";
import { tv } from "tailwind-variants";

const sectionHeader = tv({
  slots: {
    root: "flex flex-col items-start gap-2 @md:flex-row @md:items-center @md:justify-between",
    actions: "flex flex-wrap gap-2",
  },
});

export type SectionHeaderProps = {
  /** The heading: a `Heading`. */
  children: ReactNode;
  /** The actions on the region: `Button`s or `Link`s. Nothing (`undefined`, `null`, `false`) leaves the heading alone. */
  actions?: ReactNode;
};

/** A heading and its actions on one line from @md, stacked below it (an ancestor needs `container`). */
export function SectionHeader({ children, actions }: SectionHeaderProps) {
  const styles = sectionHeader();
  return (
    <div className={styles.root()}>
      {children}
      {actions !== undefined && actions !== null && actions !== false && (
        <div className={styles.actions()}>{actions}</div>
      )}
    </div>
  );
}
