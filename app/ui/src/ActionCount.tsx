// SPDX-License-Identifier: AGPL-3.0-only
// The count beside the icon of an action on a post (replies, boosts, favourites): compact in the page's locale
// ("1.2K"), nothing for zero. When the count changes after it first showed (the person's own favourite, a new reply),
// the new number slides in from below, so the result shows where the person pressed (web-client/21-experience-principles.md
// P2). It is a quiet reaction, not a celebrated one: 200 ms, no overshoot, and nothing moves with reduced motion,
// where the number itself is the change. The count the card opens with never moves.
import { useState } from "react";
import { useNumberFormatter } from "react-aria";
import { tv } from "tailwind-variants";

const actionCount = tv({
  base: "tabular-nums",
  variants: {
    changed: {
      true: [
        "inline-block motion-safe:transition-[translate,opacity] motion-safe:duration-normal motion-safe:ease-enter",
        "motion-safe:starting:translate-y-1/2 motion-safe:starting:opacity-0",
      ],
    },
  },
});

export type ActionCountProps = {
  value: number;
  /** For the action that it describes (`aria-describedby`). */
  id?: string | undefined;
};

/** A post action's count; it slides in when it changes. */
export function ActionCount({ value, id }: ActionCountProps) {
  const formatter = useNumberFormatter({ notation: "compact" });
  const [first] = useState(value);
  if (value === 0) return null;
  // A new number is a new element, so that it enters again.
  return (
    <span
      key={value}
      id={id}
      data-changed={value !== first ? "" : undefined}
      className={actionCount({ changed: value !== first })}
    >
      {formatter.format(value)}
    </span>
  );
}
