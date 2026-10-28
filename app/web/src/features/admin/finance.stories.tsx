// SPDX-License-Identifier: AGPL-3.0-only
// The instance's costs and income (/admin/finance; web-client/screens/admin-finance.md) in the states of ADR-0112 §1:
// the cost items, the totals of the costs and the income by week, month or year as charts, the billing imports, the
// money movements, and what the instance publishes of them.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
import { inLocale, narrow } from "../../test/stories";
import { costItemFixture } from "./test/handlers";
import { adminAnswering, adminFailingOn, adminLoadingOn, adminScreen, withoutPermission } from "./test/stories";

type Granularity = Schemas["CostGranularity"];
type Money = Schemas["Money"];

const day = 86_400_000;

function utc(date: string): Date {
  return new Date(`${date}T00:00:00Z`);
}

function dateString(date: Date): string {
  return date.toISOString().slice(0, 10);
}

/** The ISO 8601 week of `date`, as `YYYY-Www`. */
function isoWeek(date: Date): string {
  const thursday = new Date(date.getTime() + (3 - ((date.getUTCDay() + 6) % 7)) * day);
  const firstThursday = new Date(Date.UTC(thursday.getUTCFullYear(), 0, 4));
  const week = 1 + Math.round((thursday.getTime() - firstThursday.getTime()) / (7 * day));
  return `${String(thursday.getUTCFullYear())}-W${String(week).padStart(2, "0")}`;
}

/** The periods of `granularity` from `from` to `to` as the server cuts them, each with its index. */
function periodsOf(granularity: Granularity, from: string, to: string) {
  const periods: { label: string; starts_on: string; ends_on: string; index: number }[] = [];
  let start = utc(from);
  const end = utc(to);
  while (start <= end && periods.length < 60) {
    let next: Date;
    let label: string;
    if (granularity === "week") {
      next = new Date(start.getTime() + 7 * day);
      label = isoWeek(start);
    } else if (granularity === "month") {
      next = new Date(Date.UTC(start.getUTCFullYear(), start.getUTCMonth() + 1, 1));
      label = dateString(start).slice(0, 7);
    } else {
      next = new Date(Date.UTC(start.getUTCFullYear() + 1, 0, 1));
      label = String(start.getUTCFullYear());
    }
    periods.push({
      label,
      starts_on: dateString(start),
      ends_on: dateString(new Date(next.getTime() - day)),
      index: periods.length,
    });
    start = next;
  }
  return periods;
}

/** The scale of a period's amounts against a month's. */
const scale: Record<Granularity, number> = { week: 0.25, month: 1, year: 12 };

function yen(amount: number): Money {
  return { amount: String(Math.round(amount)), currency: "JPY" };
}

function dollars(amount: number): Money {
  return { amount: amount.toFixed(2), currency: "USD" };
}

/** The query of a totals request: its granularity, basis and dates. */
function totalsQuery(url: URL) {
  const params = url.searchParams;
  return {
    granularity: (params.get("granularity") ?? "month") as Granularity,
    basis: (params.get("basis") ?? "amortized") as Schemas["CostBasis"],
    from: params.get("from") ?? "2025-11-01",
    to: params.get("to") ?? "2026-10-29",
  };
}

/**
 * The costs of a small server that grew over the year: a VM and a database billed in yen, storage and traffic in
 * dollars imported from AWS, and the domain. The latest period is an estimate.
 */
function costTotals(url: URL): Schemas["CostTotals"] {
  const { granularity, basis, from, to } = totalsQuery(url);
  const periods = periodsOf(granularity, from, to);
  const factor = scale[granularity];
  return {
    granularity,
    basis,
    periods: periods.map(({ index, ...period }) => {
      const growth = 1 + index / periods.length;
      const compute = 2400 * factor * growth;
      const database = 1800 * factor;
      const domain = basis === "cash" ? (index === periods.length - 3 ? 1650 : 0) : 137.5 * factor;
      const storage = 8.4 * factor * growth;
      const network = 3.1 * factor * growth * growth;
      return {
        ...period,
        totals: [yen(compute + database + domain), dollars(storage + network)],
        breakdown: [
          { provider: "Sakura Cloud", category: "compute", amount: yen(compute) },
          { provider: "Sakura Cloud", category: "database", amount: yen(database) },
          ...(domain > 0 ? [{ provider: "Registrar", category: "domain" as const, amount: yen(domain) }] : []),
          { provider: "AWS", category: "storage", amount: dollars(storage) },
          { provider: "AWS", category: "network", amount: dollars(network) },
        ],
        estimated: index === periods.length - 1,
      };
    }),
    rules: { week_start: "monday", sources: [{ source: "aws", provider: "AWS", time_zone: "UTC" }] },
  };
}

