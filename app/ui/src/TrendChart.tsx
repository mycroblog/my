// SPDX-License-Identifier: AGPL-3.0-only
// TrendChart (ADR-0113 §1): the values of up to three series over periods, as bars or lines, with a switch to the same
// data as a table. The chart is one image (`role="img"`) named by its title and described by its summary sentence, which
// the caller writes (the largest, the latest, the change); its values are read in the table. It is drawn by hand in SVG
// (an exception to G10, listed in app/.oxlintrc.json): the plot is a 0–100 box stretched to the chart's size, with
// strokes that do not scale, and the axis labels are HTML around it. The time axis runs along the line, so the newest
// period is on the left in right-to-left text; SVG does not follow `dir`, so the positions mirror by the locale's
// direction. A series is told apart by its color, its line's dash and the legend's text; forced-colors mode draws every
// series in CanvasText and keeps the dashes.
import { useId } from "react";
import { useLocale } from "react-aria-components";
import { tv } from "tailwind-variants";
import {
  labelledPeriods,
  linePath,
  markedPoints,
  periodPosition,
  type Point,
  valuePosition,
  valueTicks,
} from "./chartScale";
import { Segmented, SegmentedItem } from "./Segmented";
import { Table, TableBody, TableCell, TableColumnHeader, TableHead, TableRow, TableRowHeader } from "./Table";

const trendChart = tv({
  slots: {
    root: "flex min-w-0 flex-col gap-3",
    header: "flex min-w-0 flex-wrap items-center justify-between gap-2",
    title: "min-w-0 font-bold wrap-anywhere",
    figure: "grid min-w-0 grid-cols-[auto_minmax(0,1fr)] gap-x-2",
    // Ticks are evenly spaced, so the labels spread over the plot's height; each is zero high and centred on its line.
    yAxis: "flex h-40 flex-col-reverse justify-between text-end text-xs text-fg-muted tabular-nums",
    yTick: "flex h-0 items-center justify-end",
    plot: "h-40 w-full overflow-visible",
    grid: "stroke-border forced-colors:stroke-[GrayText]",
    zero: "stroke-border-strong forced-colors:stroke-[CanvasText]",
    xAxis: "relative col-start-2 mt-1 h-5 text-xs text-fg-muted tabular-nums",
    xTick: "absolute top-0 whitespace-nowrap",
    legend: "flex min-w-0 flex-wrap gap-x-4 gap-y-1 text-sm",
    legendItem: "flex min-w-0 items-center gap-2",
    swatch: "inline-block w-5 shrink-0 border-t-3 forced-colors:border-[CanvasText]",
    summary: "text-sm text-fg-muted",
    empty: "text-sm text-fg-muted",
  },
  variants: {
    // The first label starts at the start of the axis and the last ends at its end, so neither leaves the chart; the
    // others are centred on their period.
    edge: {
      start: { xTick: "" },
      middle: { xTick: "-translate-x-1/2 rtl:translate-x-1/2" },
      end: { xTick: "-translate-x-full rtl:translate-x-full" },
      only: { xTick: "-translate-x-1/2 rtl:translate-x-1/2" },
    },
  },
});

/** The look of each series, in order: its color, its line's dash, and its bar in forced-colors mode. */
const seriesLook = [
  {
    line: "stroke-data-1 forced-colors:stroke-[CanvasText]",
    bar: "fill-data-1 stroke-none forced-colors:fill-[CanvasText]",
    swatch: "border-solid border-data-1",
    dash: undefined,
  },
  {
    line: "stroke-data-2 forced-colors:stroke-[CanvasText]",
    // Forced colors leave one color, so the bars of the second and third series are outlines with their dash.
    bar: "fill-data-2 stroke-none forced-colors:fill-[Canvas] forced-colors:stroke-[CanvasText]",
    swatch: "border-dashed border-data-2",
    dash: "6 4",
  },
  {
    line: "stroke-data-3 forced-colors:stroke-[CanvasText]",
    bar: "fill-data-3 stroke-none forced-colors:fill-[Canvas] forced-colors:stroke-[CanvasText]",
    swatch: "border-dotted border-data-3",
    dash: "1 4",
  },
] as const;

/** The newest value of a line, marked with a dot; forced-colors mode marks it in Highlight. */
const dotLook = [
  "stroke-data-1 forced-colors:stroke-[Highlight]",
  "stroke-data-2 forced-colors:stroke-[Highlight]",
  "stroke-data-3 forced-colors:stroke-[Highlight]",
] as const;

/** One period of the time axis, the oldest first. */
export type TrendPeriod = {
  id: string;
  /** The period as the caller formats it ("Oct 28", "14:00"), under the axis and in the table's row header. */
  label: string;
};

