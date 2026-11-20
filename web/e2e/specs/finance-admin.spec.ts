// SPDX-License-Identifier: AGPL-3.0-only

// Recording a cost on the real server (#3264, FR-901): an administrator finds Costs and income through Admin in the navigation
// and its list of screens, opens the costs, records a monthly one, and lands on its screen, which reads it back from
// the server with its price from its first payment; then revises the price from a later date, which the list of
// prices says it applies from, and deletes the cost, after a question, so that the run leaves nothing in the totals.
// What each screen shows in every state is the stories' on the fake (web/app, finance-admin.stories.tsx).

import { randomUUID } from "node:crypto";
import { expect, test } from "./test";

test("an administrator records a monthly cost, revises its price from a date, and deletes it", async ({
  page,
  signIn,
  fresh,
}) => {
  const admin = await fresh.person({ admin: true });
  const name = `Object storage ${randomUUID().slice(0, 8)}`;
  await signIn(admin);

  // Admin in the navigation, then Costs and income among the screens, then its costs.
  await page
    .getByRole("navigation", { name: "Main", exact: true })
    .getByRole("link", { name: "Admin", exact: true })
    .click();
  const main = page.getByRole("main");
  await main.getByRole("link", { name: /^Costs and income .+$/ }).click();
  await main.getByRole("link", { name: /^Costs .+$/ }).click();
  await main.getByRole("link", { name: "Record a cost", exact: true }).click();

  // The form: what is paid for, to whom, monthly through 2020, in euros: periods and a currency that no other spec
  // reads the totals of, on the server they share.
  await main.getByLabel("Name", { exact: true }).fill(name);
  await main.getByLabel("Provider", { exact: true }).fill("Example Cloud");
  await main.getByLabel("Category", { exact: true }).selectOption("storage");
  await main.getByLabel("How often", { exact: true }).selectOption("months");
  await main.getByLabel("First payment on", { exact: true }).fill("2020-01-01");
  await main.getByLabel("Last payment on", { exact: true }).fill("2020-12-01");
  await main.getByLabel("Amount", { exact: true }).fill("1200");
  await main.getByLabel("Currency", { exact: true }).selectOption("EUR");
  await main.getByRole("button", { name: "Record the cost", exact: true }).click();

  // The cost's screen, read from the server: its name, and its price from its first payment.
  await expect(page).toHaveURL(/\/admin\/finance\/costs\/[^/]+$/);
  await expect(main.getByRole("heading", { name, exact: true, level: 2 })).toBeVisible();
  const prices = main.getByRole("table", { name: "Prices", exact: true });
  /** The row of the price that applies from `date`. */
  const from = (date: string) =>
    prices.getByRole("row").filter({ has: page.getByRole("rowheader", { name: date, exact: true }) });
  await expect(from("Jan 1, 2020")).toContainText("€1,200.00");

  // A new price from a later date, which the prices then say it applies from.
  await main.getByLabel("Applies from", { exact: true }).fill("2020-06-01");
  await main.getByLabel("New price in EUR", { exact: true }).fill("1500");
  await main.getByRole("button", { name: "Revise the price", exact: true }).click();
  await expect(
    page.getByRole("status").getByText("From Jun 1, 2020, the price is €1,500.00.", { exact: true }),
  ).toBeVisible();
  await expect(from("Jun 1, 2020")).toContainText("€1,500.00");
  await expect(from("Jan 1, 2020")).toContainText("€1,200.00");

  // Deleted after its question, it is gone from the costs.
  await main.getByRole("button", { name: "Delete the cost…", exact: true }).click();
  await page
    .getByRole("alertdialog", { name: `Delete ${name}?`, exact: true })
    .getByRole("button", { name: "Delete", exact: true })
    .click();
  await expect(page).toHaveURL(/\/admin\/finance\/costs$/);
  await expect(main.getByRole("link", { name: "Record a cost", exact: true })).toBeVisible();
  await expect(main.getByText(name, { exact: true })).toHaveCount(0);
});
