// SPDX-License-Identifier: AGPL-3.0-only

// The reader's drafts and scheduled posts, which the composer opens in its place: the scheduled posts first, then the
// drafts, each a row that opens it in the composer and a menu of what else it does.

import { expect, type Locator, type Page } from "@playwright/test";
import { Composer } from "./composer";
import { answered } from "./write";

/** An item of a row's menu. */
type RowAction = "Open" | "Schedule…" | "Change time…" | "Return to drafts" | "Delete…";

export class Drafts {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  get dialog(): Locator {
    return this.page.getByRole("dialog", { name: "Drafts and scheduled posts", exact: true });
  }

  /** The heading of a section, which counts its rows ("Scheduled 2", "Drafts 3"). */
  heading(name: string | RegExp): Locator {
    return this.dialog.getByRole("heading", { name, exact: true });
  }

  /** The scheduled posts, soonest first. */
  get scheduled(): Locator {
    return this.dialog.getByRole("list", { name: "Scheduled posts", exact: true });
  }

  /** The drafts without a schedule, newest first. */
  get drafts(): Locator {
    return this.dialog.getByRole("list", { name: "Drafts", exact: true });
  }

  /** The row of `list` (the drafts, by default) that holds `text`. */
  row(text: string, list: Locator = this.drafts): Locator {
    return list.getByRole("listitem").filter({ hasText: text });
  }

  /** The button of a row that opens it, named "Open this draft" and what the row shows. */
  opener(text: string, list: Locator = this.drafts): Locator {
    return this.row(text, list).getByRole("button").filter({ hasText: text });
  }

  /**
   * Opens the row of `list` that holds `text` in the composer, and returns it: titled New post, or Reply for a draft of
   * a reply.
   */
  async open(text: string, list: Locator = this.drafts, title: "New post" | "Reply" = "New post"): Promise<Composer> {
    await this.opener(text, list).click();
    return new Composer(this.page.getByRole("dialog", { name: title, exact: true }));
  }

  /** Chooses `action` of the menu of the row of `list` that holds `text`. */
  async menu(text: string, action: RowAction, list: Locator = this.drafts): Promise<void> {
    await this.row(text, list).getByRole("button", { name: "More", exact: true }).click();
    await this.page
      .getByRole("menu", { name: "More", exact: true })
      .getByRole("menuitem", { name: action, exact: true })
      .click();
  }

  /** Opens the scheduled post that holds `text` in the composer from Change time…, with its time's form open. */
  async changeTime(text: string): Promise<Composer> {
    await this.menu(text, "Change time…", this.scheduled);
    const composer = new Composer(this.page.getByRole("dialog", { name: "New post", exact: true }));
    await expect(composer.scheduleForm).toBeVisible();
    return composer;
  }

  /** Takes the scheduled post that holds `text` back to the drafts, and returns once the server has answered. */
  async unschedule(text: string): Promise<void> {
    await answered(this.page, { methods: ["DELETE"], path: /\/drafts\/[^/]+\/schedule$/ }, async () => {
      await this.menu(text, "Return to drafts", this.scheduled);
    });
  }

  /** Deletes the row of `list` that holds `text` after its confirmation, and returns once the server has answered. */
  async delete(text: string, list: Locator = this.drafts): Promise<void> {
    await this.menu(text, "Delete…", list);
    const confirm = this.page.getByRole("alertdialog", { name: "Delete this draft?", exact: true });
    await answered(this.page, { methods: ["DELETE"], path: /\/drafts\/[^/]+$/ }, async () => {
      await confirm.getByRole("button", { name: "Delete", exact: true }).click();
    });
  }

  /** Closes the list with its Close button. */
  async close(): Promise<void> {
    await this.dialog.getByRole("button", { name: "Close", exact: true }).click();
    await expect(this.dialog).toBeHidden();
  }
}
