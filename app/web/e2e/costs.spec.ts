// SPDX-License-Identifier: AGPL-3.0-only
// The public running costs and income (FR-903, FR-917, ADR-0067 §8, ADR-0073 §7) in every width × locale project: the
// section of the information page and the page at /about/costs as the instance settings finance_visibility and
// income_visibility publish them, against `vite preview`, whose
// client API the spec plays with page.route (no session). Names change with the locale, so elements are found by role,
// structure and addresses, and the providers, which are the operators' words, are in English in every project.
import type { Page } from "@playwright/test";
import type { components } from "@mycroblog/mykit";
import { expect, expectNoAxeViolations, expectNoHorizontalScroll, test } from "./fixtures";

type Instance = components["schemas"]["Instance"];
type CostTotals = components["schemas"]["CostTotals"];
type IncomeTotals = components["schemas"]["IncomeTotals"];

/** The totals of the periods of a query: the last one with costs in two currencies and, with `breakdown`, their breakdown. */
function totals(url: URL, breakdown: boolean): CostTotals {
  const to = url.searchParams.get("to") ?? "2026-10-22";
  const month = to.slice(0, 7);
  const start = new Date(`${month}-01T00:00:00Z`);
  start.setUTCMonth(start.getUTCMonth() - 1);
  const previous = start.toISOString().slice(0, 7);
  return {
    granularity: (url.searchParams.get("granularity") ?? "month") as CostTotals["granularity"],
    basis: (url.searchParams.get("basis") ?? "amortized") as CostTotals["basis"],
    periods: [
      {
        label: previous,
        starts_on: `${previous}-01`,
        ends_on: `${previous}-28`,
        totals: [{ amount: "1650", currency: "JPY" }],
        breakdown: [],
        estimated: false,
      },
      {
        label: month,
        starts_on: `${month}-01`,
        ends_on: to,
        totals: [
          { amount: "1650", currency: "JPY" },
          { amount: "1234.56", currency: "USD" },
        ],
        breakdown: breakdown
          ? [
              {
                provider: "A registrar with a rather long name that has to wrap on the narrowest screens",
                category: "domain",
                amount: { amount: "1650", currency: "JPY" },
              },
              { provider: "Cloud", category: "compute", amount: { amount: "1234.56", currency: "USD" } },
            ]
          : [],
        estimated: true,
      },
    ],
    rules: { week_start: "monday", sources: breakdown ? [{ source: "aws", provider: "AWS", time_zone: "UTC" }] : [] },
  };
}

/**
 * The income totals of the periods of a query: the last one with income, with the balance while the costs are public,
 * and the members of a plan whose name has to wrap with `plans`.
 */
function incomeTotals(url: URL, balance: boolean, plans: boolean): IncomeTotals {
  const to = url.searchParams.get("to") ?? "2026-10-22";
  const month = to.slice(0, 7);
  return {
    granularity: (url.searchParams.get("granularity") ?? "month") as IncomeTotals["granularity"],
    basis: (url.searchParams.get("basis") ?? "amortized") as IncomeTotals["basis"],
    periods: [
      {
        label: month,
        starts_on: `${month}-01`,
        ends_on: to,
        net: [{ amount: "4800", currency: "JPY" }],
        ...(balance
          ? {
              balance: [
                { amount: "3150", currency: "JPY" },
                { amount: "-1234.56", currency: "USD" },
              ],
            }
          : {}),
      },
    ],
    ...(plans
      ? {
          plans: [
            {
              plan_id: "0199f0e0-0000-7000-8000-00000000aa01",
              name: "A plan with a rather long name that has to wrap on the narrowest screens",
              members: 3,
            },
          ],
        }
      : {}),
    rules: {
      week_start: "monday",
      providers: [{ provider: "stripe", time_zone: "Asia/Tokyo" }],
      amortize_after_days: 31,
    },
  };
}

/** Plays the instance with `visibility` and `income`, and its public totals, for a visitor without a session. */
async function serveCosts(
  page: Page,
  visibility: Instance["finance_visibility"],
  income: Instance["income_visibility"] = "private",
): Promise<void> {
  const instance: Instance = {
    domain: "my.example",
    name: "my example",
    version: "0.0.0-e2e",
    source_url: "https://github.com/mycroblog/my",
    registrations: { mode: "approval" },
    limits: { max_post_characters: 500, require_media_description: false },
    timelines: { local: "everyone", federated: "everyone" },
    federation: { mode: "both" },
    sends_email: true,
    search_indexing: "deny",
    ai_usage: "deny",
    finance_visibility: visibility,
    income_visibility: income,
    supporters_enabled: false,
    supporter_list_visibility: "off",
    voice_enabled: false,
    voice_room_starters: "permitted",
    rules: [],
  };
  await page.route("**/api/v1/instance", (route) => route.fulfill({ json: instance }));
  await page.route("**/api/v1/instance/domain_actions*", (route) =>
    route.fulfill({
      status: 404,
      contentType: "application/problem+json",
      json: { type: "about:blank", title: "Not Found", status: 404 },
    }),
  );
  await page.route("**/api/v1/instance/cost_totals*", (route) =>
    visibility === "private"
      ? route.fulfill({
          status: 404,
          contentType: "application/problem+json",
          json: { type: "about:blank", title: "Not Found", status: 404 },
        })
      : route.fulfill({ json: totals(new URL(route.request().url()), visibility === "breakdown") }),
  );
  await page.route("**/api/v1/instance/income_totals*", (route) =>
    income === "private"
      ? route.fulfill({
          status: 404,
          contentType: "application/problem+json",
          json: { type: "about:blank", title: "Not Found", status: 404 },
        })
      : route.fulfill({
          json: incomeTotals(new URL(route.request().url()), visibility !== "private", income === "plans"),
        }),
  );
}

