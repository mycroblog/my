// SPDX-License-Identifier: AGPL-3.0-only
// What happens, in order, after an action (apply → a moderator reviews → an email arrives): a numbered list joined by a
// line, and at its end the action that starts it. It tells the steps by their shape before the reader reads them
// (web-client/21-experience-principles.md P3); it is a list to read, not a wizard that moves from step to step.
import type { ReactNode } from "react";
import { tv } from "tailwind-variants";

const steps = tv({
  slots: {
    base: "flex flex-col gap-4",
    list: "flex flex-col",
    item: [
      "relative grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 pb-4 last:pb-0",
      // The line down to the next step, under the numbers.
      "not-last:before:absolute not-last:before:top-8 not-last:before:bottom-0 not-last:before:inset-s-3.75 not-last:before:w-0.5 not-last:before:bg-border",
    ],
    number:
      "flex size-8 items-center justify-center rounded-full border-2 border-accent bg-surface font-bold text-accent tabular-nums forced-colors:border-[CanvasText] forced-colors:text-[CanvasText]",
    body: "flex min-w-0 flex-col justify-center gap-0.5 pt-1 wrap-anywhere",
    title: "font-semibold",
    detail: "text-sm text-fg-muted",
    action: "flex flex-wrap items-center gap-3",
  },
});

/** One step. */
export type Step = {
  /** Identifies the step among its siblings. */
  key: string;
  /** What happens, in a few words ("A moderator reviews it"). */
  title: ReactNode;
  /** A line more about it, under the title. */
  detail?: ReactNode;
};

export type StepsProps = {
  /** The steps in their order. */
  steps: readonly Step[];
  /** The action that starts the steps, after the last one (a `ButtonLink`). */
  action?: ReactNode;
  /** The id of the heading that names the list (`aria-labelledby`). */
  labelledBy?: string;
};

/** The numbered steps and the action after them. */
export function Steps({ steps: items, action, labelledBy }: StepsProps) {
  const slots = steps();
  return (
    <div className={slots.base()}>
      <ol aria-labelledby={labelledBy} className={slots.list()}>
        {items.map((step, index) => (
          <li key={step.key} className={slots.item()}>
            {/* The list gives the order; the number only draws it. */}
            <span aria-hidden="true" className={slots.number()}>
              {index + 1}
            </span>
            <span className={slots.body()}>
              <span className={slots.title()}>{step.title}</span>
              {step.detail !== undefined && <span className={slots.detail()}>{step.detail}</span>}
            </span>
          </li>
        ))}
      </ol>
      {action !== undefined && <div className={slots.action()}>{action}</div>}
    </div>
  );
}
