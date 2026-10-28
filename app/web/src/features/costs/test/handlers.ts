// SPDX-License-Identifier: AGPL-3.0-only
// The default MSW handlers of the costs feature (web-client/README.md §7): the public totals of the instance's costs, and the
// totals of the finances page, and the income totals of both.
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../../test/fixtures";
import { apiBase, problem } from "../../../test/msw/http";

/**
 * The public totals of the periods of a query (GET /instance/cost_totals): 12.34 USD and 1,650 yen in the last one,
 * with its breakdown, as an instance that publishes the breakdown answers it; nothing in the others.
 */
export function publicTotalsFixture(url: URL): Schemas["CostTotals"] {
  const granularity = (url.searchParams.get("granularity") ?? "month") as Schemas["CostGranularity"];
  const basis = (url.searchParams.get("basis") ?? "amortized") as Schemas["CostBasis"];
  const to = url.searchParams.get("to") ?? "2026-10-22";
  const month = to.slice(0, 7);
  return {
    granularity,
    basis,
    periods: [
      {
        label: month,
        starts_on: `${month}-01`,
        ends_on: to,
        totals: [
          { amount: "1650", currency: "JPY" },
          { amount: "12.34", currency: "USD" },
        ],
        breakdown: [
          { provider: "Registrar", category: "domain", amount: { amount: "1650", currency: "JPY" } },
          { provider: "Cloud", category: "compute", amount: { amount: "12.34", currency: "USD" } },
        ],
        estimated: false,
      },
    ],
    rules: { week_start: "monday", sources: [] },
  };
}

/**
 * The totals of the finances page (GET /finance/cost_totals) as a server with `viewer` answers them: the public
 * fixture's periods, with the lines of the last one's items (two, one with a memo) when names or memos are shown, and
 * only the fields the settings show.
 */
export function financeTotalsFixture(
  url: URL,
  viewer: Schemas["FinanceViewerSettings"] = { item_names: false, item_memos: false, income: "totals" },
): Schemas["FinanceCostTotals"] {
  const totals = publicTotalsFixture(url);
  const lines: Schemas["FinanceCostItemLine"][] = [
    {
      provider: "Cloud",
      category: "compute",
      amount: { amount: "12.34", currency: "USD" },
      name: "Main VM",
      memo: "the main server",
    },
    { provider: "Registrar", category: "domain", amount: { amount: "1650", currency: "JPY" }, name: "example.com" },
  ];
  const shown = viewer.item_names || viewer.item_memos;
  return {
    ...totals,
    viewer,
    periods: totals.periods.map((period, index) => ({
      ...period,
      items:
        shown && index === totals.periods.length - 1
          ? lines.map(({ name, memo, ...line }) => ({
              ...line,
              ...(viewer.item_names && name !== undefined ? { name } : {}),
              ...(viewer.item_memos && memo !== undefined ? { memo } : {}),
            }))
          : [],
    })),
  };
}

/**
 * The income totals of the periods of a query (GET /instance/income_totals, GET /finance/income_totals): 4,800 yen in
 * the last one, with its balance against the costs of publicTotalsFixture (3,150 yen and −12.34 USD) and the members of
 * a plan only where `shown` says so; nothing in the others.
 */
export function incomeTotalsFixture(
  url: URL,
  shown: { balance: boolean; plans: boolean } = { balance: true, plans: false },
): Schemas["IncomeTotals"] {
  const granularity = (url.searchParams.get("granularity") ?? "month") as Schemas["CostGranularity"];
  const basis = (url.searchParams.get("basis") ?? "amortized") as Schemas["CostBasis"];
  const to = url.searchParams.get("to") ?? "2026-10-22";
  const month = to.slice(0, 7);
  return {
    granularity,
    basis,
    periods: [
      {
        label: month,
        starts_on: `${month}-01`,
        ends_on: to,
        net: [{ amount: "4800", currency: "JPY" }],
        ...(shown.balance
          ? {
              balance: [
                { amount: "3150", currency: "JPY" },
                { amount: "-12.34", currency: "USD" },
              ],
            }
          : {}),
      },
    ],
    ...(shown.plans
      ? { plans: [{ plan_id: "0199f0e0-0000-7000-8000-00000000aa01", name: "Coffee", members: 3 }] }
      : {}),
    rules: {
      week_start: "monday",
      providers: [{ provider: "stripe", time_zone: "Asia/Tokyo" }],
      amortize_after_days: 31,
    },
  };
}

export const handlers = [
  http.get(`${apiBase}/instance/cost_totals`, ({ request }) =>
    HttpResponse.json(publicTotalsFixture(new URL(request.url))),
  ),
  http.get(`${apiBase}/finance/cost_totals`, ({ request }) =>
    HttpResponse.json(financeTotalsFixture(new URL(request.url))),
  ),
  // The instance fixture publishes no income (income_visibility: private).
  http.get(`${apiBase}/instance/income_totals`, () => problem(404, "about:blank", "Not Found")),
  http.get(`${apiBase}/finance/income_totals`, ({ request }) =>
    HttpResponse.json(incomeTotalsFixture(new URL(request.url))),
  ),
];
