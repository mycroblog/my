// SPDX-License-Identifier: AGPL-3.0-only

// A person's profile: the region named by their display name, with the follow button, and the feed of their posts.

import type { Locator, Page } from "@playwright/test";
import type { Person } from "../../src/scenario";
import { Timeline } from "./timeline";
import { answered } from "./write";

const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

/**
 * The heading of the profile of the person of this server whose username is `username`, whatever their display name:
 * it tells whose account a signed-in person's own profile (the navigation's Profile) is.
 */
export function ownProfileHeading(page: Page, username: string): Locator {
  return page
    .getByRole("main")
    .getByRole("heading", { level: 1, name: new RegExp(`^.+ \\(@${escape(username)}@[^)]+\\)$`) });
}

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

  /**
   * The pictures of the card, each a decoration beside the name that says whose it is: the header and the avatar of a
   * person who has them, none for one who has neither.
   */
  get pictures(): Locator {
    return this.card.getByRole("presentation", { includeHidden: true });
  }

  /** The link on the reader's own profile that leads to the form that edits it. */
  get editProfile(): Locator {
    return this.card.getByRole("link", { name: "Edit profile", exact: true });
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

  /** The button on the card of a person the reader blocks, which lifts the block. */
  get unblockButton(): Locator {
    return this.card.getByRole("button", { name: "Unblock", exact: true });
  }

  /** Opens the card's More, and returns its item named `name`. */
  async moreItem(name: string): Promise<Locator> {
    await this.card.getByRole("button", { name: "More", exact: true }).click();
    return this.page.getByRole("menu", { name: "More", exact: true }).getByRole("menuitem", { name, exact: true });
  }

  /**
   * Blocks the person from the card's More, Block… and its confirmation, which asks by the person's handle, and
   * returns once the server has answered.
   */
  async block(): Promise<void> {
    await (await this.moreItem("Block…")).click();
    // The handle stands in the question between the marks that isolate its direction (U+2068, U+2069).
    const handle = `${String.fromCodePoint(0x2068)}?@${escape(this.person.username)}${String.fromCodePoint(0x2069)}?`;
    const asking = this.page.getByRole("alertdialog", { name: new RegExp(`^Block ${handle}\\?$`) });
    await answered(this.page, { methods: ["POST"], path: /\/accounts\/[^/]+\/block$/ }, async () => {
      await asking.getByRole("button", { name: "Block", exact: true }).click();
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
