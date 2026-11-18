// SPDX-License-Identifier: AGPL-3.0-only

// Relationships in the settings: the entry at /settings/relationships, whose four sections list the accounts the
// reader blocks and mutes, the servers they mute and the hashtags they follow, each row with the button that takes it
// out of its list. A row undone stays where it was, saying what was done, until the reader leaves the screen.

import { expect, type Locator, type Page } from "@playwright/test";
import { answered } from "./write";

/** A list of the entry, by the name of its section. */
type List = "Blocked accounts" | "Muted accounts" | "Muted servers" | "Followed hashtags";

/** What takes a row out of each list: its button, the word the row then shows, and the write it sends. */
const undoing = {
  "Blocked accounts": { button: "Unblock", done: "Unblocked", path: /\/accounts\/[^/]+\/block$/ },
  "Muted accounts": { button: "Unmute", done: "Unmuted", path: /\/accounts\/[^/]+\/mute$/ },
  "Muted servers": { button: "Unmute", done: "Unmuted", path: /\/domain_mutes\/[^/]+$/ },
  "Followed hashtags": { button: "Unfollow", done: "Unfollowed", path: /\/tags\/[^/]+\/follow$/ },
} as const satisfies Record<List, { button: string; done: string; path: RegExp }>;

export class Relationships {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** The screen's heading, the entry's name. */
  get heading(): Locator {
    return this.page.getByRole("main").getByRole("heading", { level: 1, name: "Relationships", exact: true });
  }

  /** The section of `list`, whose heading says its count after its name once the list is not empty. */
  section(list: List): Locator {
    return this.page.getByRole("main").getByRole("region", { name: new RegExp(`^${list}(?:\\s?[\\d,]+)?$`) });
  }

  /** Opens the entry at its address, and returns once its lists are read: its sections are drawn. */
  async goTo(): Promise<void> {
    await this.page.goto("/settings/relationships");
    await this.read();
  }

  /** Returns once the entry is drawn with its lists read. */
  async read(): Promise<void> {
    await expect(this.heading).toBeVisible();
    await expect(this.section("Followed hashtags")).toBeVisible();
  }

  /** The row of `list` of the account whose display name is `name`, which links to its profile. */
  account(list: "Blocked accounts" | "Muted accounts", name: string): Locator {
    return this.section(list)
      .getByRole("listitem")
      .filter({ has: this.page.getByRole("link", { name, exact: true }) });
  }

  /**
   * The button that takes a row out of `list`, in the group that `named` names: an account's handle (`@sam`, with its
   * server for one of another server), a server's domain, or a hashtag's name.
   */
  undoButton(list: List, named: string): Locator {
    return this.section(list)
      .getByRole("group", { name: named, exact: true })
      .getByRole("button", { name: undoing[list].button, exact: true });
  }

  /** Takes the row that `named` names out of `list` with its button, and returns once the server has answered. */
  async undo(list: List, named: string): Promise<void> {
    await answered(this.page, { methods: ["DELETE"], path: undoing[list].path }, async () => {
      await this.undoButton(list, named).click();
    });
  }

  /** The word a row of `list` shows in place of its button once it was taken out: "Unblocked", "Unmuted", "Unfollowed". */
  done(row: Locator, list: List): Locator {
    return row.getByText(undoing[list].done, { exact: true });
  }

  /** The `sentence` the section of `list` says in place of its rows while the list is empty. */
  empty(list: List, sentence: string): Locator {
    return this.section(list).getByText(sentence, { exact: true });
  }
}
