// SPDX-License-Identifier: AGPL-3.0-only

// The stories of the finance screens (web/app/src/finance.stories.tsx) each show the state they are named by, on the
// fake API: the public page where nothing is published, with the funding meter alone, with the costs' totals alone,
// with the breakdown, the income and the balance, with the income and its members by plan alone, with an estimate,
// for a visitor, loading and failing; and the finance page of a holder of `view_finance` with the settings' defaults,
// the items' names and notes, their notes alone, the members by plan, an estimate, the administrator's, a reader who
// may not read it, a visitor, loading and failing. What the screens do from each state is the app's route tests'
// (web/app/src/finance.dom.test.tsx); this is that the catalog shows each state for a review and a shot.

import type { Page } from "@playwright/test";
import { readCellByCell } from "../tables";
import { expect, pc, test } from "../test";

/** The story of the finance screens named `name`. */
const story = (name: string) => `screens-finance--${name}`;

const main = (page: Page) => page.getByRole("main");

const table = (page: Page, name: string | RegExp) =>
  main(page).getByRole("table", { name, exact: typeof name === "string" });

/** The totals by month. */
const byMonth = (page: Page) => table(page, "Totals by month");

/** The rows of the totals by month, the columns' row left out. */
const months = (page: Page) => byMonth(page).getByRole("rowheader");

/** The latest month's breakdown, and its items: a month as the table heads it ("November 2026"). */
const breakdown = /^[A-Z][a-z]+ \d{4} by provider$/;
const itemsOf = /^[A-Z][a-z]+ \d{4} by item$/;

/** The row of the item VPS, its note under its name where it is shown. */
const vps = /^VPS(\s.*)?$/;

const columns = (page: Page) => byMonth(page).getByRole("columnheader");

