// SPDX-License-Identifier: AGPL-3.0-only
// The numbers of the instance's finances at a glance (ADR-0113), above the totals on the public costs page, the
// finances page and the administrators' costs screen: the costs, the income and the balance of the current period as
// StatTiles with their change from the period before, and the costs against the income of every shown period as a
// TrendChart of bars, with the same data as its table. A chart has one unit, so it is drawn when every amount is in one
// currency; amounts in several currencies are never converted (ADR-0067 §10), so then the tiles say each currency and
// the screens keep their tables of the totals instead of the chart.
import { StatTile, type StatTileChange, StatTileGrid, Stack, TrendChart } from "@my/ui";
import { useIntl } from "react-intl";
import { useChartLabels, useChartView } from "../../../lib/charts";
import { useT } from "../../../lib/i18n";
import type { CostGranularity, CostPeriodTotal, Money } from "../api/costs";
import type { IncomePeriodTotal } from "../api/income";
import { useFormatTotals, usePeriodName, usePeriodShortName } from "../hooks/useCostFormat";

/** The currency the amounts are in: one, none at all (every period is empty), or several. */
export type FinanceCurrency = { kind: "one"; currency: string } | { kind: "none" } | { kind: "mixed" };

/** The currency of every amount of the shown costs and income. */
export function financeCurrency(
  costs: readonly Pick<CostPeriodTotal, "totals">[] | undefined,
  income: readonly Pick<IncomePeriodTotal, "net">[] | undefined,
): FinanceCurrency {
  const currencies = new Set([
    ...(costs ?? []).flatMap((period) => period.totals.map((money) => money.currency)),
    ...(income ?? []).flatMap((period) => period.net.map((money) => money.currency)),
  ]);
  const [only] = currencies;
  if (only === undefined) return { kind: "none" };
  return currencies.size === 1 ? { kind: "one", currency: only } : { kind: "mixed" };
}

/** Whether the overview's chart covers the totals: then a screen shows no table of them besides the chart's own. */
export function chartsTotals(currency: FinanceCurrency): boolean {
  return currency.kind !== "mixed";
}

/** The amount of `currency` in `money`, zero without any. */
function amountIn(money: readonly Money[], currency: string | undefined): number {
  return money.filter((item) => item.currency === currency).reduce((sum, item) => sum + Number(item.amount), 0);
}

export type FinanceOverviewProps = {
  granularity: CostGranularity;
  /** The periods of the costs, oldest first; undefined where the costs are not shown. */
  costs: readonly CostPeriodTotal[] | undefined;
  /** The periods of the income, oldest first; undefined where the income is not shown. */
  income: readonly IncomePeriodTotal[] | undefined;
};

