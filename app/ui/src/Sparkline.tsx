// SPDX-License-Identifier: AGPL-3.0-only
// Sparkline (ADR-0113 §1): a small line of a value over time inside a row of a list, beside the number it draws. It is
// decorative (`aria-hidden`): the row always says the number in text, and the full series is the TrendChart's. It is
// drawn by hand in SVG (an exception to G10, listed in app/.oxlintrc.json); the line spans its box from the lowest to the
// highest value, the newest point marked, and runs along the line, so the newest point is on the left in right-to-left
// text.
import { useLocale } from "react-aria-components";
import { tv } from "tailwind-variants";
import { linePath, markedPoints, type Point, pointPosition } from "./chartScale";

const sparkline = tv({
  slots: {
    root: "inline-block h-6 w-20 shrink-0 align-middle",
    plot: "size-full overflow-visible",
    line: "forced-colors:stroke-[CanvasText]",
    dot: "forced-colors:stroke-[Highlight]",
  },
  variants: {
    series: {
      1: { line: "stroke-data-1", dot: "stroke-data-1" },
      2: { line: "stroke-data-2", dot: "stroke-data-2" },
      3: { line: "stroke-data-3", dot: "stroke-data-3" },
    },
  },
  defaultVariants: { series: 1 },
});

/** Room at the top and the bottom of the box, in percent, so that the round caps and the dot are not cut off. */
const inset = 12;

export type SparklineProps = {
  /** The values, the oldest first; `null` where a period has no value, which leaves a gap. */
  values: readonly (number | null)[];
  /** The series color (`data-1` to `data-3`), to match a TrendChart's series. */
  series?: 1 | 2 | 3;
};

/** A small decorative line of `values`. The caller shows the number it stands for as text in the same row. */
export function Sparkline({ values, series }: SparklineProps) {
  const { direction } = useLocale();
  const styles = sparkline({ series });
  const present = values.filter((value): value is number => value !== null && Number.isFinite(value));
  const low = Math.min(...present);
  const high = Math.max(...present);
  const y = (value: number) => (high === low ? 50 : inset + (1 - (value - low) / (high - low)) * (100 - 2 * inset));
  const points = values.map((value, index): Point | null =>
    value === null || !Number.isFinite(value)
      ? null
      : { x: pointPosition(index, values.length, direction), y: y(value) },
  );
  return (
    <span aria-hidden="true" className={styles.root()}>
      {present.length > 0 && (
        <svg viewBox="0 0 100 100" preserveAspectRatio="none" focusable="false" className={styles.plot()}>
          <path
            d={linePath(points)}
            fill="none"
            vectorEffect="non-scaling-stroke"
            strokeWidth={1.5}
            strokeLinecap="round"
            strokeLinejoin="round"
            className={styles.line()}
          />
          {markedPoints(values).map((index) => {
            const point = points[index];
            if (point === null || point === undefined) return null;
            return (
              <path
                key={index}
                d={`M${String(point.x)} ${String(point.y)} h0`}
                fill="none"
                vectorEffect="non-scaling-stroke"
                strokeWidth={5}
                strokeLinecap="round"
                className={styles.dot()}
              />
            );
          })}
        </svg>
      )}
    </span>
  );
}
