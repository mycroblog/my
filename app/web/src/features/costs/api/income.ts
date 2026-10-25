// SPDX-License-Identifier: AGPL-3.0-only
// The totals of the instance's income (ADR-0073 §5, §7, §9, web-client/README.md S1): the net income of each currency by
// week, month or year, with the balance against the costs under the same basis and the number of members of each
// supporter plan now where they are shown. The public totals (GET /instance/income_totals) are what the instance
// setting `income_visibility` publishes: nothing (404), the totals, or the totals with the members of each plan, and
// the balance only while the costs are public too. The totals of the finances page (GET /finance/income_totals) are
// for the holders of view_finance: always with the balance. Neither carries individual payments, the subscribers,
// the gross or the fees; only the administrators' costs screen reads those (the admin feature).
import { queryOptions } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";
import { ApiError } from "../../../lib/errors";
import { type CostBasis, type CostGranularity, totalsQuery } from "./costs";

type Schemas = components["schemas"];

export type IncomeTotals = Schemas["IncomeTotals"];
export type IncomePeriodTotal = Schemas["IncomePeriodTotal"];
export type IncomePlanCount = Schemas["IncomePlanCount"];

/**
 * The public income totals of the shown periods up to `today` (GET /instance/income_totals): a 404 when the instance
 * does not publish its income.
 */
export function publicIncomeTotalsQueryOptions(api: Api, granularity: CostGranularity, basis: CostBasis, today: string) {
  const query = totalsQuery(granularity, basis, today);
  return queryOptions({
    queryKey: ["get", "/instance/income_totals", query] as const,
    queryFn: async ({ signal }) => {
      const { data, error } = await api.fetch.GET("/instance/income_totals", { params: { query }, signal });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  });
}

/**
 * The income totals of the shown periods up to `today` for the holders of view_finance or manage_finance
 * (GET /finance/income_totals): always with the balance, and with the members of each plan as far as the instance
 * shows them.
 */
export function financeIncomeTotalsQueryOptions(api: Api, granularity: CostGranularity, basis: CostBasis, today: string) {
  const query = totalsQuery(granularity, basis, today);
  return queryOptions({
    queryKey: ["get", "/finance/income_totals", query] as const,
    queryFn: async ({ signal }) => {
      const { data, error } = await api.fetch.GET("/finance/income_totals", { params: { query }, signal });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  });
}
