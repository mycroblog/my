// SPDX-License-Identifier: AGPL-3.0-only

// The screens of the server that a password reset link leads to: the one that takes the new password, and the one
// that says the password is changed and leads to the sign-in screen.

import type { Locator, Page } from "@playwright/test";

export class PasswordReset {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** Opens the link. */
  async open(link: string): Promise<void> {
    await this.page.goto(link);
  }

  get heading(): Locator {
    return this.page.getByRole("main").getByRole("heading", { level: 1, name: "Set a new password", exact: true });
  }

  /** Sets `password` as the new password. */
  async setPassword(password: string): Promise<void> {
    await this.page.getByLabel("New password", { exact: true }).fill(password);
    await this.page.getByRole("main").getByRole("button", { name: "Set the password", exact: true }).click();
  }

  /** The heading of the screen that follows the change. */
  get changed(): Locator {
    return this.page.getByRole("main").getByRole("heading", { level: 1, name: "Password changed", exact: true });
  }

  /** Follows that screen's link to the sign-in screen. */
  async toSignIn(): Promise<void> {
    await this.page.getByRole("main").getByRole("link", { name: "Sign in", exact: true }).click();
  }
}