const states: readonly (readonly [string, (page: Page) => Promise<void>])[] = [
  [
    "public-private",
    async (page) => {
      await expect(main(page).getByText(/^This server does not publish its costs or income\.$/)).toBeVisible();
      await expect(main(page).getByRole("table")).toHaveCount(0);
      await expect(main(page).getByRole("meter")).toHaveCount(0);
    },
  ],
  [
    "public-meter-only",
    async (page) => {
      await expect(main(page).getByRole("heading", { name: "This month's funding", exact: true })).toBeVisible();
      await expect(
        main(page).getByRole("meter", { name: "Share of this month's costs covered", exact: true }),
      ).toBeVisible();
      // Under the meter, only that the totals are not published.
      await expect(main(page).getByText(/^This server does not publish its totals\.$/)).toBeVisible();
      await expect(main(page).getByText(/^This server does not publish its costs or income\.$/)).toHaveCount(0);
    },
  ],
  [
    "public-totals",
    async (page) => {
      await expect(months(page)).toHaveCount(6);
      await expect(columns(page)).toHaveText(["Month", "Costs"]);
      await expect(main(page).getByText(/^This server does not publish its income\.$/)).toBeVisible();
      await expect(
        main(page).getByText(/^Periods are calendar dates with no time zone, and a week starts on Monday\. .+$/),
      ).toBeVisible();
      await expect(table(page, breakdown)).toHaveCount(0);
    },
  ],
  [
    "public-breakdown",
    async (page) => {
      await expect(months(page)).toHaveCount(6);
      await expect(columns(page)).toHaveText(["Month", "Costs", "Income", "Balance"]);
      await expect(table(page, breakdown)).toBeVisible();
      await expect(
        main(page).getByText(/^.+ AWS bills by calendar month in UTC, and its bills count as billed\. .+$/),
      ).toBeVisible();
      await expect(main(page).getByText(/^.+ Income through Stripe counts on its date in UTC\. .+$/)).toBeVisible();
      await expect(main(page).getByText(/^.+ Who paid what is never shown\.$/)).toBeVisible();
      await expect(main(page).getByText(/^¥[\d,]+ of ¥20,000$/)).toBeVisible();
      // The meter names the currency its share counts in.
      await expect(main(page).getByText(/^of [A-Z][a-z]+'s costs covered, counted in JPY$/)).toBeVisible();
    },
  ],
  [
    "public-income-only",
    async (page) => {
      await expect(columns(page)).toHaveText(["Month", "Income"]);
      await expect(main(page).getByText(/^This server does not publish its costs\.$/)).toBeVisible();
      await expect(table(page, "Members by plan").getByRole("rowheader")).toHaveText(["Supporter", "Patron"]);
    },
  ],
  [
    "public-estimated",
    async (page) => {
      await expect(byMonth(page)).toBeVisible();
      await expect
        .poll(async () => (await readCellByCell(page, "Totals by month"))[0])
        .toMatch(/^[A-Z][a-z]+ \d{4}.* \| Costs \| .*Estimate/);
      await expect(main(page).getByRole("meter")).toBeVisible();
      // What an estimate means is said once, beside the meter, which counts in dollars.
      await expect(
        main(page).getByText(/^.+, counted in USD\s*This month's costs include an estimate, so this may change\.$/),
      ).toBeVisible();
      await expect(
        main(page).getByText(/^.*This month's costs include an estimate, so this may change\.$/),
      ).toHaveCount(1);
    },
  ],
  [
    "public-visitor",
    async (page) => {
      await expect(months(page)).toHaveCount(6);
      await expect(table(page, breakdown)).toBeVisible();
    },
  ],
  [
    "public-loading",
    async (page) => {
      await expect(main(page).getByText(/^Loading the totals…$/)).toBeVisible();
    },
  ],
  [
    "public-error",
    async (page) => {
      await expect(main(page).getByRole("alert")).toHaveText(/^The totals could not be loaded\.\s*Try again$/);
    },
  ],
  [
    "finance",
    async (page) => {
      await expect(months(page)).toHaveCount(6);
      await expect(columns(page)).toHaveText(["Month", "Costs", "Income", "Balance"]);
      await expect(table(page, breakdown)).toBeVisible();
      await expect(table(page, itemsOf)).toHaveCount(0);
      await expect(table(page, "Members by plan")).toHaveCount(0);
    },
  ],
  [
    "finance-names-notes",
    async (page) => {
      const items = table(page, itemsOf);
      await expect(items.getByRole("rowheader", { name: vps })).toBeVisible();
      await expect(items.getByText("2 vCPU, 4 GB, Tokyo zone", { exact: true })).toBeVisible();
      await expect(items.getByRole("columnheader")).toHaveText(["Item", "Provider", "Category", "Amount"]);
    },
  ],
  [
    "finance-notes-only",
    async (page) => {
      const items = table(page, itemsOf);
      await expect(items.getByRole("columnheader")).toHaveText(["Provider", "Category", "Amount"]);
      await expect(items.getByText("2 vCPU, 4 GB, Tokyo zone", { exact: true })).toBeVisible();
      await expect(items.getByRole("rowheader", { name: vps })).toHaveCount(0);
    },
  ],
  [
    "finance-plans",
    async (page) => {
      await expect(table(page, "Members by plan").getByRole("rowheader")).toHaveText(["Supporter", "Patron"]);
      await expect(table(page, "Members by plan").getByRole("cell")).toHaveText(["2 members", "2 members"]);
      await expect(table(page, itemsOf)).toHaveCount(0);
    },
  ],
  [
    "finance-plans-none",
    async (page) => {
      await expect(main(page).getByRole("heading", { name: "Members by plan", exact: true })).toBeVisible();
      await expect(main(page).getByText(/^No plan has members yet\.$/)).toBeVisible();
      await expect(table(page, "Members by plan")).toHaveCount(0);
    },
  ],
  [
    "finance-estimated",
    async (page) => {
      await expect
        .poll(async () => (await readCellByCell(page, "Totals by month"))[0])
        .toMatch(/ \| Costs \| .*Estimate/);
    },
  ],
  [
    "finance-admin",
    async (page) => {
      await expect(table(page, itemsOf).getByRole("rowheader", { name: vps })).toBeVisible();
      await expect(table(page, "Members by plan")).toBeVisible();
    },
  ],
  [
    "finance-forbidden",
    async (page) => {
      await expect(
        main(page).getByText(
          /^Only people allowed to read this server's finances can see this page\. To read them, ask the server's administrators\.$/,
        ),
      ).toBeVisible();
      await expect(main(page).getByRole("table")).toHaveCount(0);
    },
  ],
  [
    "finance-visitor",
    async (page) => {
      await expect(main(page).getByRole("link", { name: "Sign in", exact: true })).toBeVisible();
      await expect(main(page).getByRole("table")).toHaveCount(0);
    },
  ],
  [
    "finance-loading",
    async (page) => {
      await expect(main(page).getByText(/^Loading the totals…$/)).toBeVisible();
    },
  ],
  [
    "finance-error",
    async (page) => {
      await expect(main(page).getByRole("alert")).toHaveText(/^The totals could not be loaded\.\s*Try again$/);
    },
  ],
];

// One page walks the stories, a step each: a page of its own per story would cost the catalog's run its time limit.
test("each story of the finance screens shows its state", async ({ page, open }) => {
  for (const [name, shows] of states)
    await test.step(name, async () => {
      await open(story(name), { width: pc });
      await shows(page);
    });
});
