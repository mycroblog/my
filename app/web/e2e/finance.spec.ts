// SPDX-License-Identifier: AGPL-3.0-only
// The costs screen in every width × locale project (ADR-0067, ADR-0068, ADR-0027): an administrator with
// manage_finance sees the totals by month amortized by default, switches to weeks and to payment dates, records a cost
// paid every year, and deletes one after its dialog; the billing imports show their state; the money movements show
// their subscribers by username and narrow to a kind with the keyboard, into the URL. Each step passes axe
// without scrolling sideways. `vite preview` has no API, so the test plays the server with page.route; names change
// with the locale, so the elements are found by role and by the names in the project's locale catalog.
import type { Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";

type Schemas = components["schemas"];
/** The client message `key` in the project's locale, with each `{name}` of `values` replaced. */
function message(key: string, values: Record<string, string> = {}): string {
  const locale = test.info().project.use.locale ?? "en";
  const catalog = readCatalog(locale);
  const text = catalog[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${locale}`);
  return Object.entries(values).reduce((out, [name, value]) => out.replace(`{${name}}`, value), text);
}

/** The signed-in administrator, as GET /accounts/verify_credentials answers. */
const account = {
  id: "0192f0c4-7a8e-7000-8000-000000000001",
  username: "alice",
  acct: "alice",
  display_name: "Alice",
  note: "",
  fields: [],
  emojis: [],
  url: "https://my.example/@alice",
  locked: false,
  bot: false,
  created_at: "2026-10-01T00:00:00Z",
  discoverable: true,
  indexable: true,
  source: { note: "", fields: [] },
  posting: { visibility_ceiling: "public", visibilities: ["public", "local", "unlisted", "private", "direct"] },
  permissions: ["manage_finance"],
};

/** A monthly server plan with a long provider name and a memo in Arabic, which reads right to left. */
const server: Schemas["CostItem"] = {
  id: "0192f0c4-7a8e-7000-8000-0000000000a1",
  schedule: "months",
  interval: 1,
  starts_on: "2026-01-31",
  name: "Main server",
  provider: "a-very-long-cloud-provider-name-that-does-not-wrap-on-its-own.example",
  category: "compute",
  memo: "الخادم الرئيسي",
  prices: [
    { effective_on: "2026-01-31", price: { amount: "12.34", currency: "USD" }, revised_at: "2026-01-31T00:00:00Z" },
  ],
  created_at: "2026-01-31T00:00:00Z",
};

/** The totals of a query: one period of the granularity asked for, with yen and dollars and an estimate. */
function totals(url: URL): Schemas["CostTotals"] {
  const granularity = (url.searchParams.get("granularity") ?? "month") as Schemas["CostGranularity"];
  const basis = (url.searchParams.get("basis") ?? "amortized") as Schemas["CostBasis"];
  const period =
    granularity === "week"
      ? { label: "2026-W43", starts_on: "2026-10-19", ends_on: "2026-10-25" }
      : granularity === "year"
        ? { label: "2026", starts_on: "2026-01-01", ends_on: "2026-12-31" }
        : { label: "2026-10", starts_on: "2026-10-01", ends_on: "2026-10-31" };
  return {
    granularity,
    basis,
    periods: [
      {
        ...period,
        totals: [
          { amount: basis === "cash" ? "1650" : "140", currency: "JPY" },
          { amount: "12.34", currency: "USD" },
        ],
        breakdown: [{ provider: "registrar", category: "domain", amount: { amount: "140", currency: "JPY" } }],
        estimated: true,
      },
    ],
    rules: { week_start: "monday", sources: [{ source: "aws", provider: "AWS", time_zone: "UTC" }] },
  };
}

/** The income of the periods of a query, as the administrators see it: the last one with a yearly payment. */
function incomeTotals(url: URL): Schemas["AdminIncomeTotals"] {
  const cost = totals(url);
  return {
    granularity: cost.granularity,
    basis: cost.basis,
    periods: cost.periods.map((period, index) => ({
      label: period.label,
      starts_on: period.starts_on,
      ends_on: period.ends_on,
      gross: index === cost.periods.length - 1 ? [{ amount: "12000", currency: "JPY" }] : [],
      fee: index === cost.periods.length - 1 ? [{ amount: "432", currency: "JPY" }] : [],
      net: index === cost.periods.length - 1 ? [{ amount: "11568", currency: "JPY" }] : [],
      balance: index === cost.periods.length - 1 ? [{ amount: "11428", currency: "JPY" }] : [],
    })),
    plans: [
      {
        plan_id: "0192f0c4-7a8e-7000-8000-0000000000c1",
        name: "A plan with a rather long name that has to wrap on the narrowest screens",
        members: 1,
      },
    ],
    rules: {
      week_start: "monday",
      providers: [{ provider: "stripe", time_zone: "Asia/Tokyo" }],
      amortize_after_days: 31,
    },
  };
}

/** A yearly payment of a supporter plan and a provider fee, as GET /admin/income_movements lists them. */
const movements: Schemas["IncomeMovement"][] = [
  {
    id: "0192f0c4-7a8e-7000-8000-0000000000d1",
    provider: "stripe",
    provider_id: "txn_1PaymentWithARatherLongIdentifierThatDoesNotWrap",
    kind: "payment",
    subscription_id: "0192f0c4-7a8e-7000-8000-0000000000d2",
    account_id: "0192f0c4-7a8e-7000-8000-0000000000d3",
    username: "a_supporter_with_a_rather_long_username_that_does_not_wrap",
    occurred_at: "2026-10-10T03:00:00Z",
    occurred_on: "2026-10-10",
    covers_starts_on: "2026-10-10",
    covers_ends_on: "2027-10-09",
    gross: { amount: "12000", currency: "JPY" },
    fee: { amount: "432", currency: "JPY" },
    net: { amount: "11568", currency: "JPY" },
    recorded_at: "2026-10-10T03:01:00Z",
  },
  {
    id: "0192f0c4-7a8e-7000-8000-0000000000d4",
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
];

const awsImport: Schemas["BillingImport"] = {
  source: "aws",
  provider: "AWS",
  time_zone: "UTC",
  method: "cost_explorer",
  last_run: {
    outcome: "succeeded",
    error_kind: "none",
    requests: 1,
    bytes_billed: 0,
    occurred_at: "2026-10-21T01:00:00Z",
  },
  last_success_at: "2026-10-21T01:00:00Z",
  requests_today: 1,
  requests_this_month: 21,
  max_requests_per_day: 4,
  max_requests_per_month: 120,
  bytes_billed_this_month: 0,
  months: [{ month: "2026-10", kind: "estimate", totals: [{ amount: "31.24", currency: "USD" }] }],
};

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({
    status,
    contentType: "application/json",
    headers: { "Cache-Control": "no-store" },
    body: JSON.stringify(body),
  });
}

/** Plays the API of the administrator: the costs change with each write, which `sent` records. */
async function serveApi(page: Page) {
  const locale = test.info().project.use.locale ?? "en";
  const sent: string[] = [];
  let items = [server];
  await page.route("**/oauth/token/refresh", (route) =>
    json(route, {
      access_token: "e2e-token",
      token_type: "Bearer",
      expires_in: 3600,
      scope: "read write follow admin",
    }),
  );
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^\/api\/v1/, "");
    const key = `${request.method()} ${path}`;
    if (key === "GET /instance") {
      return json(route, {
        domain: "my.example",
        name: "my example",
        version: "0.0.0",
        source_url: "https://github.com/mycroblog/my",
        registrations: { mode: "approval" },
        federation: { mode: "both" },
        limits: { max_post_characters: 500, require_media_description: false },
        timelines: { local: "everyone", federated: "everyone" },
        rules: [],
      });
    }
    if (key === "GET /accounts/verify_credentials") return json(route, account);
    if (key === "GET /preferences") {
      return json(route, {
        locale,
        time_zone: "UTC",
        default_visibility: "public",
        default_post_language: "und",
        require_media_description: false,
        timeline_languages: [],
      });
    }
    if (key === "GET /notifications/marker") return json(route, { last_read_id: "0", unread_count: 0 });
    if (key === "GET /admin/counts")
      return json(route, { open_reports: 0, pending_registrations: 0, pending_appeals: 0 });
    if (key === "GET /admin/audit_log") return json(route, { items: [] });
    if (key === "GET /admin/cost_totals") {
      sent.push(`totals ${url.searchParams.get("granularity") ?? ""} ${url.searchParams.get("basis") ?? ""}`);
      return json(route, totals(url));
    }
    if (key === "GET /finance/cost_totals") {
      const viewer: Schemas["FinanceViewerSettings"] = { item_names: true, item_memos: false, income: "totals" };
      return json(route, {
        ...totals(url),
        periods: totals(url).periods.map((period) => ({ ...period, items: [] })),
        viewer,
      });
    }
    if (key === "GET /admin/cost_items") return json(route, { items });
    if (key === "GET /admin/billing_imports") return json(route, { items: [awsImport] });
    if (key === "GET /admin/income_totals") return json(route, incomeTotals(url));
    if (key === "GET /admin/income_movements") {
      const kind = url.searchParams.get("kind");
      return json(route, { items: movements.filter((movement) => kind === null || movement.kind === kind) });
    }
    if (key === "POST /admin/cost_items") {
      const { price, memo, ends_on, ...body } = request.postDataJSON() as Schemas["CostItemCreateRequest"];
      sent.push(`create ${body.schedule} ${price.amount} ${price.currency}`);
      const created: Schemas["CostItem"] = {
        ...body,
        id: "0192f0c4-7a8e-7000-8000-0000000000a2",
        memo: memo ?? "",
        ...(ends_on === undefined ? {} : { ends_on }),
        prices: [{ effective_on: body.starts_on, price, revised_at: "2026-10-21T00:00:00Z" }],
        created_at: "2026-10-21T00:00:00Z",
      };
      items = [created, ...items];
      return json(route, created);
    }
    if (/^POST \/admin\/cost_items\/[^/]+\/cancel$/.test(key)) {
      sent.push("cancel");
      items = items.filter((item) => !path.includes(item.id));
      return route.fulfill({ status: 204 });
    }
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  return sent;
}

test.describe("costs", () => {
  test("switches the totals, records a yearly cost and deletes one after its dialog", async ({
    page,
    makeAxeBuilder,
  }) => {
    const sent = await serveApi(page);
    await page.goto("/admin/finance");
    const main = page.getByRole("main");
    const totalsRegion = main.getByRole("region", { name: message("admin.finance.totals.title"), exact: true });
    // The costs' table, then the income's.
    await expect(totalsRegion.getByRole("table")).toHaveCount(2);
    const basis = totalsRegion.getByRole("radiogroup", { name: message("costs.basis"), exact: true });
    await expect(basis.getByRole("radio", { name: message("costs.basis.amortized"), exact: true })).toBeChecked();
    await expect(totalsRegion.getByText(message("costs.totals.estimated"), { exact: true })).toBeVisible();
    await expect(totalsRegion.getByText(message("costs.rules.week_start"), { exact: true })).toBeVisible();
    await expect(main.getByText("الخادم الرئيسي")).toHaveAttribute("dir", "auto");
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    // The tabs follow the APG with automatic activation: the arrow key selects the weeks.
    await totalsRegion.getByRole("tab", { name: message("costs.granularity.month"), exact: true }).focus();
    await page.keyboard.press(test.info().project.use.locale === "ar-XB" ? "ArrowRight" : "ArrowLeft");
    await expect(
      totalsRegion.getByRole("tab", { name: message("costs.granularity.week"), exact: true }),
    ).toHaveAttribute("aria-selected", "true");
    await expect(
      totalsRegion
        .getByRole("table")
        .first()
        .getByRole("row", { name: /2026-W43/u }),
    ).toBeVisible();
    // React Aria hides the native radio under its drawing; the label is what the user presses.
    await basis.getByText(message("costs.basis.cash"), { exact: true }).click();
    await expect(basis.getByRole("radio", { name: message("costs.basis.cash"), exact: true })).toBeChecked();
    await expect(totalsRegion.getByText(message("costs.rules.cash"), { exact: true })).toBeVisible();
    await expect(page).toHaveURL(/granularity=week/u);
    await expect(page).toHaveURL(/basis=cash/u);
    expect(sent).toContain("totals week cash");
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    const form = main.getByRole("region", { name: message("admin.finance.new.title"), exact: true });
    await form.getByRole("textbox", { name: message("admin.finance.name"), exact: true }).fill("example.com");
    await form.getByRole("textbox", { name: message("admin.finance.provider"), exact: true }).fill("Registrar");
    const schedule = form.getByRole("radiogroup", { name: message("admin.finance.schedule"), exact: true });
    await schedule.getByText(message("admin.finance.schedule.years"), { exact: true }).click();
    await expect(
      schedule.getByRole("radio", { name: message("admin.finance.schedule.years"), exact: true }),
    ).toBeChecked();
    await form.getByLabel(message("admin.finance.starts_on"), { exact: true }).fill("2026-04-10");
    await form.getByRole("textbox", { name: message("admin.finance.amount"), exact: true }).fill("1650");
    await form.getByRole("textbox", { name: message("admin.finance.currency"), exact: true }).fill("JPY");
    await form.getByRole("button", { name: message("admin.finance.create"), exact: true }).click();
    const list = main.getByRole("region", { name: message("admin.finance.list.title"), exact: true });
    await expect(list.getByRole("heading", { level: 3, name: "example.com" })).toBeVisible();
    expect(sent).toContain("create years 1650 JPY");
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    await list
      .getByRole("button", { name: message("admin.finance.cancel_one", { name: "example.com" }), exact: true })
      .click();
    const dialog = page.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await dialog.getByRole("button", { name: message("admin.finance.cancel_item"), exact: true }).click();
    await expect(list.getByRole("heading", { level: 3, name: "example.com" })).toBeHidden();
    expect(sent).toContain("cancel");

    const imports = main.getByRole("region", { name: message("admin.finance.imports.title"), exact: true });
    await expect(imports.getByRole("heading", { level: 3, name: "AWS" })).toBeVisible();
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
  });
});

test("the income on the costs screen, with its money movements", async ({ page, makeAxeBuilder }) => {
  await serveApi(page);
  await page.goto("/admin/finance");
  const main = page.getByRole("main");
  const totalsRegion = main.getByRole("region", { name: message("admin.finance.totals.title"), exact: true });
  const income = totalsRegion.getByRole("region", { name: message("income.title"), exact: true });
  // The period, the gross, the fees, the income after fees and the balance.
  await expect(income.getByRole("table").getByRole("columnheader")).toHaveCount(5);
  await expect(
    income.getByText("A plan with a rather long name that has to wrap on the narrowest screens"),
  ).toHaveAttribute("dir", "auto");
  const movementsRegion = main.getByRole("region", { name: message("admin.finance.movements.title"), exact: true });
  await expect(movementsRegion.getByRole("row")).toHaveCount(3);
  await expect(movementsRegion.getByText("txn_1PaymentWithARatherLongIdentifierThatDoesNotWrap")).toHaveAttribute(
    "dir",
    "ltr",
  );
  await expect(
    movementsRegion.getByText("@a_supporter_with_a_rather_long_username_that_does_not_wrap"),
  ).toHaveAttribute("dir", "ltr");
  await expectNoAxeViolations(makeAxeBuilder);
  await expectNoHorizontalScroll(page);

  // The kind select follows the APG select-only combobox: the keyboard opens it and picks a kind, which goes into the URL.
  await movementsRegion.getByRole("button", { name: message("admin.finance.movements.filter.kind") }).focus();
  await page.keyboard.press("Enter");
  await page.getByRole("option", { name: message("admin.finance.movements.kind.fee"), exact: true }).press("Enter");
  await expect(page).toHaveURL(/kind=fee/u);
  await expect(movementsRegion.getByRole("row")).toHaveCount(2);
  await expect(
    movementsRegion.getByRole("button", { name: message("admin.finance.movements.filter.clear"), exact: true }),
  ).toBeVisible();
  await expectNoAxeViolations(makeAxeBuilder);
  await expectNoHorizontalScroll(page);
});
