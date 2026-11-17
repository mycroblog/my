// SPDX-License-Identifier: AGPL-3.0-only

// A person's profile: the region named by their display name, with the follow button, and the feed of their posts.

import type { Locator, Page } from "@playwright/test";
import type { Person } from "../../src/scenario";
import { Timeline } from "./timeline";
import { answered } from "./write";

const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

export class Profile {
  readonly page: Page;
  readonly person: Person;

  constructor(page: Page, person: Person) {
    this.page = page;
    this.person = person;
  }

  /** Opens the profile at its address, /@username. */
  async open(): Promise<void> {
    await this.page.goto(`/@${this.person.username}`);
  }

  /** The screen's heading: the display name and the full handle. */
  get heading(): Locator {
    return this.page.getByRole("main").getByRole("heading", {
      level: 1,
      name: new RegExp(`^${escape(this.person.displayName)} \\(@${this.person.username}@[^)]+\\)$`),
    });
  }

  /** The profile's card. */
  get card(): Locator {
    return this.page.getByRole("region", { name: this.person.displayName, exact: true });
  }

  /** The button that follows the person; "Follow back" when they follow the reader. */
  get followButton(): Locator {
    return this.card.getByRole("button", { name: /^Follow( back)?$/ });
  }

  /** The button that shows the reader follows the person, and unfollows them. */
  get followingButton(): Locator {
    return this.card.getByRole("button", { name: "Following", exact: true });
  }

  /** Follows the person, and returns once the server has answered. */
  async follow(): Promise<void> {
    await answered(this.page, { methods: ["POST"], path: /\/accounts\/[^/]+\/follow$/ }, async () => {
      await this.followButton.click();
    });
  }

  /** Unfollows the person, and returns once the server has answered. */
  async unfollow(): Promise<void> {
    await answered(this.page, { methods: ["DELETE"], path: /\/accounts\/[^/]+\/follow$/ }, async () => {
      await this.followingButton.click();
    });
  }

  /** The checkbox of the reader's list `name` in the submenu Add to list, checked while the list holds the person. */
  listChoice(name: string): Locator {
    return this.page
      .getByRole("menu", { name: "Add to list", exact: true })
      .getByRole("menuitemcheckbox", { name, exact: true });
  }

  /**
   * Puts the person in the reader's list `name` from the card's More, Add to list, and returns once the server has
   * answered, the submenu still open.
   */
  async addToList(name: string): Promise<void> {
    await this.card.getByRole("button", { name: "More", exact: true }).click();
    await this.page
      .getByRole("menu", { name: "More", exact: true })
      .getByRole("menuitem", { name: "Add to list", exact: true })
      .click();
    await answered(this.page, { methods: ["POST"], path: /\/lists\/[^/]+\/accounts\/[^/]+$/ }, async () => {
      await this.listChoice(name).click();
    });
  }

  /** The tabs of the profile's posts, in their order. */
  get tabs(): Locator {
    return this.page
      .getByRole("main")
      .getByRole("tablist", { name: "Posts of the account", exact: true })
      .getByRole("tab");
  }

  /** The tab of the profile named `name` ("Posts", "Media", "Articles"…). */
  tab(name: string): Locator {
    return this.tabs.and(this.page.getByRole("tab", { name, exact: true }));
  }

  /** The person's posts. */
  get posts(): Timeline {
    return new Timeline(this.page, this.page.getByRole("feed", { name: "Posts of the account", exact: true }));
  }
}
