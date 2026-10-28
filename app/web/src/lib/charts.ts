// SPDX-License-Identifier: AGPL-3.0-only
// What every TrendChart of the app shares (ADR-0113 §2): the form it shows, a chart or its table, which is one choice of
// this browser for every chart (`my:display:chart-view`), and the strings of the chart's own controls.
import { useT } from "./i18n";
import { chartViewKey, useStorage } from "./storage";

/** The forms of a TrendChart (`@my/ui`'s TrendChartView, which lib does not import). */
type ChartView = "chart" | "table";

/** The view and onViewChange props of a TrendChart: the form this browser shows the charts in. */
export function useChartView(): { view: ChartView; onViewChange: (view: ChartView) => void } {
  const view = useStorage(chartViewKey);
  return {
    view,
    onViewChange: (next) => {
      chartViewKey.write(next);
    },
  };
}

/**
 * The labels of a TrendChart: the switch and a missing value are the same for every chart; the column of the periods
 * (`period`, "Hour") and the text of a chart with no period (`empty`) are the caller's.
 */
export function useChartLabels({ period, empty }: { period: string; empty: string }): {
  view: string;
  chart: string;
  table: string;
  missing: string;
  period: string;
  empty: string;
} {
  const t = useT();
  return {
    view: t("client:chart.view"),
    chart: t("client:chart.view.chart"),
    table: t("client:chart.view.table"),
    missing: t("client:chart.missing"),
    period,
    empty,
  };
}
