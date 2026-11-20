// SPDX-License-Identifier: AGPL-3.0-only

// The administrators' announcements: Admin in the main navigation, its root's link to Announcements, the list at
// /admin/announcements and its Write an announcement, the form of one (its title, its text in the app's editor, when
// it shows), Publish and its question, and Withdraw and its question on an announcement's page; and the readers' list
// of announcements at /announcements, where a published one shows. Each write returns once the server answered it.

import type { Locator, Page } from "@playwright/test";
import { answered } from "./write";

export class AnnouncementsAdmin {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  private get main(): Locator {
    return this.page.getByRole("main");
  }

  /** Admin in the main navigation. */
  get place(): Locator {
    return this.page
      .getByRole("navigation", { name: "Main", exact: true })
      .getByRole("link", { name: /^Admin(?: .+)?$/ });
  }

  /** The root's link to the announcements, with its hint. */
  get entry(): Locator {
    return this.main
      .getByRole("navigation", { name: "Administration screens", exact: true })
      .getByRole("link", { name: /^Announcements .+$/ });
  }

  /** Opens the form of a new announcement from the list. */
  async write(): Promise<void> {
    await this.main.getByRole("link", { name: "Write an announcement", exact: true }).click();
  }

  /** Fills the form: its title and its text. */
  async fill(o: { title: string; text: string }): Promise<void> {
    await this.main.getByRole("textbox", { name: "Title", exact: true }).fill(o.title);
    const text = this.main.getByRole("textbox", { name: "Text", exact: true });
    await text.click();
    await this.page.keyboard.type(o.text);
  }

  /** Publishes the announcement the form holds, answering its question, and returns once the server answered. */
  async publish(): Promise<void> {
    await this.main.getByRole("button", { name: "Publish", exact: true }).click();
    const dialog = this.page.getByRole("alertdialog", { name: "Publish this announcement?", exact: true });
    await answered(this.page, { methods: ["POST", "PUT"], path: /\/admin\/announcements(?:\/[^/]+\/publish)?$/ }, () =>
      dialog.getByRole("button", { name: "Publish", exact: true }).click(),
    );
  }

  /** What the screen says a press did, in its words. */
  said(words: string): Locator {
    return this.main.getByText(words, { exact: true });
  }

  /** Withdraws the announcement whose page is open, answering its question, and returns once the server answered. */
  async withdraw(): Promise<void> {
    await this.main.getByRole("button", { name: "Withdraw", exact: true }).click();
    const dialog = this.page.getByRole("alertdialog", { name: "Withdraw this announcement?", exact: true });
    await answered(this.page, { methods: ["POST"], path: /\/admin\/announcements\/[^/]+\/withdraw$/ }, () =>
      dialog.getByRole("button", { name: "Withdraw", exact: true }).click(),
    );
  }

  /** The heading of an announcement's page, its title. */
  heading(title: string): Locator {
    return this.main.getByRole("heading", { name: title, exact: true });
  }

  /** The link to the announcement titled `title` in the readers' list at /announcements, on a reader's `page`. */
  listedOn(page: Page, title: string): Locator {
    return page.getByRole("main").getByRole("link", { name: title, exact: true });
  }
}
