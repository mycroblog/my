// SPDX-License-Identifier: AGPL-3.0-only

// Creating a supporter plan on the real server with its fake payment provider (#3264, FR-910, FR-912): an
// administrator, given a role a plan can be on, finds Supporters through Admin in the navigation and its list of
// screens, opens the plans, creates one on that role, and lands on the plan's screen, which reads its terms back from
// the server; then retires it, after typing its name. What each screen shows in every state is the stories' on the fake (web/app,
// supporters-admin.stories.tsx).

import { randomUUID } from "node:crypto";
import { expect, test } from "./test";

test("an administrator creates a supporter plan on a role of its own, then retires it", async ({
  page,
  signIn,
  fresh,
}) => {
  const admin = await fresh.person({ admin: true });
  const role = await fresh.planRole(admin);
  const name = `Plan ${randomUUID().slice(0, 8)}`;
  await signIn(admin);

  // Admin in the navigation, then Supporters among the screens, then its plans.
  await page
    .getByRole("navigation", { name: "Main", exact: true })
    .getByRole("link", { name: "Admin", exact: true })
    .click();
  const main = page.getByRole("main");
  await main.getByRole("link", { name: /^Supporters .+$/ }).click();
  await main.getByRole("link", { name: /^Plans(?: .+)?$/ }).click();
  await main.getByRole("link", { name: "Create a plan", exact: true }).click();

  // The form: the role among those a plan can be on, the terms, then Create the plan.
  await main.getByLabel("Role", { exact: true }).selectOption(role);
  await main.getByLabel("Name", { exact: true }).fill(name);
  await main.getByLabel("Description", { exact: true }).fill("A plan of the end-to-end tests.");
  await main.getByLabel("Monthly price", { exact: true }).fill("800");
  await main.getByLabel("Yearly discount (%)", { exact: true }).fill("10");
  await main.getByLabel("Order", { exact: true }).fill(String(100 + Math.floor(Math.random() * 900_000)));
  await main.getByRole("button", { name: "Create the plan", exact: true }).click();

  // The plan's screen, read from the server: its name, its role, and its prices.
  await expect(page).toHaveURL(/\/admin\/supporters\/plans\/[^/]+$/);
  await expect(main.getByRole("heading", { name, exact: true, level: 2 })).toBeVisible();
  await expect(main.getByText(role, { exact: true })).toBeVisible();
  await expect(main.getByText(/^¥800 a month, ¥[\d,]+ a year \(10% off\)$/)).toBeVisible();

  // Retired, after its name is typed, it takes no subscriptions, and leaves the order of the plans to the next run.
  await main.getByRole("button", { name: "Retire the plan…", exact: true }).click();
  const dialog = page.getByRole("alertdialog");
  await dialog.getByRole("textbox").fill(name);
  await dialog.getByRole("button", { name: "Retire", exact: true }).click();
  await expect(page.getByRole("status").getByText("The plan is retired.", { exact: true })).toBeVisible();
  await expect(main.getByRole("button", { name: "Retire the plan…", exact: true })).toHaveCount(0);
});
