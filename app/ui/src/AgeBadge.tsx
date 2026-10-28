// SPDX-License-Identifier: AGPL-3.0-only
// AgeBadge (ADR-0110 P3): how long an item of a queue has waited (a report, a registration, a job), as a StatusBadge
// with an hourglass and the time in words, in a tone that rises as the wait grows. The caller decides the level from
// its own thresholds and writes the words; the badge is a `<time>` whose title is the full date.
import { WaitingIcon } from "./Icons";
import { StatusBadge } from "./StatusBadge";

/** How long the item has waited against what its queue expects. */
export type AgeLevel = "fresh" | "aging" | "overdue";

const tones = { fresh: "neutral", aging: "warning", overdue: "danger" } as const;

export type AgeBadgeProps = {
  /** The wait in words ("Waiting 3 days"). */
  label: string;
  /** When the wait started (ISO 8601), for the `<time>`. */
  dateTime: string;
  /** The full date and time it started, as the tooltip. */
  title: string;
  /** `fresh` (neutral, the default), `aging` (warning) or `overdue` (danger); the words say the wait on their own. */
  level?: AgeLevel;
};

/** How long an item of a queue has waited: an hourglass and the wait in words, its tone rising with the level. */
export function AgeBadge({ label, dateTime, title, level = "fresh" }: AgeBadgeProps) {
  return (
    <time dateTime={dateTime} title={title} data-level={level}>
      <StatusBadge icon={<WaitingIcon />} label={label} tone={tones[level]} />
    </time>
  );
}
