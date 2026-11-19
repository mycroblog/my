// SPDX-License-Identifier: AGPL-3.0-only

// The finance on the real server (#3246, FR-902, FR-903, FR-916, FR-917): a fresh administrator records a monthly cost
// with a provider of its own, and the server publishes its costs' breakdown; a visitor reads at /about/costs the
// published totals the API answers for this month and the cost's provider in the month's breakdown; the administrator
// tips the server once on the fake payment provider's checkout, which the server records as income; and as a holder
// of `view_finance` reads at /finance the cost and income totals the API answers them, with the item's name. What the
// screens show in each state is the stories' and the route tests' on the fake (web/app, finance.stories.tsx,
// finance.dom.test.tsx).

import { randomUUID } from "node:crypto";
import type { Page } from "@playwright/test";
import { readCellByCell } from "../catalog/specs/tables";
import { signIn as apiSignIn } from "../src/seed/sign-in";
import { runMyOn } from "../src/server/demo";
import { expect, test } from "./test";

type Money = { amount: string; currency: string };

/** What the server publishes of its costs now (`my instance get`), `private` where it was never set. */
function visibilityOf(envFile: string): string {
  try {
    return runMyOn(envFile, ["instance", "get", "finance_visibility"]).trim();
  } catch {
    return "private";
  }
}

/** Puts back what the test changed of the server's settings, whatever became of the test, the last change first. */
const restores: (() => unknown)[] = [];
test.afterEach(async () => {
  for (const restore of restores.splice(0).toReversed()) await restore();
});

/** The amounts as the screen writes them, in en. */
const written = (amounts: readonly Money[]) =>
  amounts.map((m) => new Intl.NumberFormat("en", { style: "currency", currency: m.currency }).format(Number(m.amount)));

/** The latest period of the totals the screen asked for at `url`, as the server answers it to `get`. */
async function latestOf(
  get: (url: string) => Promise<unknown>,
  url: string,
): Promise<{ label: string; totals: Money[] }> {
  const answer = (await get(url)) as { periods: { label: string; totals: Money[] }[] };
  const latest = answer.periods.at(-1);
  if (latest === undefined) throw new Error(`e2e: the server answered no period to ${url}`);
  return latest;
}

/** The name of the month labelled `label` (`2026-11`) as the table heads it ("November 2026"). */
function monthName(label: string): string {
  const [year = 0, month = 1] = label.split("-").map(Number);
  return new Intl.DateTimeFormat("en", { timeZone: "UTC", year: "numeric", month: "long" }).format(
    Date.UTC(year, month - 1, 1),
  );
}

/** The row of the month labelled `label` in the totals by month, headed by its name. */
function monthRow(page: Page, label: string) {
  const name = monthName(label);
  return page
    .getByRole("main")
    .getByRole("table", { name: "Totals by month", exact: true })
    .getByRole("row")
    .filter({ has: page.getByRole("rowheader", { name: new RegExp(`^${name}( So far)?$`) }) });
}

