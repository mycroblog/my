// SPDX-License-Identifier: AGPL-3.0-only

// The app's Service Worker in the E2E tests (ADR-0028 §3): waiting until it controls a page, so that what the page
// loads next goes through it.

import type { Page } from "@playwright/test";

/** Resolves once the app's Service Worker is active and controls `page`. */
export async function controlledBy(page: Page): Promise<void> {
  await page.waitForFunction(async () => {
    await navigator.serviceWorker.ready;
    return navigator.serviceWorker.controller !== null;
  });
}
