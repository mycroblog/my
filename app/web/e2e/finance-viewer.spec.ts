// SPDX-License-Identifier: AGPL-3.0-only
// The finances page in every width × locale project (ADR-0070 §5, ADR-0027): a user whose roles have view_finance and
// no staff permission finds Finances in the rail from md and in the menu's sheet below it, with neither Moderation
// nor Administration, and reads the totals with their breakdown and the lines of the items that the server shows (a
// long name and a memo in Arabic, which reads right to left), and the income with the balance and the members of a
// plan under the same tabs, passing axe without scrolling sideways. The app asks
// for no admin data. `vite preview` has no API, so the test plays the server with page.route; names change with the
// locale, so the elements are found by role, by address and by the names in the project's locale catalog.
import type { Page, Route } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";
import { readCatalog } from "./catalog.ts";

type Schemas = components["schemas"];
/** The client message `key` in the project's locale. */
function message(key: string): string {
  const locale = test.info().project.use.locale ?? "en";
  const catalog = readCatalog(locale);
  const text = catalog[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key} in ${locale}`);
  return text;
}

/** The signed-in supporter, as GET /accounts/verify_credentials answers: view_finance and nothing of the staff's. */
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
  permissions: ["view_finance"],
};

const longName = "a-very-long-name-of-the-main-server-that-does-not-wrap-on-its-own.example";

/** The totals of a query as a server that shows the names and the memos answers them: one period with two items. */
function totals(url: URL): Schemas["FinanceCostTotals"] {
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
          { amount: "1650", currency: "JPY" },
          { amount: "12.34", currency: "USD" },
        ],
        breakdown: [
          { provider: "cloud", category: "compute", amount: { amount: "12.34", currency: "USD" } },
          { provider: "registrar", category: "domain", amount: { amount: "1650", currency: "JPY" } },
        ],
        estimated: false,
        items: [
          { provider: "cloud", category: "compute", amount: { amount: "12.34", currency: "USD" }, name: longName, memo: "الخادم الرئيسي" },
          { provider: "registrar", category: "domain", amount: { amount: "1650", currency: "JPY" }, name: "example.com" },
        ],
      },
    ],
    rules: { week_start: "monday", sources: [] },
    viewer: { item_names: true, item_memos: true, income: "plans" },
  };
}

/** The income of the periods of a query as a server that shows the plans answers it: the net, the balance and a plan. */
function incomeTotals(url: URL): Schemas["IncomeTotals"] {
  const cost = totals(url);
  return {
    granularity: cost.granularity,
    basis: cost.basis,
    periods: cost.periods.map((period) => ({
      label: period.label,
      starts_on: period.starts_on,
      ends_on: period.ends_on,
      net: [{ amount: "4800", currency: "JPY" }],
      balance: [
        { amount: "3150", currency: "JPY" },
        { amount: "-12.34", currency: "USD" },
      ],
    })),
    plans: [{ plan_id: "0192f0c4-7a8e-7000-8000-0000000000c1", name: "A plan with a rather long name that has to wrap on the narrowest screens", members: 3 }],
    rules: { week_start: "monday", providers: [{ provider: "stripe", time_zone: "Asia/Tokyo" }], amortize_after_days: 31 },
  };
}

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({ status, contentType: "application/json", headers: { "Cache-Control": "no-store" }, body: JSON.stringify(body) });
}

/** Plays the API of the supporter; returns the paths of the admin requests the app sent, which should be none. */
async function serveApi(page: Page) {
  const locale = test.info().project.use.locale ?? "en";
  const adminRequests: string[] = [];
  await page.route("**/oauth/token/refresh", (route) => json(route, { access_token: "e2e-token", token_type: "Bearer", expires_in: 3600, scope: "read write follow push" }));
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^\/api\/v1/, "");
    if (path.startsWith("/admin/")) adminRequests.push(path);
    const key = `${request.method()} ${path}`;
    if (key === "GET /instance") {
      return json(route, {
        domain: "my.example",
        name: "my example",
        version: "0.0.0",
        source_url: "https://github.com/mycroblog/my",
        registrations: { mode: "approval" }, federation: { mode: "both" },
        limits: { max_post_characters: 500, require_media_description: false },
        timelines: { local: "everyone", federated: "everyone" },
        rules: [],
      });
    }
    if (key === "GET /accounts/verify_credentials") return json(route, account);
    if (key === "GET /preferences") {
      return json(route, { locale, time_zone: "UTC", default_visibility: "public", default_post_language: "und", require_media_description: false, timeline_languages: [] });
    }
    if (key === "GET /notifications/marker") return json(route, { last_read_id: "0", unread_count: 0 });
    if (key === "GET /timelines/home") return json(route, { items: [] });
    if (key === "GET /finance/cost_totals") return json(route, totals(url));
    if (key === "GET /finance/income_totals") return json(route, incomeTotals(url));
    return json(route, { type: "about:blank", title: "Not Found", status: 404 }, 404);
  });
  return adminRequests;
}

/** Whether the project's viewport has the phones' tab bar rather than the rail (Tailwind's md). */
function hasTabBar(page: Page): boolean {
  return (page.viewportSize()?.width ?? 0) < 768;
}

test.describe("the finances page", () => {
  test("is in the navigation without the staff's entries, and shows the totals and the items", async ({ page, makeAxeBuilder }) => {
    const adminRequests = await serveApi(page);
    await page.goto("/home");
    const nav = page.getByRole("navigation").first();
    const entry = nav.locator('a[href="/finance"]');
    await expect(nav.locator('a[href="/moderation"]')).toHaveCount(0);
    await expect(nav.locator('a[href="/admin"]')).toHaveCount(0);
    if (hasTabBar(page)) {
      await expect(entry).toBeHidden();
      await nav.getByRole("button").last().click();
      const link = page.getByRole("dialog").locator('a[href="/finance"]');
      await expect(link).toHaveAccessibleName(message("shell.nav.finance"));
      await expect(page.locator("[data-entering]")).toHaveCount(0);
      await link.click();
    } else {
      await expect(entry).toBeVisible();
      await expect(entry).toHaveAccessibleName(message("shell.nav.finance"));
      await entry.click();
    }

    await expect(page).toHaveURL(/\/finance$/u);
    const main = page.getByRole("main");
    await expect(main.getByRole("heading", { level: 1, name: message("finance.title"), exact: true })).toBeFocused();
    const costs = main.getByRole("region", { name: message("finance.costs.title"), exact: true });
    await expect(costs.getByRole("table")).toBeVisible();
    await expect(costs.getByText(longName)).toHaveAttribute("dir", "auto");
    await expect(costs.getByText("الخادم الرئيسي")).toHaveAttribute("dir", "auto");
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);

    const income = main.getByRole("region", { name: message("income.title"), exact: true });
    await expect(income.getByRole("table")).toBeVisible();
    await expect(income.getByText("A plan with a rather long name that has to wrap on the narrowest screens")).toHaveAttribute("dir", "auto");

    // The tabs choose the periods of the costs and of the income together.
    await main.getByRole("tab", { name: message("costs.granularity.week"), exact: true }).click();
    await expect(costs.getByRole("row", { name: /2026-W43/u })).toBeVisible();
    await expect(page).toHaveURL(/granularity=week/u);
    await expectNoAxeViolations(makeAxeBuilder);
    await expectNoHorizontalScroll(page);
    expect(adminRequests).toEqual([]);
  });
});
