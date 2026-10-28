// SPDX-License-Identifier: AGPL-3.0-only
// StatTile (ADR-0113 §1): one number of a summary (the open reports, the failed jobs, this month's costs) with its name
// and its change from the period before. The change shows by an arrow and its text ("+12%"), never by color alone, and a
// state that needs attention by a pill with an icon and a word in the status colors of Badge.
import type { ReactNode } from "react";
import { tv } from "tailwind-variants";
import { DecreaseIcon, IncreaseIcon, WarningIcon } from "./Icons";

const statTile = tv({
  slots: {
    root: "flex min-w-0 flex-col gap-1 rounded-lg border border-border bg-surface-raised p-4",
    top: "flex min-w-0 flex-wrap items-center justify-between gap-2",
    label: "min-w-0 text-sm font-bold text-fg-muted wrap-anywhere",
    status:
      "inline-flex shrink-0 items-center gap-1 rounded-full px-2 py-0.5 text-xs font-extrabold forced-colors:border forced-colors:border-[CanvasText] [&_svg]:size-4",
    value: "text-2xl font-extrabold tabular-nums wrap-anywhere",
    change: "flex min-w-0 flex-wrap items-center gap-x-1 text-sm [&_svg]:size-4",
    changeValue: "inline-flex items-center gap-0.5 font-bold tabular-nums",
    period: "text-fg-muted",
    extra: "mt-1",
  },
  variants: {
    status: {
      warning: { status: "bg-warning text-warning-fg" },
      danger: { status: "bg-danger text-danger-fg" },
    },
    tone: {
      positive: { changeValue: "text-success" },
      negative: { changeValue: "text-danger" },
      neutral: { changeValue: "text-fg" },
    },
  },
});

/** The change of the value from the period before. */
export type StatTileChange = {
  /** Which way the value moved; `flat` draws no arrow. */
  direction: "up" | "down" | "flat";
  /** The change as the caller formats it ("+12%", "−3"). */
  text: string;
  /** The period it is measured against ("from last week"), after the change. */
  period?: string;
  /**
   * Whether the change is good news: `positive` in the success color, `negative` in the danger color, `neutral` (the
   * default) in the text color. More errors is `negative` although it goes up.
   */
  tone?: "positive" | "negative" | "neutral";
};

export type StatTileProps = {
  /** What the number counts ("Open reports"). */
  label: string;
  /** The number as the caller formats it ("1,234", "$56.78"). */
  value: string;
  change?: StatTileChange;
  /** A state that needs attention, in the status colors of Badge with a warning icon and `statusLabel`. */
  status?: "warning" | "danger";
  /** The word of the state ("Needs review", "Failing"); required with `status`. */
  statusLabel?: string;
  /** Something to show under the change, such as a `Sparkline` of the same number. */
  children?: ReactNode;
};

/** One number of a summary: its name, its value, its change from the period before, and a state that needs attention. */
export function StatTile({ label, value, change, status, statusLabel, children }: StatTileProps) {
  const styles = statTile({ status, tone: change?.tone ?? "neutral" });
  return (
    <div className={styles.root()}>
      <div className={styles.top()}>
        <p className={styles.label()}>{label}</p>
        {status !== undefined && statusLabel !== undefined && (
          <span className={styles.status()}>
            <WarningIcon />
            {statusLabel}
          </span>
        )}
      </div>
      <p className={styles.value()}>
        <bdi dir="ltr">{value}</bdi>
      </p>
      {change !== undefined && (
        <p className={styles.change()}>
          <span className={styles.changeValue()}>
            {change.direction === "up" && <IncreaseIcon />}
            {change.direction === "down" && <DecreaseIcon />}
            <bdi dir="ltr">{change.text}</bdi>
          </span>
          {change.period !== undefined && <span className={styles.period()}>{change.period}</span>}
        </p>
      )}
      {children !== undefined && <div className={styles.extra()}>{children}</div>}
    </div>
  );
}
