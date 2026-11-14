// SPDX-License-Identifier: AGPL-3.0-only
// The E2E tests assert with retrying assertions, match a whole name, act as a person does, and run without motion.
// Outside specs/ (the demo server, the seed), a timeout, the clock and `force` are open.

import { expect, type Locator, type Page } from "@playwright/test";

export async function flaky(page: Page, item: Locator, name: string) {
  await expect(item).toHaveText("Saved");
  await expect(item).toHaveCount(2);
  await expect(item).toHaveAttribute("aria-pressed", "true");
  await expect.poll(() => name.length).toBe(4);
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await page.getByRole("button", { name: /^Favourite(, \d+)?$/ }).click();
  await page.getByRole("heading", { name: new RegExp(`^${name} \\(@\\w+\\)$`) }).click();
  await page.getByRole("button", { name: /^(Boost|Quote)$/ }).click();
  await page.getByRole("feed").click();
  await page.getByRole("button", { name }).click();
  await page.getByLabel("Password", { exact: true }).fill("x");
  await page.getByText(/^Saved$/).click();
  await page.getByRole("radio", { name: "Spam", exact: true }).click();
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.waitForResponse((r) => r.url().endsWith("/favourite"), { timeout: 5_000 });
  await item.click({ force: true });
  await Promise.resolve(Date.now() + new Date().getTime());

  expect(await page.title()).toBe("Home"); // error: e2e-flaky expect(await)
  await item.textContent(); // error: e2e-flaky .textContent()
  await item.innerText(); // error: e2e-flaky .innerText()
  await item.innerHTML(); // error: e2e-flaky .innerHTML()
  await item.isVisible(); // error: e2e-flaky .isVisible()
  await item.isHidden(); // error: e2e-flaky .isHidden()
  await item.isChecked(); // error: e2e-flaky .isChecked()
  await item.isEnabled(); // error: e2e-flaky .isEnabled()
  await item.isDisabled(); // error: e2e-flaky .isDisabled()
  await item.count(); // error: e2e-flaky .count()
  await item.getAttribute("aria-pressed"); // error: e2e-flaky .getAttribute()
  await item.inputValue(); // error: e2e-flaky .inputValue()
  await item.allTextContents(); // error: e2e-flaky .allTextContents()
  await item.allInnerTexts(); // error: e2e-flaky .allInnerTexts()

  await page.getByRole("button", { name: "Save" }).click(); // error: e2e-flaky getByRole(string)
  await page.getByRole("button", { name: `Save ${name}`, exact: false }).click(); // error: e2e-flaky getByRole(string)
  await page.getByRole("button", { name: /Favourite/ }).click(); // error: e2e-flaky getByRole(/unanchored/)
  await page.getByRole("button", { name: /^Favourite/ }).click(); // error: e2e-flaky getByRole(/unanchored/)
  await page.getByRole("button", { name: /^Boost|Quote$/ }).click(); // error: e2e-flaky getByRole(/unanchored/)
  await page.getByRole("button", { name: /^Price \$/ }).click(); // error: e2e-flaky getByRole(/unanchored/)
  await page.getByRole("heading", { name: new RegExp(`^${name} \\(@`) }); // error: e2e-flaky getByRole(/unanchored/)
  await page.getByLabel("Password").fill("x"); // error: e2e-flaky getByLabel(string)
  await page.getByText("Saved").click(); // error: e2e-flaky getByText(string)
  await page.getByPlaceholder("Search").fill("x"); // error: e2e-flaky getByPlaceholder(string)
  await page.getByTitle("Close").click(); // error: e2e-flaky getByTitle(string)
  await page.getByAltText("Avatar").click(); // error: e2e-flaky getByAltText(string)
  await page.getByText(/Saved/).click(); // error: e2e-flaky getByText(/unanchored/)

  await page.getByRole("checkbox", { name: "Spam", exact: true }).check(); // error: e2e-flaky .check()
  await page.getByRole("checkbox", { name: "Spam", exact: true }).uncheck(); // error: e2e-flaky .uncheck()
  await page.getByRole("checkbox", { name: "Spam", exact: true }).setChecked(true); // error: e2e-flaky .setChecked()

  await page.emulateMedia({ reducedMotion: "no-preference" }); // error: e2e-flaky reducedMotion
}
