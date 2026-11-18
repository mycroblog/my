// SPDX-License-Identifier: AGPL-3.0-only

// The screens of the server that the link of a verification mail leads to: the one that asks the person to confirm
// their email address, and the one that says it is confirmed.

import type { Locator, Page } from "@playwright/test";

export class VerifyEmail {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** Opens the link of the mail. */
  async open(link: string): Promise<void> {
    await this.page.goto(link);
  }

  get heading(): Locator {
    return this.page
      .getByRole("main")
      .getByRole("heading", { level: 1, name: "Confirm your email address", exact: true });
  }

  /** Confirms the email address. */
  async confirm(): Promise<void> {
    await this.page.getByRole("main").getByRole("button", { name: "Confirm email address", exact: true }).click();
  }

  /** The heading of the screen that follows the confirmation. */
  get confirmed(): Locator {
    return this.page.getByRole("main").getByRole("heading", { level: 1, name: "Email address confirmed", exact: true });
  }

  /** What that screen says of the confirmation. */
  get outcome(): Locator {
    return this.page.getByRole("main").getByRole("status");
  }
}