/** One series: its values per period, in the order of `periods`; `null` where the period has no value. */
export type TrendSeries = {
  id: string;
  /** The series' name, in the legend and the table's column header. */
  name: string;
  values: readonly (number | null)[];
};

/** Which form shows the data. */
export type TrendChartView = "chart" | "table";

/** The strings of a TrendChart other than the data's; `@my/ui` has no catalog, so the caller formats them. */
export type TrendChartLabels = {
  /** The name of the switch between the chart and the table ("Show as"). */
  view: string;
  /** The switch's options. */
  chart: string;
  table: string;
  /** The column header of the periods in the table ("Day"). */
  period: string;
  /** A missing value in the table ("No data"). */
  missing: string;
  /** Shown instead of the chart when there is no period ("No errors were recorded in this period."). */
  empty: string;
};

export type TrendChartProps = {
  /** What the chart shows ("Errors per hour"). It names the chart and the table. */
  title: string;
  /**
   * One sentence of what the chart shows, written by the caller: the largest value, the latest value and the change
   * ("Peaked at 42 at 14:00; 12 in the last hour, down 30% from the hour before."). It describes the chart.
   */
  summary: string;
  /** `bar` for amounts per period (costs, counts), `line` for a level over time (a queue's length). */
  kind?: "bar" | "line";
  periods: readonly TrendPeriod[];
  /** Up to three series. */
  series: readonly TrendSeries[];
  /** Formats a value for the axis and the table, in the caller's locale and unit. */
  formatValue: (value: number) => string;
  /**
   * The form shown. It is the screen's state, kept in the URL or `lib/storage`, so that a reader who prefers the table
   * keeps it.
   */
  view: TrendChartView;
  onViewChange: (view: TrendChartView) => void;
  labels: TrendChartLabels;
};

/** Up to three series of values over periods, as a chart or as a table. */
export function TrendChart({
  title,
  summary,
  kind = "line",
  periods,
  series,
  formatValue,
  view,
  onViewChange,
  labels,
}: TrendChartProps) {
  const titleId = useId();
  const summaryId = useId();
  const styles = trendChart();
  const shown = series.slice(0, seriesLook.length);
  const empty = periods.length === 0;
  return (
    <div className={styles.root()}>
      <div className={styles.header()}>
        <p id={titleId} className={styles.title()}>
          {title}
        </p>
        {!empty && (
          <Segmented
            aria-label={labels.view}
            value={view}
            onChange={(key) => {
              onViewChange(key === "table" ? "table" : "chart");
            }}
          >
            <SegmentedItem id="chart">{labels.chart}</SegmentedItem>
            <SegmentedItem id="table">{labels.table}</SegmentedItem>
          </Segmented>
        )}
      </div>
      {empty ? (
        <p className={styles.empty()}>{labels.empty}</p>
      ) : view === "table" ? (
        <TrendTable
          titleId={titleId}
          periods={periods}
          series={shown}
          formatValue={formatValue}
          periodLabel={labels.period}
          missing={labels.missing}
        />
      ) : (
        <>
          <Plot
            titleId={titleId}
            summaryId={summaryId}
            kind={kind}
            periods={periods}
            series={shown}
            formatValue={formatValue}
          />
          <ul className={styles.legend()}>
            {shown.map((s, index) => (
              <li key={s.id} className={styles.legendItem()}>
                <span aria-hidden="true" className={styles.swatch({ class: seriesLook[index]?.swatch })} />
                <span dir="auto">{s.name}</span>
              </li>
            ))}
          </ul>
        </>
      )}
      {!empty && (
        <p id={summaryId} className={styles.summary()}>
          {summary}
        </p>
      )}
    </div>
  );
}

type PlotProps = {
  titleId: string;
  summaryId: string;
  kind: "bar" | "line";
  periods: readonly TrendPeriod[];
  series: readonly TrendSeries[];
  formatValue: (value: number) => string;
};

/** The share of a period's band that its bars fill, so that bars of neighbouring periods stand apart. */
const barGroupShare = 0.7;

