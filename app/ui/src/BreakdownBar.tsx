// SPDX-License-Identifier: AGPL-3.0-only
// BreakdownBar (ADR-0113 §1): the shares of a whole (the costs by item, the jobs by state) as segments of one bar, with
// a legend that says each item's name, value and share in text. The bar is drawn in HTML and is decorative; the legend
// carries the data. The segments are flex items in the line's direction, so they mirror in right-to-left text. A segment
// is told apart by its color and by the border between segments, which forced-colors mode keeps.
import { useLocale } from "react-aria-components";
import { tv } from "tailwind-variants";

const breakdownBar = tv({
  slots: {
    root: "flex min-w-0 flex-col gap-2",
    bar: "flex h-3 w-full overflow-hidden rounded-full bg-surface-sunken forced-colors:border forced-colors:border-[CanvasText]",
    // The border between segments is the surface's color, and CanvasText in forced-colors mode.
    segment: [
      "h-full shrink-0 border-s-2 border-surface-raised first:border-s-0 forced-colors:border-[CanvasText] forced-colors:bg-[Canvas]",
      "motion-safe:transition-[inline-size] motion-safe:duration-fast motion-safe:ease-standard",
    ],
    legend: "flex min-w-0 flex-col gap-1 text-sm",
    item: "flex min-w-0 items-baseline gap-2",
    swatch:
      "inline-block size-3 shrink-0 self-center rounded-sm forced-colors:border forced-colors:border-[CanvasText]",
    name: "min-w-0 flex-1 wrap-anywhere",
    value: "shrink-0 tabular-nums",
    share: "w-12 shrink-0 text-end text-fg-muted tabular-nums",
  },
});

/**
 * The colors of the items in order: the three series colors, then fg-muted for the fourth, which a caller uses for
 * "Other" when the whole has more parts.
 */
const itemColors = ["bg-data-1", "bg-data-2", "bg-data-3", "bg-fg-muted"] as const;

/** One part of the whole. */
export type BreakdownItem = {
  id: string;
  /** The part's name ("Hosting"). */
  name: string;
  /** Its amount, which sets its share of the whole; zero and negative amounts take no room. */
  value: number;
  /** Its amount as the caller formats it ("$12.00"). */
  valueText: string;
};

export type BreakdownBarProps = {
  /** What the whole is ("Costs in October"). It names the legend's list. */
  label: string;
  /**
   * Up to four parts, the largest first; fold the rest into a fourth "Other". A fifth part and beyond are left out of the
   * bar but kept in the legend.
   */
  items: readonly BreakdownItem[];
};

/** The shares of a whole as one bar of segments, with a legend of each part's name, value and share. */
export function BreakdownBar({ label, items }: BreakdownBarProps) {
  const { locale } = useLocale();
  const styles = breakdownBar();
  const percent = new Intl.NumberFormat(locale, { style: "percent", maximumFractionDigits: 0 });
  const total = items.reduce((sum, item) => sum + Math.max(0, item.value), 0);
  const share = (item: BreakdownItem) => (total > 0 ? Math.max(0, item.value) / total : 0);
  return (
    <div className={styles.root()}>
      <div aria-hidden="true" className={styles.bar()}>
        {items
          .slice(0, itemColors.length)
          .map((item, index) =>
            share(item) > 0 ? (
              <span
                key={item.id}
                style={{ inlineSize: `${String(share(item) * 100)}%` }}
                className={styles.segment({ class: itemColors[index] })}
              />
            ) : null,
          )}
      </div>
      <ul aria-label={label} className={styles.legend()}>
        {items.map((item, index) => (
          <li key={item.id} className={styles.item()}>
            <span aria-hidden="true" className={styles.swatch({ class: itemColors[index] ?? "bg-surface-sunken" })} />
            <span dir="auto" className={styles.name()}>
              {item.name}
            </span>
            <bdi dir="ltr" className={styles.value()}>
              {item.valueText}
            </bdi>
            <bdi dir="ltr" className={styles.share()}>
              {percent.format(share(item))}
            </bdi>
          </li>
        ))}
      </ul>
    </div>
  );
}