/** The income of the supporters: a few hundred yen a month that grows, the fees, and the balance after the costs. */
function incomeTotals(url: URL): Schemas["AdminIncomeTotals"] {
  const { granularity, basis, from, to } = totalsQuery(url);
  const periods = periodsOf(granularity, from, to);
  const factor = scale[granularity];
  return {
    granularity,
    basis,
    periods: periods.map(({ index, ...period }) => {
      const gross = 500 * factor * (index + 1);
      const fee = gross * 0.036;
      const costs = (2400 + 1800 + 137.5) * factor;
      return {
        ...period,
        gross: [yen(gross)],
        fee: [yen(fee)],
        net: [yen(gross - fee)],
        balance: [yen(gross - fee - costs)],
      };
    }),
    plans: [
      { plan_id: "0199f0e0-0000-7000-8000-00000000aa01", name: "Supporter", members: 38 },
      { plan_id: "0199f0e0-0000-7000-8000-00000000aa02", name: "Patron", members: 9 },
      { plan_id: "0199f0e0-0000-7000-8000-00000000aa03", name: "Early bird", members: 0 },
    ],
    rules: {
      week_start: "monday",
      providers: [{ provider: "stripe", time_zone: "Asia/Tokyo" }],
      amortize_after_days: 31,
    },
  };
}

/** The recorded costs: of several categories and schedules, one with a price change, one ended, and long labels. */
const costItems: Schemas["CostItem"][] = [
  costItemFixture("0199f0e0-0000-7000-8000-0000000000a1"),
  costItemFixture("0199f0e0-0000-7000-8000-0000000000a2", {
    schedule: "months",
    starts_on: "2025-11-01",
    name: "Virtual machine (4 vCPU, 8 GB memory) for the web server and the workers",
    provider: "Sakura Cloud",
    category: "compute",
    memo: "Upgraded from 2 vCPU in May when the federation traffic doubled.",
    prices: [
      { effective_on: "2025-11-01", price: yen(1800), revised_at: "2025-11-01T00:00:00Z" },
      { effective_on: "2026-05-01", price: yen(3200), revised_at: "2026-05-01T00:00:00Z" },
    ],
    created_at: "2025-11-01T00:00:00Z",
  }),
  costItemFixture("0199f0e0-0000-7000-8000-0000000000a3", {
    schedule: "months",
    starts_on: "2025-11-01",
    name: "Managed PostgreSQL",
    provider: "Sakura Cloud",
    category: "database",
    prices: [{ effective_on: "2025-11-01", price: yen(1800), revised_at: "2025-11-01T00:00:00Z" }],
    created_at: "2025-11-01T00:00:00Z",
  }),
  costItemFixture("0199f0e0-0000-7000-8000-0000000000a4", {
    schedule: "months",
    starts_on: "2025-11-01",
    ends_on: "2026-06-30",
    name: "Transactional email",
    provider: "Mail provider",
    category: "email",
    memo: "Replaced by the free tier of another provider.",
    prices: [{ effective_on: "2025-11-01", price: dollars(15), revised_at: "2025-11-01T00:00:00Z" }],
    created_at: "2025-11-01T00:00:00Z",
  }),
  costItemFixture("0199f0e0-0000-7000-8000-0000000000a5", {
    schedule: "once",
    starts_on: "2026-02-14",
    name: "Logo design",
    provider: "Freelance designer",
    category: "other",
    prices: [{ effective_on: "2026-02-14", price: yen(30000), revised_at: "2026-02-14T00:00:00Z" }],
    created_at: "2026-02-14T00:00:00Z",
  }),
];

/** The billing import of AWS, whose last run hit the day's cap of requests. */
const billingImports: Schemas["BillingImportList"] = {
  items: [
    {
      source: "aws",
      provider: "AWS",
      time_zone: "UTC",
      method: "cost_explorer",
      last_run: {
        outcome: "failed",
        error_kind: "request_cap",
        requests: 0,
        bytes_billed: 0,
        occurred_at: "2026-10-28T01:00:00Z",
      },
      last_success_at: "2026-10-27T01:00:00Z",
      requests_today: 4,
      requests_this_month: 104,
      max_requests_per_day: 4,
      max_requests_per_month: 120,
      bytes_billed_this_month: 0,
      months: [
        { month: "2026-09", kind: "final", totals: [dollars(28.71)] },
        { month: "2026-10", kind: "estimate", totals: [dollars(31.24)] },
      ],
    },
  ],
};