function Plot({ titleId, summaryId, kind, periods, series, formatValue }: PlotProps) {
  const { direction } = useLocale();
  const styles = trendChart();
  const count = periods.length;
  const ticks = valueTicks(series.flatMap((s) => s.values));
  const zero = valuePosition(0, ticks);
  const y = (value: number) => valuePosition(value, ticks);
  const x = (index: number) => periodPosition(index, count, direction);
  const band = 100 / count;
  const barWidth = (band * barGroupShare) / Math.max(1, series.length);
  const labelled = labelledPeriods(count);
  return (
    <div role="img" aria-labelledby={titleId} aria-describedby={summaryId} className={styles.figure()}>
      <div className={styles.yAxis()}>
        {ticks.map((tick) => (
          <span key={tick} className={styles.yTick()}>
            <bdi dir="ltr">{formatValue(tick)}</bdi>
          </span>
        ))}
      </div>
      <svg viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden="true" className={styles.plot()}>
        {ticks.map((tick) => (
          <line
            key={tick}
            x1={0}
            x2={100}
            y1={y(tick)}
            y2={y(tick)}
            vectorEffect="non-scaling-stroke"
            strokeWidth={1}
            className={tick === 0 ? styles.zero() : styles.grid()}
          />
        ))}
        {kind === "bar"
          ? series.map((s, seriesIndex) => {
              const look = seriesLook[seriesIndex] ?? seriesLook[0];
              return s.values.map((value, period) => {
                if (value === null || period >= count) return null;
                // The bars of a period follow the line's direction too: the first series at the start of the line.
                const along = -(band * barGroupShare) / 2 + seriesIndex * barWidth;
                const left = direction === "rtl" ? x(period) - along - barWidth : x(period) + along;
                const top = Math.min(y(value), zero);
                return (
                  <rect
                    key={`${s.id}-${periods[period]?.id ?? period}`}
                    x={left}
                    width={barWidth}
                    y={top}
                    height={Math.abs(zero - y(value))}
                    vectorEffect="non-scaling-stroke"
                    strokeWidth={2}
                    strokeDasharray={look.dash}
                    className={look.bar}
                  />
                );
              });
            })
          : series.map((s, seriesIndex) => {
              const look = seriesLook[seriesIndex] ?? seriesLook[0];
              const points = s.values
                .slice(0, count)
                .map((value, period): Point | null => (value === null ? null : { x: x(period), y: y(value) }));
              const dot = dotLook[seriesIndex] ?? dotLook[0];
              return (
                <g key={s.id}>
                  <path
                    d={linePath(points)}
                    fill="none"
                    vectorEffect="non-scaling-stroke"
                    strokeWidth={2}
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeDasharray={look.dash}
                    className={look.line}
                  />
                  {markedPoints(s.values.slice(0, count)).map((period) => {
                    const point = points[period];
                    if (point === null || point === undefined) return null;
                    // A line of no length with round caps is a dot that the stretched box does not distort.
                    return (
                      <path
                        key={period}
                        d={`M${String(point.x)} ${String(point.y)} h0`}
                        fill="none"
                        vectorEffect="non-scaling-stroke"
                        strokeWidth={7}
                        strokeLinecap="round"
                        className={dot}
                      />
                    );
                  })}
                </g>
              );
            })}
      </svg>
      <div className={styles.xAxis()}>
        {labelled.map((period) => {
          const edge =
            count === 1 ? "only" : period === 0 ? "start" : period === count - 1 ? "end" : ("middle" as const);
          // Logical offsets follow the writing direction, so the labels mirror with the plot.
          const along = edge === "start" ? 0 : edge === "end" ? 100 : ((period + 0.5) / count) * 100;
          return (
            <span
              key={periods[period]?.id ?? period}
              style={{ insetInlineStart: `${String(along)}%` }}
              className={trendChart({ edge }).xTick()}
            >
              <bdi dir="ltr">{periods[period]?.label}</bdi>
            </span>
          );
        })}
      </div>
    </div>
  );
}

type TrendTableProps = {
  titleId: string;
  periods: readonly TrendPeriod[];
  series: readonly TrendSeries[];
  formatValue: (value: number) => string;
  periodLabel: string;
  missing: string;
};

/** The same data as a table: a row per period, a column per series. */
function TrendTable({ titleId, periods, series, formatValue, periodLabel, missing }: TrendTableProps) {
  return (
    <Table aria-labelledby={titleId}>
      <TableHead>
        <TableColumnHeader>{periodLabel}</TableColumnHeader>
        {series.map((s) => (
          <TableColumnHeader key={s.id}>
            <span dir="auto">{s.name}</span>
          </TableColumnHeader>
        ))}
      </TableHead>
      <TableBody>
        {periods.map((period, index) => (
          <TableRow key={period.id}>
            <TableRowHeader>
              <bdi dir="ltr">{period.label}</bdi>
            </TableRowHeader>
            {series.map((s) => {
              const value = s.values[index] ?? null;
              return (
                <TableCell key={s.id} numeric>
                  {value === null ? missing : <bdi dir="ltr">{formatValue(value)}</bdi>}
                </TableCell>
              );
            })}
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
