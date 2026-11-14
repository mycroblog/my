// SPDX-License-Identifier: AGPL-3.0-only

// The server's announcements: below the width where the column beside the main one shows them, the one line above the
// home timeline that counts the unread and links to their list; and the list, at /announcements, each announcement
// an entry named by its title, a link to its page, marked while unread.

import type { Locator, Page } from "@playwright/test";
import { answered } from "./write";

/** The line above the home timeline, named by how many announcements are unread. */
export class HomeAnnouncements {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** The line, whatever the count. */
  get line(): Locator {
    return this.page.getByRole("main").getByRole("link", { name: /^\d+ unread announcements?$/ });
  }

  /** The line while `count` announcements are unread. */
  unread(count: number): Locator {
    const name = `${String(count)} unread ${count === 1 ? "announcement" : "announcements"}`;
    return this.page.getByRole("main").getByRole("link", { name, exact: true });
  }

  /** Follows the line to the list of announcements. */
  async openList(): Promise<AnnouncementList> {
    await this.line.click();
    return new AnnouncementList(this.page);
  }
}

/** The list of the server's announcements, at /announcements. */
class AnnouncementList {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** The announcement of `title`. */
  entry(title: string): Locator {
    return this.page.getByRole("main").getByRole("article", { name: title, exact: true });
  }

  /** The announcement of `title`, marked unread. */
  unread(title: string): Locator {
    return this.entry(title).getByRole("img", { name: "Unread", exact: true });
  }

  /** Opens the announcement of `title`, and returns once the server has answered marking it read. */
  async open(title: string): Promise<void> {
    await answered(this.page, { methods: ["POST"], path: /\/announcements\/[^/]+\/read$/ }, async () => {
      await this.entry(title).getByRole("link", { name: title, exact: true }).click();
    });
  }
}
