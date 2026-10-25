// SPDX-License-Identifier: AGPL-3.0-only
// The totals of the instance's costs (ADR-0067 §4, §5, §9, web-client/README.md S1): the types and the dates that the
// administrators' costs screen, the public costs page and the finances page share; the public totals
// (GET /instance/cost_totals), which the instance setting `finance_visibility` publishes: nothing (404), the totals
// only, or their breakdown by provider and category; and the totals of the finances page (GET /finance/cost_totals,
// ADR-0070 §5), for the holders of view_finance, with the lines of the items as far as the instance settings show.
import { queryOptions } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";
import { ApiError } from "../../../lib/errors";

type Schemas = components["schemas"];

export type CostCategory = Schemas["CostCategory"];
export type CostBasis = Schemas["CostBasis"];
export type CostGranularity = Schemas["CostGranularity"];
export type CostTotals = Schemas["CostTotals"];
export type CostPeriodTotal = Schemas["CostPeriodTotal"];
export type Money = Schemas["Money"];
export type FinanceCostTotals = Schemas["FinanceCostTotals"];
export type FinanceCostItemLine = Schemas["FinanceCostItemLine"];
export type FinanceViewerSettings = Schemas["FinanceViewerSettings"];

/** The categories, in the order of the API (ADR-0067 §2). */
export const costCategories = [
  "compute",
  "database",
  "storage",
  "network",
  "domain",
  "email",
  "software",
  "other",
] as const satisfies readonly CostCategory[];

/** The granularities of the totals, in the order of their tabs. */
export const costGranularities = ["week", "month", "year"] as const satisfies readonly CostGranularity[];

/** The bases of the totals; amortized is the default (ADR-0067 §4). */
export const costBases = ["amortized", "cash"] as const satisfies readonly CostBasis[];

/** How many periods back from the current one the totals show, the current one included. */
const shownPeriods: Readonly<Record<CostGranularity, number>> = { week: 12, month: 12, year: 5 };

/** A calendar date `YYYY-MM-DD` as a UTC Date, for date arithmetic and for Intl in the UTC time zone. */
export function utcDate(date: string): Date {
  return new Date(`${date}T00:00:00Z`);
}

/** The calendar date `YYYY-MM-DD` of a UTC Date. */
function dateString(date: Date): string {
  return date.toISOString().slice(0, 10);
}

/**
 * Today's calendar date in the device's time zone (ADR-0067 §5: only "today" depends on the viewer, and the API
 * takes dates). Called outside rendering, by the routes.
 */
export function deviceToday(now: Date = new Date()): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${String(now.getFullYear()).padStart(4, "0")}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`;
}

/** The query of the totals: from a date in the first of the shown periods to `today`, in the last one. */
export function totalsQuery(granularity: CostGranularity, basis: CostBasis, today: string) {
  const date = utcDate(today);
  const back = shownPeriods[granularity] - 1;
  if (granularity === "week") date.setUTCDate(date.getUTCDate() - 7 * back);
  // The first day of the month or year: stepping back from the 31st would skip a shorter month.
  else if (granularity === "month") date.setUTCFullYear(date.getUTCFullYear(), date.getUTCMonth() - back, 1);
  else date.setUTCFullYear(date.getUTCFullYear() - back, 0, 1);
  return { granularity, basis, from: dateString(date), to: today };
}

/**
 * The public totals of the shown periods up to `today` (GET /instance/cost_totals): a 404 when the instance does not
 * publish its costs, no breakdown when it publishes the totals only.
 */
export function publicCostTotalsQueryOptions(api: Api, granularity: CostGranularity, basis: CostBasis, today: string) {
  const query = totalsQuery(granularity, basis, today);
  return queryOptions({
    queryKey: ["get", "/instance/cost_totals", query] as const,
    queryFn: async ({ signal }) => {
      const { data, error } = await api.fetch.GET("/instance/cost_totals", { params: { query }, signal });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  });
}

/**
 * The totals of the shown periods up to `today` for the holders of view_finance or manage_finance
 * (GET /finance/cost_totals): always with their breakdown, with the lines of the items while the instance shows their
 * names or memos, and the instance settings that decide it.
 */
export function financeCostTotalsQueryOptions(api: Api, granularity: CostGranularity, basis: CostBasis, today: string) {
  const query = totalsQuery(granularity, basis, today);
  return queryOptions({
    queryKey: ["get", "/finance/cost_totals", query] as const,
    queryFn: async ({ signal }) => {
      const { data, error } = await api.fetch.GET("/finance/cost_totals", { params: { query }, signal });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  });
}
