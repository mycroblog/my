// SPDX-License-Identifier: AGPL-3.0-only

// The notice at the bottom of the page that says what a write did.

import type { Locator, Page } from "@playwright/test";

export class Notice {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** The notice whose words are `text`. */
  text(text: string): Locator {
    return this.page.getByText(text, { exact: true });
  }
}
