// SPDX-License-Identifier: AGPL-3.0-only
// Under specs/ (the specs and the operations of the screens), a spec waits for a state, never for time, and takes no
// time from the clock or a timeout of its own, and clicks as a person does, without force.

import { expect, type Locator, type Page } from "@playwright/test";

export async function timing(page: Page, item: Locator) {
  await expect(item).toBeVisible();
  await expect.poll(() => page.url()).toContain("/notifications");
  await page.clock.setFixedTime(new Date("2026-01-01T00:00:00Z"));

  await page.waitForTimeout(500); // error: e2e-flaky .waitForTimeout()
  await new Promise((resolve) => setTimeout(resolve, 500)); // error: e2e-flaky setTimeout()
  clearInterval(setInterval(() => undefined, 100)); // error: e2e-flaky setInterval()
  globalThis.setTimeout(() => undefined, 100); // error: e2e-flaky setTimeout()
  await expect(item).toHaveText(String(Date.now())); // error: e2e-flaky Date.now()
  await expect(item).toHaveText(new Date().toISOString()); // error: e2e-flaky new Date()
  await expect(item).toBeVisible({ timeout: 15_000 }); // error: e2e-flaky timeout:
  await item.click({ force: true }); // error: e2e-flaky force:true
}
