// SPDX-License-Identifier: AGPL-3.0-only

// The landing page, which a visitor without a session sees at the server's address: the server's name as its heading,
// the account's actions, and the places a visitor may go to without an account.

import type { Locator, Page } from "@playwright/test";

/** A place the landing page leads a visitor to without an account, by its button's English words. */
export type LandingPlace = "Read the local timeline" | "Read the federated timeline" | "Search";

export class Landing {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** Opens the server's address without a session. */
  async open(): Promise<void> {
    await this.page.goto("/");
  }

  /** The page's heading, the server's name. */
  heading(name: string): Locator {
    return this.page.getByRole("banner").getByRole("heading", { level: 1, name, exact: true });
  }

  /** The action that opens the server's registration. */
  get createAccount(): Locator {
    return this.page.getByRole("main").getByRole("link", { name: "Create an account", exact: true });
  }

  /** The action that signs in. */
  get signIn(): Locator {
    return this.page.getByRole("main").getByRole("link", { name: "Sign in", exact: true });
  }

  /** The link to a place a visitor may go to without an account. */
  place(place: LandingPlace): Locator {
    return this.page
      .getByRole("region", { name: "Without an account", exact: true })
      .getByRole("link", { name: place, exact: true });
  }
}
