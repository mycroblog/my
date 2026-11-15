// SPDX-License-Identifier: AGPL-3.0-only

// The screen of who reacted to a post with one emoji (/@user/status/id/reactions/emoji), which a row under the post on
// its own page opens: its heading, the title of the list ("Reacted with 😀" and the count), and a row per account,
// each a link to the account's profile named by its display name.

import type { Locator, Page } from "@playwright/test";

export class Reactors {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** The screen's heading. */
  get heading(): Locator {
    return this.page.getByRole("main").getByRole("heading", { level: 1, name: "Reactions", exact: true });
  }

  /** The title of the list: "Reacted with {emoji}", then how many reacted with it. */
  get title(): Locator {
    return this.page.getByRole("main").getByRole("heading", { level: 2 });
  }

  /** The row of the account whose display name is `name`, a link to their profile. */
  account(name: string): Locator {
    return this.page.getByRole("main").getByRole("link", { name, exact: true });
  }

  /** Goes back to the post's own page. */
  async back(): Promise<void> {
    await this.page.getByRole("main").getByRole("button", { name: "Back to the post", exact: true }).click();
  }
}
