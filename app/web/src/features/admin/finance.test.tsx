// SPDX-License-Identifier: AGPL-3.0-only
// The costs screen through the whole app (web-client/README.md T4, ADR-0067, ADR-0068): an administrator with
// manage_finance sees the totals by month amortized by default and switches to weeks, years and cash, with the rules
// next to them; records a cost paid every few months; edits, revises, ends and cancels one; and imports a provider's
// charges now; reads the income with its gross, fees and balance under the same tabs, the members of each plan, and the
// money movements with their subscribers by username, narrowed to a kind and a range of dates kept in the URL. A viewer
// without the permission sees why; with MSW playing the API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import { instanceFixture, type Schemas } from "../../test/fixtures";
import { apiBase, operatorHandler, problem, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { deviceToday } from "../costs";
import { costItemFixture } from "./test/handlers";

beforeEach(() => {
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

type Totals = Schemas["CostTotals"];

const monthTotals: Totals = {
  granularity: "month",
  basis: "amortized",
  periods: [
    {
      label: "2026-09",
      starts_on: "2026-09-01",
      ends_on: "2026-09-30",
      totals: [{ amount: "135", currency: "JPY" }],
      breakdown: [],
      estimated: false,
    },
    {
      label: "2026-10",
      starts_on: "2026-10-01",
      ends_on: "2026-10-31",
      totals: [
        { amount: "140", currency: "JPY" },
        { amount: "31.24", currency: "USD" },
      ],
      breakdown: [
        { provider: "registrar", category: "domain", amount: { amount: "140", currency: "JPY" } },
        { provider: "aws", category: "storage", amount: { amount: "31.24", currency: "USD" } },
      ],
      estimated: true,
    },
  ],
  rules: { week_start: "monday", sources: [{ source: "aws", provider: "AWS", time_zone: "UTC" }] },
};

/** Plays the totals: answers each granularity and basis, and records the queries. */
function totalsServer() {
  const queries: Record<string, string>[] = [];
  server.use(
    http.get(`${apiBase}/admin/cost_totals`, ({ request }) => {
      const query = Object.fromEntries(new URL(request.url).searchParams);
      queries.push(query);
      const granularity = query["granularity"] as Totals["granularity"];
      const basis = query["basis"] as Totals["basis"];
      if (granularity === "month") return HttpResponse.json({ ...monthTotals, basis });
      const period =
        granularity === "year"
          ? { label: "2026", starts_on: "2026-01-01", ends_on: "2026-12-31" }
          : { label: "2026-W43", starts_on: "2026-10-19", ends_on: "2026-10-25" };
      return HttpResponse.json({
        ...monthTotals,
        granularity,
        basis,
        periods: [
          {
            ...period,
            totals: [{ amount: basis === "cash" ? "1650" : "1000", currency: "JPY" }],
            breakdown: [],
            estimated: false,
          },
        ],
      });
    }),
  );
  return { queries };
}

/** Plays the cost items: GET lists them, and each write changes them; records the writes. */
function itemsServer(initial: readonly Schemas["CostItem"][]) {
  let items = [...initial];
  const writes: { method: string; path: string; body: unknown }[] = [];
  const record = async (request: Request) => {
    const text = await request.clone().text();
    writes.push({
      method: request.method,
      path: new URL(request.url).pathname.replace("/api/v1", ""),
      body: text === "" ? undefined : JSON.parse(text),
    });
  };
  const change = (id: unknown, edit: (item: Schemas["CostItem"]) => Schemas["CostItem"]) => {
    items = items.map((item) => (item.id === id ? edit(item) : item));
    return HttpResponse.json(items.find((item) => item.id === id));
  };
  server.use(
    http.get(`${apiBase}/admin/cost_items`, () => HttpResponse.json({ items })),
    http.post(`${apiBase}/admin/cost_items`, async ({ request }) => {
      await record(request);
      const { price, memo, ends_on, ...body } = (await request.json()) as Schemas["CostItemCreateRequest"];
      const created = costItemFixture("0199f0e0-0000-7000-8000-0000000000a9", {
        ...body,
        memo: memo ?? "",
        ...(ends_on === undefined ? {} : { ends_on }),
        prices: [{ effective_on: body.starts_on, price, revised_at: "2026-10-22T00:00:00Z" }],
      });
      items = [created, ...items];
      return HttpResponse.json(created);
    }),
    http.patch(`${apiBase}/admin/cost_items/:id`, async ({ request, params }) => {
      await record(request);
      const body = (await request.json()) as Schemas["CostItemLabelRequest"];
      return change(params["id"], (item) => ({ ...item, ...body }));
    }),
    http.post(`${apiBase}/admin/cost_items/:id/price_revisions`, async ({ request, params }) => {
      await record(request);
      const body = (await request.json()) as Schemas["CostPriceRevisionRequest"];
      return change(params["id"], (item) => ({
        ...item,
        prices: [...item.prices, { ...body, revised_at: "2026-10-22T00:00:00Z" }],
      }));
    }),
    http.put(`${apiBase}/admin/cost_items/:id/end`, async ({ request, params }) => {
      await record(request);
      const { ends_on } = (await request.json()) as Schemas["CostItemEndRequest"];
      return change(params["id"], (item) => {
        const { ends_on: _previous, ...rest } = item;
        return ends_on === undefined ? rest : { ...rest, ends_on };
      });
    }),
    http.post(`${apiBase}/admin/cost_items/:id/cancel`, async ({ request, params }) => {
      await record(request);
      items = items.filter((item) => item.id !== params["id"]);
      return new HttpResponse(null, { status: 204 });
    }),
  );
  return { writes };
}

const domain = costItemFixture("0199f0e0-0000-7000-8000-0000000000a1");
const vm = costItemFixture("0199f0e0-0000-7000-8000-0000000000a2", {
  schedule: "months",
  interval: 3,
  starts_on: "2026-01-31",
  name: "Main VM",
  provider: "Cloud",
  category: "compute",
  memo: "the main server",
  prices: [
    { effective_on: "2026-01-31", price: { amount: "12.34", currency: "USD" }, revised_at: "2026-01-31T00:00:00Z" },
  ],
});

describe("the costs screen", () => {
  beforeEach(() => {
    server.use(signedInHandler, operatorHandler(["manage_finance"]));
  });

  test("shows the months amortized by default, with the estimates and the rules, and switches to weeks, years and cash", async () => {
    const totals = totalsServer();
    itemsServer([]);
    const { user, router } = await renderApp({ path: "/admin/finance" });

    expect(await screen.findByRole("heading", { level: 1, name: "Costs" })).toBeInTheDocument();
    const region = screen.getByRole("region", { name: "Totals" });
    expect(within(region).getByRole("radio", { name: /Spread over the period paid for/u })).toBeChecked();
    expect(within(region).getByRole("tab", { name: "Months", selected: true })).toBeInTheDocument();
    const table = within(region).getByRole("table", { name: "Totals by Months, Spread over the period paid for" });
    const rows = within(table).getAllByRole("row");
    // The newest period first; currencies are never converted.
    expect(rows[1]).toHaveTextContent("October 2026");
    expect(rows[1]).toHaveTextContent("¥140 + $31.24");
    expect(rows[1]).toHaveTextContent("Includes estimates");
    expect(rows[2]).toHaveTextContent("September 2026¥135");
    expect(within(region).getByRole("region", { name: "Breakdown of October 2026" })).toHaveTextContent(
      "awsStorage$31.24",
    );
    const rules = within(region).getByRole("region", { name: "How the periods are counted" });
    expect(rules).toHaveTextContent("Weeks run from Monday to Sunday");
    expect(rules).toHaveTextContent("AWS bills by month in the UTC time zone");
    expect(totals.queries[0]).toEqual({
      granularity: "month",
      basis: "amortized",
      from: expect.stringMatching(/-01$/u) as string,
      to: deviceToday(),
    });

    // In one currency, the overview's tiles and chart show the totals, and no other table repeats them.
    await user.click(within(region).getByRole("tab", { name: "Weeks" }));
    const week = await within(region).findByRole("list", { name: /^Oct 19.+ at a glance$/u });
    expect(within(week).getAllByRole("listitem")[0]).toHaveTextContent("Costs this week¥1,000");
    expect(within(region).getByRole("img", { name: "Costs and income by week" })).toHaveAccessibleDescription(
      /^Oct 19.+25, 2026 \(2026-W43\): costs ¥1,000, income ¥0\./u,
    );
    expect(within(region).queryByRole("table", { name: /^Totals by/u })).not.toBeInTheDocument();
    expect(router.state.location.search).toEqual({ granularity: "week" });

    await user.click(within(region).getByRole("radio", { name: /On the payment date/u }));
    expect(await within(region).findByText("¥1,650")).toBeInTheDocument();
    expect(within(region).getByRole("region", { name: "How the periods are counted" })).toHaveTextContent(
      "Every payment counts on its payment date.",
    );
    expect(router.state.location.search).toEqual({ granularity: "week", basis: "cash" });

    await user.click(within(region).getByRole("tab", { name: "Years" }));
    const year = await within(region).findByRole("list", { name: "2026 at a glance" });
    expect(within(year).getAllByRole("listitem")[0]).toHaveTextContent("Costs this year¥1,650");
    expect(totals.queries.at(-1)).toMatchObject({
      granularity: "year",
      basis: "cash",
      from: expect.stringMatching(/-01-01$/u) as string,
    });
  });

  test("records a cost paid every three months after checking the form", async () => {
    totalsServer();
    const items = itemsServer([domain]);
    const { user } = await renderApp({ path: "/admin/finance" });
    const form = await screen.findByRole("region", { name: "Record a cost" });

    await user.click(within(form).getByRole("button", { name: "Record the cost" }));
    expect(within(form).getAllByText("Fill this in.").length).toBeGreaterThanOrEqual(3);
    expect(within(form).getByText("Enter an amount such as 1650 or 12.34.")).toBeInTheDocument();
    expect(items.writes).toEqual([]);

    await user.type(within(form).getByRole("textbox", { name: "Name" }), "Main VM");
    await user.type(within(form).getByRole("textbox", { name: "Provider" }), "Cloud");
    await user.click(within(form).getByRole("radio", { name: "Every few months" }));
    const interval = within(form).getByRole("textbox", { name: "Every how many" });
    await user.clear(interval);
    await user.type(interval, "3");
    await user.type(within(form).getByLabelText(/First payment/u), "2026-01-31");
    await user.type(within(form).getByRole("textbox", { name: "Amount" }), "12.34");
    // The currency starts as that of the newest item.
    const currency = within(form).getByRole("textbox", { name: "Currency" });
    expect(currency).toHaveValue("JPY");
    await user.clear(currency);
    await user.type(currency, "usd");
    await user.click(within(form).getByRole("button", { name: "Record the cost" }));

    expect(await within(form).findByText("Recorded Main VM. The audit log records the change.")).toBeInTheDocument();
    expect(items.writes[0]?.body).toEqual({
      schedule: "months",
      interval: 3,
      starts_on: "2026-01-31",
      name: "Main VM",
      provider: "Cloud",
      category: "other",
      memo: "",
      price: { amount: "12.34", currency: "USD" },
    });
    const list = screen.getByRole("region", { name: "Recorded costs" });
    expect(within(list).getByRole("heading", { level: 3, name: "Main VM" })).toBeInTheDocument();
    expect(within(list).getByText("Every 3 months")).toBeInTheDocument();
  });

  test("edits an item, changes its price, ends it, and deletes one after confirming", async () => {
    totalsServer();
    const items = itemsServer([vm, domain]);
    const { user } = await renderApp({ path: "/admin/finance" });
    const list = await screen.findByRole("region", { name: "Recorded costs" });
    expect(within(list).getByText("the main server")).toHaveAttribute("dir", "auto");

    await user.click(within(list).getByRole("button", { name: "Edit: Main VM" }));
    const edit = await screen.findByRole("dialog", { name: "Edit the cost" });
    const name = within(edit).getByRole("textbox", { name: "Name" });
    await user.clear(name);
    await user.type(name, "Primary VM");
    await user.click(within(edit).getByRole("button", { name: "Save" }));
    expect(await within(list).findByText("Changed Primary VM. The audit log records the change.")).toBeInTheDocument();
    await waitFor(() => {
      expect(within(list).getByRole("heading", { level: 3, name: "Primary VM" })).toHaveFocus();
    });

    await user.click(within(list).getByRole("button", { name: "Change the price: Primary VM" }));
    const revise = await screen.findByRole("dialog", { name: "Change the price of Primary VM" });
    expect(within(revise).getByRole("textbox", { name: "Currency" })).toHaveValue("USD");
    await user.type(within(revise).getByLabelText(/From/u), "2026-07-31");
    await user.type(within(revise).getByRole("textbox", { name: "Amount" }), "15.00");
    await user.click(within(revise).getByRole("button", { name: "Save" }));
    expect(
      await within(list).findByText("Changed the price of Primary VM. The audit log records the change."),
    ).toBeInTheDocument();
    expect(within(list).getByText("$15.00 from Jul 31, 2026")).toBeInTheDocument();

    await user.click(within(list).getByRole("button", { name: "Set the end: Primary VM" }));
    const end = await screen.findByRole("dialog", { name: "End Primary VM" });
    await user.type(within(end).getByLabelText(/Last payment by/u), "2026-12-31");
    await user.click(within(end).getByRole("button", { name: "Save" }));
    expect(
      await within(list).findByText("Set the end of Primary VM. The audit log records the change."),
    ).toBeInTheDocument();
    expect(within(list).getByText("Dec 31, 2026")).toBeInTheDocument();

    await user.click(within(list).getByRole("button", { name: "Delete: example.com" }));
    const confirm = await screen.findByRole("alertdialog", { name: "Delete example.com?" });
    await user.click(within(confirm).getByRole("button", { name: "Delete" }));
    expect(await within(list).findByText("Deleted example.com. The audit log records the change.")).toBeInTheDocument();
    expect(within(list).queryByRole("heading", { name: "example.com" })).not.toBeInTheDocument();

    expect(items.writes.map((write) => `${write.method} ${write.path}`)).toEqual([
      `PATCH /admin/cost_items/${vm.id}`,
      `POST /admin/cost_items/${vm.id}/price_revisions`,
      `PUT /admin/cost_items/${vm.id}/end`,
      `POST /admin/cost_items/${domain.id}/cancel`,
    ]);
    expect(items.writes[1]?.body).toEqual({ effective_on: "2026-07-31", price: { amount: "15.00", currency: "USD" } });
    expect(items.writes[2]?.body).toEqual({ ends_on: "2026-12-31" });
  });

  test("keeps a rejected change in its dialog", async () => {
    totalsServer();
    itemsServer([vm]);
    server.use(
      http.post(`${apiBase}/admin/cost_items/:id/price_revisions`, () =>
        problem(422, "about:blank", "invalid value: price"),
      ),
    );
    const { user } = await renderApp({ path: "/admin/finance" });
    const list = await screen.findByRole("region", { name: "Recorded costs" });

    await user.click(within(list).getByRole("button", { name: "Change the price: Main VM" }));
    const revise = await screen.findByRole("dialog", { name: "Change the price of Main VM" });
    await user.type(within(revise).getByLabelText(/From/u), "2026-07-31");
    await user.type(within(revise).getByRole("textbox", { name: "Amount" }), "1.234");
    await user.click(within(revise).getByRole("button", { name: "Save" }));
    expect(await within(revise).findByRole("alert")).toHaveTextContent(
      "Could not change the cost: invalid value: price",
    );
  });

  test("shows the state of the billing imports and imports now once an hour", async () => {
    totalsServer();
    itemsServer([]);
    const runs: string[] = [];
    server.use(
      http.get(`${apiBase}/admin/billing_imports`, () =>
        HttpResponse.json({
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
                occurred_at: "2026-10-21T01:00:00Z",
              },
              last_success_at: "2026-10-20T01:00:00Z",
              requests_today: 4,
              requests_this_month: 40,
              max_requests_per_day: 4,
              max_requests_per_month: 120,
              bytes_billed_this_month: 0,
              months: [{ month: "2026-10", kind: "estimate", totals: [{ amount: "31.24", currency: "USD" }] }],
            },
          ],
        } satisfies Schemas["BillingImportList"]),
      ),
      http.post(`${apiBase}/admin/billing_imports/:source/runs`, ({ params }) => {
        runs.push(String(params["source"]));
        if (runs.length > 1) return problem(429, "about:blank", "Too Many Requests");
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const { user } = await renderApp({ path: "/admin/finance" });
    const region = await screen.findByRole("region", { name: "Billing imports" });

    expect(within(region).getByRole("heading", { level: 3, name: "AWS" })).toBeInTheDocument();
    expect(region).toHaveTextContent("Skipped: the cap of billed requests is reached.");
    expect(region).toHaveTextContent("4 of 4");
    expect(within(region).getByRole("list", { name: "Billing months of AWS" })).toHaveTextContent(
      "October 2026Estimate$31.24",
    );

    await user.click(within(region).getByRole("button", { name: "Import now: AWS" }));
    expect(await within(region).findByText("Started an import of AWS. The audit log records it.")).toBeInTheDocument();
    await user.click(within(region).getByRole("button", { name: "Import now: AWS" }));
    expect(await within(region).findByRole("alert")).toHaveTextContent(
      "An import of this provider ran in the last hour.",
    );
    expect(runs).toEqual(["aws", "aws"]);
  });

  test.each([
    { method: "budget", note: true },
    { method: "bigquery", note: false },
  ] as const)(
    "says whether the Google Cloud figures are an approximation for the $method method",
    async ({ method, note }) => {
      totalsServer();
      itemsServer([]);
      server.use(
        http.get(`${apiBase}/admin/billing_imports`, () =>
          HttpResponse.json({
            items: [
              {
                source: "gcp",
                provider: "Google Cloud",
                time_zone: "America/Los_Angeles",
                method,
                requests_today: 1,
                requests_this_month: 3,
                bytes_billed_this_month: 2048,
                months: [{ month: "2026-10", kind: "estimate", totals: [{ amount: "12.50", currency: "USD" }] }],
              },
            ],
          } satisfies Schemas["BillingImportList"]),
        ),
      );
      await renderApp({ path: "/admin/finance" });
      const region = await screen.findByRole("region", { name: "Billing imports" });

      expect(within(region).getByRole("heading", { level: 3, name: "Google Cloud" })).toBeInTheDocument();
      const approximation =
        "An approximation from the budget notifications: totals based on estimates, without a breakdown by service.";
      if (note) {
        expect(region).toHaveTextContent(approximation);
        expect(region).not.toHaveTextContent("Bytes billed this month");
      } else {
        expect(region).not.toHaveTextContent(approximation);
        expect(region).toHaveTextContent("Bytes billed this month");
      }
    },
  );
});

describe("what the costs screen says of their publication", () => {
  function serveVisibility(visibility: Schemas["Instance"]["finance_visibility"]) {
    server.use(
      http.get(`${apiBase}/instance`, () => HttpResponse.json({ ...instanceFixture, finance_visibility: visibility })),
    );
  }

  test("shows what the server publishes, and links to the server settings for a viewer who may change it", async () => {
    server.use(signedInHandler, operatorHandler(["manage_finance", "manage_settings"]));
    serveVisibility("breakdown");
    await renderApp({ path: "/admin/finance" });

    expect(
      await screen.findByText(
        "The about page publishes the totals of these costs and their breakdown by provider and category.",
      ),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Change in the server settings" })).toHaveAttribute(
      "href",
      "/admin/settings",
    );
  });

  test("shows what the holders of view_finance see", async () => {
    server.use(
      signedInHandler,
      operatorHandler(["manage_finance"]),
      http.get(`${apiBase}/finance/cost_totals`, () =>
        HttpResponse.json({
          ...monthTotals,
          periods: monthTotals.periods.map((period) => ({ ...period, items: [] })),
          viewer: { item_names: true, item_memos: false, income: "plans" },
        } satisfies Schemas["FinanceCostTotals"]),
      ),
    );
    serveVisibility("private");
    await renderApp({ path: "/admin/finance" });

    expect(
      await screen.findByText(
        "People who may read the finances see the names of the items, do not see their memos, and see the income with the members of each plan.",
      ),
    ).toBeInTheDocument();
  });

  test("shows that nothing is published, without the link for a viewer who may not change it", async () => {
    server.use(signedInHandler, operatorHandler(["manage_finance"]));
    serveVisibility("private");
    await renderApp({ path: "/admin/finance" });

    expect(await screen.findByText("These costs are not published.")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Change in the server settings" })).not.toBeInTheDocument();
  });
});

describe("the income on the costs screen", () => {
  beforeEach(() => {
    server.use(signedInHandler);
  });

  const movement: Schemas["IncomeMovement"] = {
    id: "0199f0e0-0000-7000-8000-00000000bb01",
    provider: "stripe",
    provider_id: "txn_1",
    kind: "payment",
    subscription_id: "0199f0e0-0000-7000-8000-00000000cc01",
    account_id: "0199f0e0-0000-7000-8000-00000000dd01",
    username: "alice",
    occurred_at: "2026-10-10T03:00:00Z",
    occurred_on: "2026-10-10",
    covers_starts_on: "2026-10-10",
    covers_ends_on: "2027-10-09",
    gross: { amount: "12000", currency: "JPY" },
    fee: { amount: "432", currency: "JPY" },
    net: { amount: "11568", currency: "JPY" },
    recorded_at: "2026-10-10T03:01:00Z",
  };

  /** A refund whose subscriber's account has been deleted: the subscription stays, the account and its username do not. */
  const orphanRefund: Schemas["IncomeMovement"] = {
    id: "0199f0e0-0000-7000-8000-00000000bb03",
    provider: "stripe",
    provider_id: "txn_refund",
    kind: "refund",
    parent_provider_id: "txn_0",
    subscription_id: "0199f0e0-0000-7000-8000-00000000cc02",
    occurred_at: "2026-10-09T00:00:00Z",
    occurred_on: "2026-10-09",
    gross: { amount: "-500", currency: "JPY" },
    fee: { amount: "0", currency: "JPY" },
    net: { amount: "-500", currency: "JPY" },
    recorded_at: "2026-10-09T00:01:00Z",
  };

  /** Plays the money movements, narrowed by kind and dates as the server does, and records the queries but the page size. */
  function serveMovements(items: readonly Schemas["IncomeMovement"][]) {
    const queries: Record<string, string>[] = [];
    server.use(
      http.get(`${apiBase}/admin/income_movements`, ({ request }) => {
        const { limit: _limit, ...query } = Object.fromEntries(new URL(request.url).searchParams);
        queries.push(query);
        const { kind, from, to } = query;
        return HttpResponse.json({
          items: items.filter(
            (item) =>
              (kind === undefined || item.kind === kind) &&
              (from === undefined || item.occurred_on >= from) &&
              (to === undefined || item.occurred_on <= to),
          ),
        } satisfies Schemas["IncomeMovementPage"]);
      }),
    );
    return { queries };
  }

  function serveIncome() {
    const queries: Record<string, string>[] = [];
    server.use(
      http.get(`${apiBase}/admin/income_totals`, ({ request }) => {
        const query = Object.fromEntries(new URL(request.url).searchParams);
        queries.push(query);
        return HttpResponse.json({
          granularity: query["granularity"] as Schemas["CostGranularity"],
          basis: query["basis"] as Schemas["CostBasis"],
          periods: [
            {
              label: "2026-10",
              starts_on: "2026-10-01",
              ends_on: "2026-10-31",
              gross: [{ amount: "12000", currency: "JPY" }],
              fee: [{ amount: "432", currency: "JPY" }],
              net: [{ amount: "11568", currency: "JPY" }],
              balance: [{ amount: "11428", currency: "JPY" }],
            },
          ],
          plans: [{ plan_id: "0199f0e0-0000-7000-8000-00000000aa01", name: "Coffee", members: 1 }],
          rules: {
            week_start: "monday",
            providers: [{ provider: "stripe", time_zone: "Asia/Tokyo" }],
            amortize_after_days: 31,
          },
        } satisfies Schemas["AdminIncomeTotals"]);
      }),
      http.get(`${apiBase}/admin/income_movements`, () =>
        HttpResponse.json({
          items: [
            movement,
            {
              id: "0199f0e0-0000-7000-8000-00000000bb02",
              provider: "stripe",
              provider_id: "txn_fee",
              kind: "fee",
              occurred_at: "2026-10-11T00:00:00Z",
              occurred_on: "2026-10-11",
              gross: { amount: "0", currency: "JPY" },
              fee: { amount: "84", currency: "JPY" },
              net: { amount: "-84", currency: "JPY" },
              recorded_at: "2026-10-11T00:01:00Z",
            },
          ],
        } satisfies Schemas["IncomeMovementPage"]),
      ),
    );
    return { queries };
  }

  test("shows the gross, the fees, the net and the balance by period with the members of each plan, under the tabs of the costs", async () => {
    server.use(operatorHandler(["manage_finance"]));
    totalsServer();
    itemsServer([]);
    const income = serveIncome();
    const { user } = await renderApp({ path: "/admin/finance" });

    const region = await screen.findByRole("region", { name: "Totals" });
    const section = within(region).getByRole("region", { name: "Income" });
    const table = within(section).getByRole("table", { name: "Income by Months, Spread over the period paid for" });
    expect(
      within(table)
        .getAllByRole("columnheader")
        .map((cell) => cell.textContent),
    ).toEqual(["Period", "Gross", "Fees", "Income after fees", "Balance"]);
    expect(within(table).getAllByRole("row")[1]).toHaveTextContent("October 2026¥12,000¥432¥11,568+¥11,428");
    expect(within(section).getByRole("heading", { level: 4, name: "Members of each plan" })).toBeInTheDocument();
    expect(within(section).getByRole("region", { name: "Members of each plan" })).toHaveTextContent("Coffee1 member");
    const rules = within(section).getByRole("region", { name: "How the income is counted" });
    expect(rules).toHaveTextContent("stripe dates its payments in the Asia/Tokyo time zone.");
    expect(rules).toHaveTextContent("A payment for more than 31 days");
    expect(income.queries[0]).toEqual({
      granularity: "month",
      basis: "amortized",
      from: expect.stringMatching(/-01$/u) as string,
      to: deviceToday(),
    });

    await user.click(within(region).getByRole("radio", { name: /On the payment date/u }));
    expect(
      await within(section).findByRole("table", { name: "Income by Months, On the payment date" }),
    ).toBeInTheDocument();
    expect(income.queries.at(-1)).toMatchObject({ granularity: "month", basis: "cash" });
  });

  test("lists the money movements with their subscribers by username, linked for a viewer who may open the accounts", async () => {
    server.use(operatorHandler(["manage_finance", "manage_accounts"]));
    serveIncome();
    await renderApp({ path: "/admin/finance" });

    const region = await screen.findByRole("region", { name: "Payments and refunds" });
    const rows = within(region).getAllByRole("row").slice(1);
    expect(within(rows[0] as HTMLElement).getByRole("rowheader")).toHaveTextContent("Payment");
    expect(rows[0]).toHaveTextContent("¥11,568");
    expect(rows[0]).toHaveTextContent("¥12,000");
    expect(rows[0]).toHaveTextContent("¥432");
    expect(within(rows[0] as HTMLElement).getByRole("link", { name: "@alice" })).toHaveAttribute(
      "href",
      `/moderation/accounts/${String(movement.account_id)}`,
    );
    expect(within(rows[0] as HTMLElement).getByText("@alice")).toHaveAttribute("dir", "ltr");
    expect(rows[0]).not.toHaveTextContent(String(movement.account_id));
    expect(within(rows[0] as HTMLElement).getByText("txn_1")).toHaveAttribute("dir", "ltr");
    expect(rows[1]).toHaveTextContent("Provider fee");
    expect(rows[1]).toHaveTextContent("-¥84");
    expect(rows[1]).toHaveTextContent("None");
  });

  test("shows the username without a link to a viewer who may not open the accounts, and a deleted subscriber's account as deleted", async () => {
    server.use(operatorHandler(["manage_finance"]));
    serveIncome();
    serveMovements([movement, orphanRefund]);
    await renderApp({ path: "/admin/finance" });

    const region = await screen.findByRole("region", { name: "Payments and refunds" });
    const rows = within(region).getAllByRole("row").slice(1);
    expect(rows[0]).toHaveTextContent("@alice");
    expect(within(rows[0] as HTMLElement).queryByRole("link")).not.toBeInTheDocument();
    expect(within(rows[1] as HTMLElement).getByRole("rowheader")).toHaveTextContent("Refund");
    expect(rows[1]).toHaveTextContent("-¥500");
    expect(rows[1]).toHaveTextContent("Deleted account");
    expect(rows[1]).toHaveTextContent("Of the payment txn_0");
  });

  test("narrows the money movements to a kind and a range of dates, kept in the URL", async () => {
    server.use(operatorHandler(["manage_finance", "manage_accounts"]));
    serveIncome();
    const movements = serveMovements([movement, orphanRefund]);
    const { user, router } = await renderApp({ path: "/admin/finance?basis=cash" });

    const region = await screen.findByRole("region", { name: "Payments and refunds" });
    expect(within(region).getAllByRole("row")).toHaveLength(3);
    expect(movements.queries[0]).toEqual({});

    await user.click(within(region).getByRole("button", { name: /Kind/u }));
    await user.click(await screen.findByRole("option", { name: "Refund" }));
    await waitFor(() => {
      expect(router.state.location.search).toEqual({ basis: "cash", kind: "refund" });
    });
    expect(movements.queries.at(-1)).toEqual({ kind: "refund" });
    await waitFor(() => {
      expect(within(region).getAllByRole("row")).toHaveLength(2);
    });

    // A last date before the first is not applied.
    const from = within(region).getByLabelText("Counted from");
    const to = within(region).getByLabelText("Counted until");
    await user.type(from, "2026-10-10");
    await user.type(to, "2026-10-01");
    expect(within(region).getByText("The last date is before the first date.")).toBeInTheDocument();
    await user.click(within(region).getByRole("button", { name: "Show these dates" }));
    expect(router.state.location.search).toEqual({ basis: "cash", kind: "refund" });

    await user.clear(to);
    await user.type(to, "2026-10-31");
    await user.click(within(region).getByRole("button", { name: "Show these dates" }));
    await waitFor(() => {
      expect(router.state.location.search).toEqual({
        basis: "cash",
        kind: "refund",
        from: "2026-10-10",
        to: "2026-10-31",
      });
    });
    expect(movements.queries.at(-1)).toEqual({ kind: "refund", from: "2026-10-10", to: "2026-10-31" });
    // Nothing matches: the empty list offers to clear the filters, which moves focus to the kind select.
    expect(
      await within(region).findByRole("heading", { level: 3, name: "No movements match the filters" }),
    ).toBeInTheDocument();
    const clears = within(region).getAllByRole("button", { name: "Clear the filters" });
    // The one above the filters, and the one of the empty list.
    expect(clears).toHaveLength(2);
    await user.click(clears[1] as HTMLElement);
    await waitFor(() => {
      expect(router.state.location.search).toEqual({ basis: "cash" });
    });
    expect(within(region).getByRole("button", { name: /Kind/u })).toHaveFocus();
    expect(from).toHaveValue("");
    expect(await within(region).findAllByRole("row")).toHaveLength(3);
  });

  test("reads the movements' filter from the URL and ignores what it does not know", async () => {
    server.use(operatorHandler(["manage_finance"]));
    serveIncome();
    const movements = serveMovements([movement, orphanRefund]);
    await renderApp({ path: "/admin/finance?kind=payment&from=2026-10-01&to=2026-13-40" });

    const region = await screen.findByRole("region", { name: "Payments and refunds" });
    expect(movements.queries[0]).toEqual({ kind: "payment", from: "2026-10-01" });
    expect(within(region).getAllByRole("row")).toHaveLength(2);
    expect(within(region).getByLabelText("Counted from")).toHaveValue("2026-10-01");
    expect(within(region).getByLabelText("Counted until")).toHaveValue("");
    expect(within(region).getByRole("button", { name: "Clear the filters" })).toBeInTheDocument();
  });

  test("shows what the server publishes of the income", async () => {
    server.use(
      operatorHandler(["manage_finance"]),
      http.get(`${apiBase}/instance`, () => HttpResponse.json({ ...instanceFixture, income_visibility: "plans" })),
    );
    await renderApp({ path: "/admin/finance" });

    expect(
      await screen.findByText("The about page publishes the totals of the income and the members of each plan."),
    ).toBeInTheDocument();
  });
});

describe("the costs screen without the permission", () => {
  test("says why it shows nothing", async () => {
    server.use(signedInHandler, operatorHandler(["manage_settings"]));
    await renderApp({ path: "/admin/finance" });

    expect(await screen.findByRole("heading", { level: 1, name: "Costs" })).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Totals" })).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Record a cost" })).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Payments and refunds" })).not.toBeInTheDocument();
  });
});
