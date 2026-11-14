// SPDX-License-Identifier: AGPL-3.0-only

// A person's profile: the region named by their display name, with the follow button, their note and fields, and
// the feed of their posts.

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

  /** The paragraph of the card that holds `text`, such as the person's note. */
  paragraph(text: string): Locator {
    return this.card.getByRole("paragraph").filter({ hasText: text });
  }

  /** The name of the profile field `name`. */
  fieldName(name: string): Locator {
    return this.card.getByRole("term").filter({ hasText: name });
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

  /** The person's posts. */
  get posts(): Timeline {
    return new Timeline(this.page, this.page.getByRole("feed", { name: "Posts of the account", exact: true }));
  }
}
