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
    return this.page.getByRole("navigation", { name: "Main" });
  }

  /** Follows the main navigation's link to `place`. */
  async goTo(place: Place): Promise<void> {
    await this.navigation.getByRole("link", { name: place, exact: true }).click();
  }

  /** The heading of the screen, which names it. */
  heading(name: string | RegExp): Locator {
    return this.page.getByRole("main").getByRole("heading", { level: 1, name, exact: true });
  }

  /** Opens the composer of a new post. */
  async newPost(): Promise<Composer> {
    await this.navigation.getByRole("button", { name: "New post" }).click();
    return new Composer(this.page.getByRole("dialog", { name: "New post" }));
  }
}
