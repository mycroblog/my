// SPDX-License-Identifier: AGPL-3.0-only

// A post's own screen: the post, then its replies, in one feed named "Thread".

import type { Locator, Page } from "@playwright/test";
import { Timeline } from "./timeline";

export class Thread extends Timeline {
  constructor(page: Page) {
    super(page, page.getByRole("feed", { name: "Thread", exact: true }));
  }

  /** The screen's heading. */
  get heading(): Locator {
    return this.page.getByRole("main").getByRole("heading", { level: 1, name: "Post", exact: true });
  }

  /** Goes back to the screen the post was opened from. */
  async back(): Promise<void> {
    await this.page.getByRole("main").getByRole("button", { name: "Back", exact: true }).click();
  }
}