/** The current period's costs, income and balance as tiles, and the costs against the income of every period as bars. */
export function FinanceOverview({ granularity, costs, income }: FinanceOverviewProps) {
  const t = useT();
  const intl = useIntl();
  const periodName = usePeriodName();
  const shortName = usePeriodShortName();
  const formatTotals = useFormatTotals();
  const chartView = useChartView();
  const labels = useChartLabels({ period: t("client:costs.totals.period"), empty: t("client:costs.chart.empty") });
  const periods = costs ?? income ?? [];
  const currency = financeCurrency(costs, income);
  const code = currency.kind === "one" ? currency.currency : undefined;
  const money = (value: number, signed = false) =>
    code === undefined
      ? intl.formatNumber(value, { signDisplay: signed ? "exceptZero" : "auto" })
      : intl.formatNumber(value, { style: "currency", currency: code, signDisplay: signed ? "exceptZero" : "auto" });
  const incomeOf = new Map((income ?? []).map((period) => [period.label, period.net]));
  const costValues = (costs ?? []).map((period) => amountIn(period.totals, code));
  const incomeValues = periods.map((period) => amountIn(incomeOf.get(period.label) ?? [], code));
  const last = periods.length - 1;
  const current = periods[last];
  if (current === undefined) return null;
  const currentName = periodName(granularity, current);
  const since = t("client:costs.overview.since", { granularity });

  /** The change of `values` from the period before: a percentage, or nothing from zero; `upIs` says whether up is good. */
  const percentChange = (values: readonly number[], upIs: "positive" | "negative"): StatTileChange | undefined => {
    const now = values[last];
    const before = values[last - 1];
    if (currency.kind !== "one" || now === undefined || before === undefined || before === 0) return undefined;
    const ratio = (now - before) / before;
    const direction = ratio > 0 ? "up" : ratio < 0 ? "down" : "flat";
    const good = upIs === "positive" ? "positive" : "negative";
    const bad = upIs === "positive" ? "negative" : "positive";
    return {
      direction,
      text: intl.formatNumber(ratio, { style: "percent", maximumFractionDigits: 0, signDisplay: "exceptZero" }),
      period: since,
      tone: direction === "flat" ? "neutral" : direction === "up" ? good : bad,
    };
  };

  const balances =
    costs !== undefined && income !== undefined ? incomeValues.map((v, i) => v - (costValues[i] ?? 0)) : [];
  const balanceNow = balances[last];
  const balanceBefore = balances[last - 1];
  const balanceChange: StatTileChange | undefined =
    currency.kind !== "one" || balanceNow === undefined || balanceBefore === undefined
      ? undefined
      : {
          direction: balanceNow > balanceBefore ? "up" : balanceNow < balanceBefore ? "down" : "flat",
          text: money(balanceNow - balanceBefore, true),
          period: since,
          tone: balanceNow > balanceBefore ? "positive" : balanceNow < balanceBefore ? "negative" : "neutral",
        };

  const costPeak = Math.max(...costValues);
  const peakIndex = costValues.indexOf(costPeak);
  const peakPeriod = periods[peakIndex];
  const summary =
    costs !== undefined && income !== undefined
      ? t("client:costs.chart.summary.both", {
          period: currentName,
          costs: money(costValues[last] ?? 0),
          income: money(incomeValues[last] ?? 0),
          peak: money(costPeak),
          peakPeriod: peakPeriod === undefined ? currentName : periodName(granularity, peakPeriod),
        })
      : costs !== undefined
        ? t("client:costs.chart.summary.costs", {
            period: currentName,
            costs: money(costValues[last] ?? 0),
            peak: money(costPeak),
            peakPeriod: peakPeriod === undefined ? currentName : periodName(granularity, peakPeriod),
          })
        : t("client:costs.chart.summary.income", { period: currentName, income: money(incomeValues[last] ?? 0) });

  const shown = costs !== undefined && income !== undefined ? "both" : costs !== undefined ? "costs" : "income";
  return (
    <Stack gap="6">
      <StatTileGrid aria-label={t("client:costs.overview.label", { period: currentName })}>
        {costs !== undefined && (
          <StatTile
            as="li"
            label={t("client:costs.overview.costs", { granularity })}
            value={code === undefined ? formatTotals(costs[last]?.totals ?? []) : money(costValues[last] ?? 0)}
            {...optionalChange(percentChange(costValues, "negative"))}
          />
        )}
        {income !== undefined && (
          <StatTile
            as="li"
            label={t("client:costs.overview.income", { granularity })}
            value={
              code === undefined ? formatTotals(incomeOf.get(current.label) ?? []) : money(incomeValues[last] ?? 0)
            }
            {...optionalChange(percentChange(incomeValues, "positive"))}
          />
        )}
        {balanceNow !== undefined && currency.kind !== "mixed" && (
          <StatTile
            as="li"
            label={t("client:costs.overview.balance", { granularity })}
            value={money(balanceNow, true)}
            {...optionalChange(balanceChange)}
            {...(balanceNow < 0 && { status: "warning", statusLabel: t("client:costs.overview.balance.short") })}
          />
        )}
      </StatTileGrid>
      {chartsTotals(currency) && (
        <TrendChart
          title={t("client:costs.chart.title", { shown, granularity })}
          summary={summary}
          kind="bar"
          periods={periods.map((period) => ({
            id: period.label,
            label:
              "estimated" in period && period.estimated
                ? t("client:costs.chart.estimated_period", { period: shortName(granularity, period) })
                : shortName(granularity, period),
          }))}
          series={[
            ...(costs === undefined ? [] : [{ id: "costs", name: t("client:costs.public.costs"), values: costValues }]),
            ...(income === undefined
              ? []
              : [{ id: "income", name: t("client:income.totals.net"), values: incomeValues }]),
          ]}
          formatValue={(value) => money(value)}
          labels={labels}
          {...chartView}
        />
      )}
    </Stack>
  );
}

/** The change prop of a StatTile, left out when there is none. */
function optionalChange(change: StatTileChange | undefined): { change?: StatTileChange } {
  return change === undefined ? {} : { change };
}
