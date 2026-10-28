// SPDX-License-Identifier: AGPL-3.0-only
// How the totals of the costs read in the viewer's locale (ADR-0067 §10): amounts are decimal strings in the API and
// are formatted with Intl.NumberFormat in their currency, totals in different currencies are never converted, and a
// period is named with Intl.DateTimeFormat in UTC, so that a date is the same day in every time zone.
import { useIntl } from "react-intl";
import { useT } from "../../../lib/i18n";
import { type CostGranularity, type CostPeriodTotal, type Money, utcDate } from "../api/costs";

/** Formats an amount of the API in its currency, in the viewer's locale (ADR-0067 §10). */
export function useFormatMoney(): (money: Money) => string {
  const intl = useIntl();
  // The amount is a decimal string with the digits of its currency; a Number keeps them for any real cost.
  return (money) => intl.formatNumber(Number(money.amount), { style: "currency", currency: money.currency });
}

/** Formats the totals of a period, one per currency and never converted: "¥12,300 + $45.67", or zero without any. */
export function useFormatTotals(): (totals: readonly Money[]) => string {
  const t = useT();
  const format = useFormatMoney();
  return (totals) => {
    const [first, ...rest] = totals.map(format);
    if (first === undefined) return t("client:costs.totals.none");
    return rest.reduce((sum, amount) => t("client:costs.totals.plus", { sum, amount }), first);
  };
}

/** The name of a period in the viewer's locale: the month and year, the year, or the dates of an ISO week with its number. */
export function usePeriodName(): (
  granularity: CostGranularity,
  period: Pick<CostPeriodTotal, "label" | "starts_on" | "ends_on">,
) => string {
  const intl = useIntl();
  const t = useT();
  return (granularity, period) => {
    const start = utcDate(period.starts_on);
    if (granularity === "year") return intl.formatDate(start, { year: "numeric", timeZone: "UTC" });
    if (granularity === "month") return intl.formatDate(start, { year: "numeric", month: "long", timeZone: "UTC" });
    const range = intl.formatDateTimeRange(start, utcDate(period.ends_on), {
      year: "numeric",
      month: "short",
      day: "numeric",
      timeZone: "UTC",
    });
    return t("client:costs.period.week", { range, week: period.label });
  };
}
