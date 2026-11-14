// SPDX-License-Identifier: AGPL-3.0-only

// The frame around every screen of a signed-in person: the main navigation, the screen's heading, and the button that
// opens the composer.

import type { Locator, Page } from "@playwright/test";
import { Composer } from "./composer";

/** A place the main navigation links to. */
export type Place = "Home" | "Notifications" | "Profile";

export class Frame {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** The main navigation; its name is in the reader's language. */
  get navigation(): Locator {
    return this.page.getByRole("navigation", { name: "Main", exact: true });
  }

  /** The main navigation's link to `place`, by its English name. */
  link(place: Place): Locator {
    return this.navigation.getByRole("link", { name: place, exact: true });
  }

  /** Follows the main navigation's link to `place`. */
  async goTo(place: Place): Promise<void> {
    await this.link(place).click();
  }

  /** The language of the document (`<html lang>`), the language the client speaks to the reader in. */
  async language(): Promise<string> {
    return this.page.evaluate(() => document.documentElement.lang);
  }

  /** The heading of the screen, which names it. */
  heading(name: string | RegExp): Locator {
    return this.page.getByRole("main").getByRole("heading", { level: 1, name, exact: true });
  }

  /** Opens the composer of a new post. */
  async newPost(): Promise<Composer> {
    await this.navigation.getByRole("button", { name: "New post", exact: true }).click();
    return new Composer(this.page.getByRole("dialog", { name: "New post", exact: true }));
  }
}
