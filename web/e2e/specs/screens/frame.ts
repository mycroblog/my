// SPDX-License-Identifier: AGPL-3.0-only

// The frame around every screen of a signed-in person: the main navigation, the screen's heading, the button that
// opens the composer, and the menu that holds the places the navigation does not (on a phone, the tab bar's last item).

import type { Locator, Page } from "@playwright/test";
import { Composer } from "./composer";

/** A place the main navigation links to. */
export type Place = "Home" | "Notifications" | "Profile";

/** A place the menu holds. */
export type MenuPlace = "Profile" | "Bookmarks" | "Settings" | "Announcements" | "Events";

/** The menu open over the screen: the places the navigation does not hold, and what is around the reader. */
export class Menu {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  get dialog(): Locator {
    return this.page.getByRole("dialog", { name: "Menu", exact: true });
  }

  /** The menu's link to `place`. */
  link(place: MenuPlace): Locator {
    return this.dialog.getByRole("link", { name: place, exact: true });
  }

  /** Follows the menu's link to `place`. */
  async goTo(place: MenuPlace): Promise<void> {
    await this.link(place).click();
  }

  /** Closes the menu with its Close button. */
  async close(): Promise<void> {
    await this.dialog.getByRole("button", { name: "Close", exact: true }).click();
  }
}

export class Frame {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** The main navigation; its name is in the reader's language. */
  get navigation(): Locator {
    return this.page.getByRole("navigation", { name: "Main", exact: true });
  }

  /**
   * The main navigation's link to `place`, by its English name, which the words of its count follow while it has one
   * (Notifications, "3 unread").
   */
  link(place: Place): Locator {
    return this.navigation.getByRole("link", { name: new RegExp(`^${place}(?: \\d+ unread)?$`) });
  }

  /** Follows the main navigation's link to `place`. */
  async goTo(place: Place): Promise<void> {
    await this.link(place).click();
  }

  /** The main navigation's button that opens the menu (a reader without staff rights: nothing in it is marked). */
  get menuButton(): Locator {
    return this.navigation.getByRole("button", { name: "Menu", exact: true });
  }

  /** Opens the menu. */
  async openMenu(): Promise<Menu> {
    await this.menuButton.click();
    return new Menu(this.page);
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
