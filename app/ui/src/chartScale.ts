// SPDX-License-Identifier: AGPL-3.0-only
// The geometry of the hand-written charts (ADR-0113 §1): a linear value axis with ticks at steps of 1, 2 or 5 × 10ⁿ,
// and a time axis that runs along the line, so that in right-to-left text the newest period is on the left (§3).
// The charts draw in a 0–100 box on both axes (an SVG viewBox stretched to the chart's size), so every position here is a
// share of the box in percent.

/** The writing direction of the chart's locale, as React Aria's `useLocale().direction` reports it. */
export type Direction = "ltr" | "rtl";

/** The span between ticks for `span` split into about `count` steps: 1, 2 or 5 × 10ⁿ, at least the raw step. */
export function niceStep(span: number, count: number): number {
  const raw = span / Math.max(1, count);
  if (!(raw > 0) || !Number.isFinite(raw)) return 1;
  const power = 10 ** Math.floor(Math.log10(raw));
  const fraction = raw / power;
  const nice = fraction <= 1 ? 1 : fraction <= 2 ? 2 : fraction <= 5 ? 5 : 10;
  return nice * power;
}

/** Rounds away the binary noise of a sum of steps (0.1 + 0.2), and turns -0 into 0. */
function clean(value: number): number {
  const rounded = Number(value.toPrecision(12));
  return rounded === 0 ? 0 : rounded;
}

/**
 * The ticks of a value axis, from the lowest to the highest, at a nice step. The axis always holds zero, so a bar
 * grows from it and a negative value (a loss) shows below it. Without a value, or with only zeros, it is 0 to 1.
 * Missing values (`null`) and values that are not finite are left out.
 */
export function valueTicks(values: Iterable<number | null>, count = 4): number[] {
  let min = 0;
  let max = 0;
  for (const value of values) {
    if (value === null || !Number.isFinite(value)) continue;
    min = Math.min(min, value);
    max = Math.max(max, value);
  }
  if (min === max) max = 1;
  const step = niceStep(max - min, count);
  const first = Math.floor(clean(min / step)) * step;
  const last = Math.ceil(clean(max / step)) * step;
  const steps = Math.round((last - first) / step);
  return Array.from({ length: steps + 1 }, (_, i) => clean(first + i * step));
}

/**
 * The vertical position of `value`, in percent from the top of the box, on an axis from `ticks[0]` at the bottom to the
 * last tick at the top.
 */
export function valuePosition(value: number, ticks: readonly number[]): number {
  const low = ticks[0] ?? 0;
  const high = ticks.at(-1) ?? 1;
  if (high === low) return 50;
  return 100 - ((value - low) / (high - low)) * 100;
}

/**
 * The horizontal position of period `index` of `count` (the oldest first), in percent from the left of the box: the
 * middle of its band, so that a bar and a label sit at the same place. It runs right to left in a right-to-left locale.
 */
export function periodPosition(index: number, count: number, direction: Direction): number {
  const along = ((index + 0.5) / Math.max(1, count)) * 100;
  return direction === "rtl" ? 100 - along : along;
}

/**
 * The horizontal position of point `index` of `count` on a line that spans the whole box (a sparkline): the oldest at
 * the start of the line and the newest at its end, a single point in the middle.
 */
export function pointPosition(index: number, count: number, direction: Direction): number {
  const along = count <= 1 ? 50 : (index / (count - 1)) * 100;
  return direction === "rtl" ? 100 - along : along;
}

/**
 * The periods that get a label under a time axis of `count` periods, at most `max`: the first and the last always,
 * and between them every k-th period. A period too close to the last one is dropped so that their labels do not meet.
 */
export function labelledPeriods(count: number, max = 5): number[] {
  if (count <= 0) return [];
  if (count <= max) return Array.from({ length: count }, (_, i) => i);
  const step = Math.ceil((count - 1) / (Math.max(2, max) - 1));
  const shown: number[] = [];
  for (let i = 0; i < count - 1; i += step) shown.push(i);
  const last = count - 1;
  const before = shown.at(-1);
  if (before !== undefined && before !== 0 && last - before < step / 2) shown.pop();
  shown.push(last);
  return shown;
}

/** A point of a line in the box. */
export type Point = { x: number; y: number };

/**
 * The SVG path data of a line through `points`, broken where a point is missing (`null`): a missing period is a gap and
 * not a fall to zero. A run of a single point draws nothing here; the chart marks it with a dot.
 */
export function linePath(points: readonly (Point | null)[]): string {
  const runs: Point[][] = [];
  let run: Point[] = [];
  for (const point of points) {
    if (point === null) {
      if (run.length > 0) runs.push(run);
      run = [];
    } else {
      run.push(point);
    }
  }
  if (run.length > 0) runs.push(run);
  return runs
    .filter((r) => r.length > 1)
    .map((r) => r.map((p, i) => `${i === 0 ? "M" : "L"}${format(p.x)} ${format(p.y)}`).join(" "))
    .join(" ");
}

/**
 * The indexes of the points to mark with a dot: the newest point that has a value, and every point whose neighbours are
 * both missing, which a line cannot show.
 */
export function markedPoints(values: readonly (number | null)[]): number[] {
  const marked = new Set<number>();
  values.forEach((value, i) => {
    if (value === null) return;
    if ((values[i - 1] ?? null) === null && (values[i + 1] ?? null) === null) marked.add(i);
  });
  const newest = values.findLastIndex((value) => value !== null);
  if (newest >= 0) marked.add(newest);
  return [...marked].sort((a, b) => a - b);
}

/** A coordinate with at most three decimals, so that a path stays short and stable in snapshots. */
function format(value: number): string {
  return String(Math.round(value * 1000) / 1000);
}