test("a visitor reads the published totals, and a holder of view_finance the finance totals", async ({
  page,
  signIn,
  fresh,
  baseURL,
}) => {
  const envFile = process.env["E2E_DEMO_ENV"];
  if (envFile === undefined || baseURL === undefined) throw new Error("e2e: no demo server");
  const id = randomUUID().slice(0, 8);
  const provider = `E2E Cloud ${id}`;
  const name = `E2E hosting ${id}`;

  // The server publishes its costs' totals and their breakdown while the test runs, and keeps them as it did after it,
  // so that no other spec depends on the order; a fresh administrator records a monthly cost.
  const before = visibilityOf(envFile);
  runMyOn(envFile, ["instance", "set", "finance_visibility", "-"], "breakdown");
  restores.push(() => runMyOn(envFile, ["instance", "set", "finance_visibility", "-"], before));
  await expect
    .poll(
      async () =>
        ((await (await page.request.get("/api/v1/instance")).json()) as Record<string, unknown>)["finance_visibility"],
    )
    .toBe("breakdown");
  const admin = await fresh.person({ admin: true });
  const api = await apiSignIn(baseURL, admin);
  const created = await api.POST("/admin/cost_items", {
    body: {
      schedule: "months",
      interval: 1,
      starts_on: "2025-01-01",
      name,
      provider,
      category: "compute",
      price: { amount: "1200", currency: "JPY" },
    },
  });
  expect(created.error).toBeUndefined();

  // A visitor: the published totals of this month, as the API answers them, and the cost's provider in its breakdown.
  const asked = page.waitForRequest((r) => new URL(r.url()).pathname === "/api/v1/instance/cost_totals");
  await page.goto("/about/costs");
  const published = await latestOf(async (url) => (await page.request.get(url)).json(), (await asked).url());
  expect(published.totals.length).toBeGreaterThan(0);
  for (const figure of written(published.totals)) await expect(monthRow(page, published.label)).toContainText(figure);
  const breakdown = page.getByRole("main").getByRole("table", { name: /^.+ by provider$/ });
  await expect(breakdown.getByRole("rowheader", { name: provider, exact: true })).toBeVisible();

  // The administrator tips the server ¥1,000 on the fake provider's checkout, with the supporter program and the tips
  // on as the supporters' spec has them; the payment the provider books is the server's income.
  await fresh.supporterProgram();
  const tipAmount = { amount: "1000", currency: "JPY" };
  const tip = await api.POST("/supporter/tips", { body: { amount: tipAmount } });
  expect(tip.error).toBeUndefined();
  await signIn(admin);
  await page.goto(tip.data?.redirect_url ?? "");
  await page.getByRole("button", { name: "Pay", exact: true }).click();
  // The return from the checkout reads the tip again until it is paid, as the tip's screen does.
  const tipId = tip.data?.tip_id ?? "";
  await expect
    .poll(async () => (await api.GET("/supporter/tips/{id}", { params: { path: { id: tipId } } })).data?.state, {
      message: "the tip is paid",
    })
    .toBe("paid");
  // The provider books the payment's money among its changes, which the server reads at its reconciliation: the
  // administrator asks for one now, as the server's settings let them (one an hour; a demo server is new each run).
  const reconciled = await api.POST("/admin/payment_reconciliation/runs");
  expect(reconciled.response.status).toBe(202);

  // The administrator, who holds view_finance: the finance totals the API answers them, and the item by its name.
  const read = page.waitForRequest((r) => new URL(r.url()).pathname === "/api/v1/finance/cost_totals");
  await page.goto("/finance");
  const query = Object.fromEntries(new URL((await read).url()).searchParams) as {
    granularity: "month";
    from: string;
    to: string;
  };
  const finance = await latestOf(
    async () => (await api.GET("/finance/cost_totals", { params: { query } })).data,
    "/api/v1/finance/cost_totals",
  );
  expect(finance.totals.length).toBeGreaterThan(0);
  for (const figure of written(finance.totals)) await expect(monthRow(page, finance.label)).toContainText(figure);
  const items = page.getByRole("main").getByRole("table", { name: /^.+ by item$/ });
  await expect(items.getByRole("rowheader", { name, exact: true })).toBeVisible();

  // The income totals, once the server recorded the tip: this month's net in yen, as the API answers it, in its
  // Income cell.
  const incomeNow = async () => {
    const answer = (await api.GET("/finance/income_totals", { params: { query } })).data;
    return answer?.periods.at(-1);
  };
  await expect
    .poll(async () => Number((await incomeNow())?.net.find((m) => m.currency === "JPY")?.amount ?? 0), {
      message: "the tip is recorded as income",
    })
    .toBeGreaterThan(0);
  await page.reload();
  const income = await incomeNow();
  if (income === undefined) throw new Error("e2e: the server answered no period of income");
  const incomeCell = async () =>
    (await readCellByCell(page, "Totals by month")).find(
      (cell) => cell.startsWith(monthName(income.label)) && cell.includes(" | Income | "),
    ) ?? "";
  for (const figure of written(income.net))
    await expect.poll(incomeCell, { message: `the Income cell shows ${figure}` }).toContain(figure);
});
