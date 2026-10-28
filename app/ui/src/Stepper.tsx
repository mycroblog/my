// SPDX-License-Identifier: AGPL-3.0-only
// Where a flow of a few steps stands (a report, a move of the account): one segment per step, those done and the
// current one in the accent, and the position in words ("Step 2 of 3") beside them. The shape shows how much is left
// before the words are read (ADR-0110 P3); the words carry it for assistive technology, and the segments are drawn only.
// The fill moves with a quiet color transition (web-client/21-experience-principles.md §21.2).
import { tv } from "tailwind-variants";

const stepper = tv({
  slots: {
    base: "flex min-w-0 flex-col gap-1.5",
    text: "text-sm text-fg-muted",
    track: "flex gap-1",
    segment: [
      "h-1 min-w-0 flex-1 rounded-full bg-border forced-colors:border forced-colors:border-[CanvasText]",
      "data-reached:bg-accent forced-colors:data-reached:bg-[Highlight]",
      "motion-safe:transition-colors motion-safe:duration-normal motion-safe:ease-standard",
    ],
  },
});

export type StepperProps = {
  /** The current step, from 1. */
  current: number;
  /** How many steps the flow has. */
  total: number;
  /** The position in words ("Step 2 of 3"), shown above the segments. */
  label: string;
};

/** The position in a flow of steps, in words and as segments. */
export function Stepper({ current, total, label }: StepperProps) {
  const slots = stepper();
  return (
    <div className={slots.base()}>
      <p className={slots.text()}>{label}</p>
      <div aria-hidden="true" className={slots.track()}>
        {Array.from({ length: total }, (_, index) => (
          <span key={index} data-reached={index < current ? "" : undefined} className={slots.segment()} />
        ))}
      </div>
    </div>
  );
}
