// SPDX-License-Identifier: AGPL-3.0-only

// The welcome step of a new account at /welcome, which the server's screen of a confirmed email address leads to: a
// display name and an avatar, both optional, then the home timeline.

import { expect, type Locator, type Page } from "@playwright/test";
import { answered } from "./write";

export class Welcome {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** The step's heading, which welcomes the reader to the server, whatever its name. */
  get heading(): Locator {
    return this.page.getByRole("main").getByRole("heading", { level: 1, name: /^Welcome to .+$/ });
  }

  /** Follows the way on of the server's screen that says the email address is confirmed. */
  async arrive(): Promise<void> {
    await this.page.getByRole("main").getByRole("link", { name: "Continue", exact: true }).click();
    await expect(this.heading).toBeVisible();
    await expect(this.page).toHaveURL(/\/welcome$/);
  }

  /** Gives `name` as the display name and continues, and returns once the server has saved it. */
  async continueAs(name: string): Promise<void> {
    await this.page.getByRole("main").getByRole("textbox", { name: "Display name", exact: true }).fill(name);
    await answered(this.page, { methods: ["PATCH"], path: /\/accounts\/update_credentials$/ }, async () => {
      await this.page.getByRole("main").getByRole("button", { name: "Continue", exact: true }).click();
    });
  }

  /** Skips the step. */
  async skip(): Promise<void> {
    await this.page.getByRole("main").getByRole("button", { name: "Skip for now", exact: true }).click();
  }
}