test.describe("the costs page of a server that publishes the breakdown", () => {
  test.beforeEach(async ({ page }) => {
    await serveCosts(page, "breakdown");
    await page.goto("/about/costs");
    await expect(page.getByRole("main").getByRole("table")).toBeVisible();
  });

  test("shows the totals by period, the breakdown of the current one, and the rules", async ({ page }) => {
    const main = page.getByRole("main");
    await expect(main.getByRole("heading", { level: 1 })).toContainText("my example");
    await expect(main.getByRole("tab")).toHaveCount(3);
    await expect(main.getByRole("table").getByRole("row")).toHaveCount(3);
    await expect(main.locator('[dir="auto"]', { hasText: "Cloud" })).toBeVisible();
    // The costs, their breakdown and their rules; no income.
    await expect(main.getByRole("region")).toHaveCount(3);
  });

  test("has no axe violations", async ({ makeAxeBuilder }) => {
    await expectNoAxeViolations(makeAxeBuilder);
  });

  test("does not scroll horizontally", async ({ page }) => {
    await expectNoHorizontalScroll(page);
  });
});

test("the costs page of a server that publishes the totals only", async ({ page, makeAxeBuilder }) => {
  await serveCosts(page, "totals");
  await page.goto("/about/costs");
  const main = page.getByRole("main");
  await expect(main.getByRole("table")).toBeVisible();
  // The costs and their rules: no breakdown.
  await expect(main.getByRole("region")).toHaveCount(2);
  await expect(main.getByText("Cloud")).toHaveCount(0);
  await expectNoAxeViolations(makeAxeBuilder);
  await expectNoHorizontalScroll(page);
});

test("the costs page of a server that publishes nothing", async ({ page, makeAxeBuilder }) => {
  await serveCosts(page, "private");
  await page.goto("/about/costs");
  const main = page.getByRole("main");
  await expect(main.locator('a[href="/about"]')).toBeVisible();
  await expect(main.getByRole("table")).toHaveCount(0);
  await expectNoAxeViolations(makeAxeBuilder);
  await expectNoHorizontalScroll(page);
});

test("the information page of a server that publishes its costs links to them", async ({ page, makeAxeBuilder }) => {
  await serveCosts(page, "totals");
  await page.goto("/about");
  const main = page.getByRole("main");
  await expect(main.locator('a[href="/about/costs"]')).toBeVisible();
  await expectNoAxeViolations(makeAxeBuilder);
  await expectNoHorizontalScroll(page);
});

test.describe("the costs page of a server that publishes its costs and its income with the plans", () => {
  test.beforeEach(async ({ page }) => {
    await serveCosts(page, "totals", "plans");
    await page.goto("/about/costs");
    await expect(page.getByRole("main").getByRole("table")).toHaveCount(2);
  });

  test("shows the income with the balance and the members of each plan, under the same tabs", async ({ page }) => {
    const main = page.getByRole("main");
    await expect(main.getByRole("tab")).toHaveCount(3);
    // The income table: the period, the income after fees and the balance.
    await expect(main.getByRole("table").nth(1).getByRole("columnheader")).toHaveCount(3);
    await expect(main.locator('[dir="auto"]', { hasText: "A plan with a rather long name" })).toBeVisible();
    // The costs, their rules, the income, the members of each plan and the income's rules.
    await expect(main.getByRole("region")).toHaveCount(5);
  });

  test("has no axe violations", async ({ makeAxeBuilder }) => {
    await expectNoAxeViolations(makeAxeBuilder);
  });

  test("does not scroll horizontally", async ({ page }) => {
    await expectNoHorizontalScroll(page);
  });
});

test("the costs page of a server that publishes its income only", async ({ page, makeAxeBuilder }) => {
  await serveCosts(page, "private", "totals");
  await page.goto("/about/costs");
  const main = page.getByRole("main");
  await expect(main.getByRole("table")).toHaveCount(1);
  // The income without the balance: the period and the income after fees.
  await expect(main.getByRole("table").getByRole("columnheader")).toHaveCount(2);
  await expectNoAxeViolations(makeAxeBuilder);
  await expectNoHorizontalScroll(page);
});

test("the information page of a server that publishes its income only links to it", async ({
  page,
  makeAxeBuilder,
}) => {
  await serveCosts(page, "private", "totals");
  await page.goto("/about");
  const main = page.getByRole("main");
  await expect(main.locator('a[href="/about/costs"]')).toBeVisible();
  await expectNoAxeViolations(makeAxeBuilder);
  await expectNoHorizontalScroll(page);
});
