// SPDX-License-Identifier: AGPL-3.0-only
// ProgressMeter (ADR-0113 §1): a known amount as a bar, on React Aria's ProgressBar (`progress`: how far a task with an
// end has gone, such as the deliveries after a service closes) or Meter (`meter`: how full something is, such as the
// storage used). Its label and its value in words ("1,234 / 5,000 items") are visible, and the value in words is the
// `aria-valuetext`. The fill grows from the start of the line, so it mirrors in right-to-left text; forced-colors mode
// draws it in Highlight inside a CanvasText border. A meter may set apart the pending part of its value (the requests
// not decided yet): a lighter segment at the end of the fill, GrayText in forced-colors mode, which the value in words
// says too.
import { Label, Meter, ProgressBar } from "react-aria-components";
import { tv } from "tailwind-variants";

const progressMeter = tv({
  slots: {
    root: "flex min-w-0 flex-col gap-1",
    top: "flex min-w-0 flex-wrap items-baseline justify-between gap-x-3",
    label: "min-w-0 text-sm font-bold wrap-anywhere",
    value: "text-sm text-fg-muted tabular-nums",
    track:
      "block h-2 w-full overflow-hidden rounded-full bg-surface-sunken forced-colors:border forced-colors:border-[CanvasText]",
    fill: [
      "flex h-full overflow-hidden rounded-full",
      "motion-safe:transition-[inline-size] motion-safe:duration-fast motion-safe:ease-standard",
    ],
    settled: "block h-full min-w-0 flex-1 forced-colors:bg-[Highlight]",
    pending: "block h-full shrink-0 opacity-40 forced-colors:bg-[GrayText] forced-colors:opacity-100",
  },
  variants: {
    tone: {
      normal: { settled: "bg-accent", pending: "bg-accent" },
      warning: { settled: "bg-warning", pending: "bg-warning" },
      danger: { settled: "bg-danger", pending: "bg-danger" },
    },
  },
  defaultVariants: { tone: "normal" },
});

export type ProgressMeterProps = {
  /** `progress` for a task with an end (a `progressbar`), `meter` for how full something is (a `meter`). */
  kind: "progress" | "meter";
  /** What is measured ("Deliveries sent", "Storage used"). */
  label: string;
  value: number;
  /** The lowest value, 0 by default. */
  minValue?: number;
  /** The highest value, 100 by default. */
  maxValue?: number;
  /** The value in words, as the caller formats it ("1,234 / 5,000 items"); shown and read as `aria-valuetext`. */
  valueText: string;
  /**
   * The level of a meter: `warning` and `danger` color the fill when the amount nears or passes its limit. Say the level
   * in `valueText` too ("4.8 / 5 GB, almost full"), as the color alone does not.
   */
  tone?: "normal" | "warning" | "danger";
  /**
   * The part of `value` that is pending (requests not decided yet), drawn as a lighter segment at the end of the fill.
   * Say it in `valueText` too ("3 of 5, 1 of them pending"), as the shade alone does not.
   */
  pending?: number;
};

/** A known amount as a bar with its label and its value in words. */
export function ProgressMeter({
  kind,
  label,
  value,
  minValue,
  maxValue,
  valueText,
  tone,
  pending,
}: ProgressMeterProps) {
  const styles = progressMeter({ tone });
  // The pending segment's share of the fill, which is the part of the range from minValue to value.
  const filled = value - (minValue ?? 0);
  const pendingShare = pending !== undefined && pending > 0 && filled > 0 ? Math.min(pending, filled) / filled : 0;
  const common = {
    value,
    valueLabel: valueText,
    ...(minValue !== undefined && { minValue }),
    ...(maxValue !== undefined && { maxValue }),
    className: styles.root(),
  };
  const content = ({ percentage }: { percentage: number | undefined }) => (
    <>
      <span className={styles.top()}>
        <Label className={styles.label()}>{label}</Label>
        <bdi dir="ltr" className={styles.value()}>
          {valueText}
        </bdi>
      </span>
      <span aria-hidden="true" className={styles.track()}>
        <span style={{ inlineSize: `${String(percentage ?? 0)}%` }} className={styles.fill()}>
          <span className={styles.settled()} />
          {pendingShare > 0 && (
            <span style={{ inlineSize: `${String(pendingShare * 100)}%` }} className={styles.pending()} />
          )}
        </span>
      </span>
    </>
  );
  return kind === "meter" ? <Meter {...common}>{content}</Meter> : <ProgressBar {...common}>{content}</ProgressBar>;
}
