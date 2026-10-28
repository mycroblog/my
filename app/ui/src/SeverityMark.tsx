// SPDX-License-Identifier: AGPL-3.0-only
// SeverityMark (ADR-0110 P3): where one choice stands on a scale from the lightest to the heaviest (a moderation action),
// beside its name: a row of steps filled up to its level, in a tone that rises with it, and a "cannot be undone" badge
// for a choice that cannot be reversed. The steps are decorative; `label` ("Severity 3 of 10") says the level in words
// to assistive technology, so the mark adds to the choice's accessible name when it sits inside the label of a Radio.
import { tv } from "tailwind-variants";
import { WarningIcon } from "./Icons";
import { StatusBadge } from "./StatusBadge";
import { VisuallyHidden } from "./VisuallyHidden";

const severityMark = tv({
  slots: {
    root: "inline-flex flex-wrap items-center gap-2",
    steps: "inline-flex items-center gap-0.5",
    step: "h-2 w-1.5 rounded-full bg-border forced-colors:bg-[GrayText]",
  },
  variants: {
    tone: {
      low: {},
      medium: {},
      high: {},
    },
    filled: { true: {} },
  },
  compoundVariants: [
    { filled: true, tone: "low", class: { step: "bg-fg-muted forced-colors:bg-[CanvasText]" } },
    { filled: true, tone: "medium", class: { step: "bg-warning forced-colors:bg-[CanvasText]" } },
    { filled: true, tone: "high", class: { step: "bg-danger forced-colors:bg-[CanvasText]" } },
  ],
});

export type SeverityMarkProps = {
  /** The choice's step, from 1 (the lightest) to `max`. */
  level: number;
  /** The number of steps of the scale. */
  max: number;
  /** The level in words ("Severity 3 of 10"), for assistive technology. */
  label: string;
  /** The words of the badge of a choice that cannot be undone ("Cannot be undone"); none for one that can. */
  irreversibleLabel?: string;
};

/** Where a choice stands on a scale of severity: filled steps, its level in words, and whether it can be undone. */
export function SeverityMark({ level, max, label, irreversibleLabel }: SeverityMarkProps) {
  const tone = level * 3 <= max ? "low" : level * 3 <= max * 2 ? "medium" : "high";
  const styles = severityMark({ tone });
  return (
    <span className={styles.root()}>
      <span className={styles.steps()} aria-hidden="true">
        {Array.from({ length: max }, (_, index) => (
          <span key={index} className={severityMark({ tone, filled: index < level }).step()} />
        ))}
      </span>
      <VisuallyHidden>{label}</VisuallyHidden>
      {irreversibleLabel !== undefined && (
        <StatusBadge icon={<WarningIcon />} label={irreversibleLabel} tone="danger" />
      )}
    </span>
  );
}