const usernames = ["alice", "bob", "carol", "dave_the_very_long_username", "erin", "frank"];

/** The money movements: payments of monthly and yearly subscriptions, a fee, a refund of a deleted account. */
const movements: Schemas["IncomeMovement"][] = [
  ...usernames.map((username, index): Schemas["IncomeMovement"] => ({
    id: `0199f0e0-0000-7000-8000-00000000bb1${String(index)}`,
    provider: "stripe",
    provider_id: `txn_${String(index)}`,
    kind: "payment",
    subscription_id: `0199f0e0-0000-7000-8000-00000000cc0${String(index)}`,
    account_id: `0199f0e0-0000-7000-8000-00000000dd0${String(index)}`,
    username,
    occurred_at: `2026-10-${String(20 - index * 3).padStart(2, "0")}T03:00:00Z`,
    occurred_on: `2026-10-${String(20 - index * 3).padStart(2, "0")}`,
    covers_starts_on: `2026-10-${String(20 - index * 3).padStart(2, "0")}`,
    covers_ends_on: index % 2 === 0 ? "2026-11-19" : "2027-10-19",
    gross: yen(index % 2 === 0 ? 500 : 15000),
    fee: yen(index % 2 === 0 ? 18 : 540),
    net: yen(index % 2 === 0 ? 482 : 14460),
    recorded_at: `2026-10-${String(20 - index * 3).padStart(2, "0")}T03:01:00Z`,
  })),
  {
    id: "0199f0e0-0000-7000-8000-00000000bb02",
    provider: "stripe",
    provider_id: "txn_fee",
    kind: "fee",
    occurred_at: "2026-10-11T00:00:00Z",
    occurred_on: "2026-10-11",
    gross: yen(0),
    fee: yen(84),
    net: yen(-84),
    recorded_at: "2026-10-11T00:01:00Z",
  },
  {
    id: "0199f0e0-0000-7000-8000-00000000bb03",
    provider: "stripe",
    provider_id: "txn_refund",
    kind: "refund",
    parent_provider_id: "txn_0",
    subscription_id: "0199f0e0-0000-7000-8000-00000000cc09",
    occurred_at: "2026-10-09T00:00:00Z",
    occurred_on: "2026-10-09",
    gross: yen(-500),
    fee: yen(0),
    net: yen(-500),
    recorded_at: "2026-10-09T00:01:00Z",
  },
];

const finance = [
  http.get(`${apiBase}/admin/cost_totals`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(costTotals(new URL(request.url)))),
  ),
  http.get(`${apiBase}/admin/income_totals`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(incomeTotals(new URL(request.url)))),
  ),
  http.get(`${apiBase}/admin/cost_items`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: costItems } satisfies Schemas["CostItemPage"])),
  ),
  http.get(`${apiBase}/admin/billing_imports`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(billingImports)),
  ),
  http.get(`${apiBase}/admin/income_movements`, ({ request }) => {
    const kind = new URL(request.url).searchParams.get("kind");
    const items = movements.filter((movement) => kind === null || movement.kind === kind);
    return withViewer(request, () => HttpResponse.json({ items } satisfies Schemas["IncomeMovementPage"]));
  }),
];

const meta = { title: "Screens/Admin/Finance", ...adminScreen("/admin/finance", finance) } satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** A year of costs and income by month, amortized, with the items, the billing import and the money movements. */
export const Normal: Story = {};

/** The totals by week. */
export const Weekly: Story = { parameters: { screen: { path: "/admin/finance?granularity=week" } } };

/** The totals by year, as paid. */
export const YearlyCash: Story = { parameters: { screen: { path: "/admin/finance?granularity=year&basis=cash" } } };

/** The movements narrowed to the refunds. */
export const Refunds: Story = { parameters: { screen: { path: "/admin/finance?kind=refund" } } };

/** A server that recorded nothing yet (the default handlers). */
export const Empty: Story = adminAnswering();

/** A moderator without manage_finance. */
export const MissingPermission: Story = withoutPermission;

/** The totals of the costs have not arrived yet. */
export const Loading: Story = adminLoadingOn("get", "/admin/cost_totals");

/** The server failed to answer the cost items. */
export const ServerError: Story = adminFailingOn("get", "/admin/cost_items");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
