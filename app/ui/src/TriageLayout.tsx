// SPDX-License-Identifier: AGPL-3.0-only
// TriageLayout (ADR-0110 P1): the page of one item of a queue (a report, an account under review), laid out so that
// the decision stays in reach while the evidence is read. In a wide container the summary and the evidence fill the
// first column and the decision stands at the top of a second column beside them; in a narrow one the decision follows
// the summary, before the evidence, so it is near the top at every width. It does not stick as the page scrolls: a
// decision taller than the screen would hide its own end. The document order is the narrow order (summary, decision,
// evidence), which the keyboard and screen readers follow at every width.
import type { ReactNode } from "react";
import { tv } from "tailwind-variants";

const triageLayout = tv({
  slots: {
    root: "@container min-w-0",
    grid: [
      "grid min-w-0 grid-cols-1 gap-8 [grid-template-areas:'summary'_'decision'_'evidence']",
      "@3xl:grid-cols-[minmax(0,1fr)_minmax(18rem,22rem)] @3xl:[grid-template-areas:'summary_decision'_'evidence_decision']",
    ],
    summary: "min-w-0 [grid-area:summary]",
    decision: "flex min-w-0 flex-col gap-8 [grid-area:decision] @3xl:self-start",
    evidence: "flex min-w-0 flex-col gap-8 [grid-area:evidence]",
  },
});

export type TriageLayoutProps = {
  /** What the item is, in a few facts and badges: first at every width. */
  summary: ReactNode;
  /** The region with the one primary decision on the item (resolve, take an action). */
  decision: ReactNode;
  /** The regions to read before deciding (the posts, the history, the notes), in order. */
  evidence: ReactNode;
};

/** A queue item's page: its summary, its decision in reach, and the evidence to read. */
export function TriageLayout({ summary, decision, evidence }: TriageLayoutProps) {
  const styles = triageLayout();
  return (
    <div className={styles.root()}>
      <div className={styles.grid()}>
        <div className={styles.summary()}>{summary}</div>
        <div className={styles.decision()} data-triage-decision="">
          {decision}
        </div>
        <div className={styles.evidence()}>{evidence}</div>
      </div>
    </div>
  );
}
