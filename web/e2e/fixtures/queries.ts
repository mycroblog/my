// SPDX-License-Identifier: AGPL-3.0-only
// The E2E tests find an element by its role and accessible name, never by its position, its markup or a test ID.

import type { Locator, Page } from "@playwright/test";

export async function queries(page: Page, item: Locator, selector: string) {
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await page.getByLabel("Password", { exact: true }).fill("x");
  await page.getByRole("feed").getByRole("article").filter({ hasText: "bread" }).click();
  await item.locator(page.getByRole("link")).click();
  await page.locator(selector).click();
  await page.getByRole("article").nth(1).click(); // error: e2e .nth()
  await page.getByRole("article").first().click(); // error: e2e .first()
  await page.getByRole("article").last().click(); // error: e2e .last()
  await page.locator("main button[type=submit]").click(); // error: e2e .locator(selector)
  await item.locator(`//article[${String(1)}]`).click(); // error: e2e .locator(selector)
  await page.locator("article >> " + selector).click(); // error: e2e .locator(selector)
  await page.$("main"); // error: e2e .$()
  await page.$$("article"); // error: e2e .$$()
  await page.getByTestId("composer").click(); // error: e2e .getByTestId()
}
