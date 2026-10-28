// SPDX-License-Identifier: AGPL-3.0-only
// The data of the screen stories of the costs (costs.stories.tsx) and of the finances (finance.stories.tsx): six periods
// that differ, ending with the default handlers' last one, which has the breakdown and the items.
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../../test/fixtures";
import { apiBase } from "../../../test/msw/http";
import { financeTotalsFixture, incomeTotalsFixture, publicTotalsFixture } from "./handlers";

/** The labels and dates of the five periods before the last one of `url`'s query, oldest first, by month. */
function earlierMonths(url: URL): { label: string; starts_on: string; ends_on: string }[] {
  const to = url.searchParams.get("to") ?? "2026-10-22";
  const [year, month] = to.split("-").map(Number) as [number, number];
  return [5, 4, 3, 2, 1].map((back) => {
    const date = new Date(Date.UTC(year, month - 1 - back, 1));
    const label = date.toISOString().slice(0, 7);
    const last = new Date(Date.UTC(date.getUTCFullYear(), date.getUTCMonth() + 1, 0)).toISOString().slice(0, 10);
    return { label, starts_on: `${label}-01`, ends_on: last };
  });
}

/** The cost totals of an earlier period `i`: the yen of the domain once a year, and dollars that grow. */
function earlierTotals(i: number): Schemas["Money"][] {
  return [
    ...(i === 0 ? [{ amount: "1650", currency: "JPY" }] : []),
    { amount: (8.5 + i * 0.75).toFixed(2), currency: "USD" },
  ];
}

/** GET /instance/cost_totals with six periods. */
export const publicCostsOverSixMonths = http.get(`${apiBase}/instance/cost_totals`, ({ request }) => {
  const url = new URL(request.url);
  const totals = publicTotalsFixture(url);
  return HttpResponse.json({
    ...totals,
    periods: [
      ...earlierMonths(url).map((period, i) => ({
        ...period,
        totals: earlierTotals(i),
        breakdown: [],
        estimated: false,
      })),
      ...totals.periods.map((period) => ({ ...period, estimated: true })),
    ],
  } satisfies Schemas["CostTotals"]);
});

/** GET /finance/cost_totals with six periods, and the names and memos of the last one's items. */
export const financeCostsOverSixMonths = http.get(`${apiBase}/finance/cost_totals`, ({ request }) => {
  const url = new URL(request.url);
  const totals = financeTotalsFixture(url, { item_names: true, item_memos: true, income: "plans" });
  return HttpResponse.json({
    ...totals,
    periods: [
      ...earlierMonths(url).map((period, i) => ({
        ...period,
        totals: earlierTotals(i),
        breakdown: [],
        items: [],
        estimated: false,
      })),
      ...totals.periods,
    ],
  } satisfies Schemas["FinanceCostTotals"]);
});

/** The income totals of `path` with six periods, the balance, and the members of each plan. */
export function incomeOverSixMonths(path: "/instance/income_totals" | "/finance/income_totals") {
  return http.get(`${apiBase}${path}`, ({ request }) => {
    const url = new URL(request.url);
    const totals = incomeTotalsFixture(url, { balance: true, plans: true });
    return HttpResponse.json({
      ...totals,
      periods: [
        ...earlierMonths(url).map((period, i) => ({
          ...period,
          net: [{ amount: String(3000 + i * 300), currency: "JPY" }],
          balance: [{ amount: String(1350 + i * 300), currency: "JPY" }],
        })),
        ...totals.periods,
      ],
      plans: [
        { plan_id: "0199f0e0-0000-7000-8000-00000000aa01", name: "Coffee", members: 3 },
        { plan_id: "0199f0e0-0000-7000-8000-00000000aa02", name: "A whole loaf of sourdough every month", members: 12 },
      ],
    } satisfies Schemas["IncomeTotals"]);
  });
}
