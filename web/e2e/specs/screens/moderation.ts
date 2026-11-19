// SPDX-License-Identifier: AGPL-3.0-only

// The moderators' screens: Moderation in the main navigation, its root's list of the screens the reader's permissions
// open, the queue of reports at /moderation/reports, and a report's screen, where a moderator takes it, writes a
// note, resolves it and reopens it. Each write returns once the server has answered it.

import { expect, type Locator, type Page } from "@playwright/test";
import { answered } from "./write";

const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

/** A write on a report: `/admin/reports/{id}/<what>`. */
const reportWrite = (what: string) =>
  ({ methods: ["POST"], path: new RegExp(`/admin/reports/[^/]+/${what}$`) }) as const;

export class Moderation {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** Moderation in the main navigation, whose name the words of what waits there follow ("Moderation 3 waiting"). */
  get place(): Locator {
    return this.page
      .getByRole("navigation", { name: "Main", exact: true })
      .getByRole("link", { name: /^Moderation(?: \d+ waiting)?$/ });
  }

  /** The list of the moderation screens the reader's permissions open, at /moderation. */
  get screens(): Locator {
    return this.page.getByRole("main").getByRole("navigation", { name: "Moderation screens", exact: true });
  }

  /** The root's link to the queue of reports, its hint and, while reports are open, how many after its name. */
  get reportsLink(): Locator {
    return this.screens.getByRole("link", { name: /^Reports .+$/ });
  }

  /** The table of the queue of reports. */
  get queue(): Locator {
    return this.page.getByRole("main").getByRole("table", { name: "Reports", exact: true });
  }

  /** The row of the queue whose report is of the account of `handle` (`@user`). */
  row(handle: string): Locator {
    return this.queue
      .getByRole("row")
      .filter({ has: this.page.getByRole("rowheader", { name: new RegExp(`^${escape(handle)}(?: .+)?$`) }) });
  }

  /** Opens the report of the account of `handle` from the queue. */
  async open(handle: string): Promise<void> {
    await this.row(handle)
      .getByRole("link", { name: new RegExp(`^${escape(handle)}(?: .+)?$`) })
      .click();
  }

  /** The heading of a report's screen, which names the account it reports. */
  reportHeading(handle: string): Locator {
    return this.page.getByRole("main").getByRole("heading", { level: 1, name: `Report on ${handle}`, exact: true });
  }

  /** The facts of the report on its screen. */
  get facts(): Locator {
    return this.page.getByRole("main").getByLabel("About the report", { exact: true });
  }

  /** What the screen says a press did, in its words. */
  said(words: string): Locator {
    return this.page.getByRole("main").getByText(words, { exact: true });
  }

  private button(name: string): Locator {
    return this.page.getByRole("main").getByRole("button", { name, exact: true });
  }

  /** Takes the report, and returns once the server answered. */
  async assignToMe(): Promise<void> {
    await answered(this.page, reportWrite("assign_to_self"), async () => {
      await this.button("Assign to me").click();
    });
  }

  /** Writes `body` as a note, and returns once the server answered. */
  async addNote(body: string): Promise<void> {
    await this.page.getByRole("main").getByRole("textbox", { name: "New note", exact: true }).fill(body);
    await answered(this.page, reportWrite("notes"), async () => {
      await this.button("Add note").click();
    });
  }

  /** The note of the report whose words are `body`. */
  note(body: string): Locator {
    return this.page
      .getByRole("main")
      .getByRole("list", { name: "Notes", exact: true })
      .getByRole("listitem")
      .filter({ hasText: body });
  }

  /** Resolves the report with `outcome`, and returns once the server answered. */
  async resolve(outcome: "Action taken" | "No action needed"): Promise<void> {
    await this.button("Resolve").click();
    await answered(this.page, reportWrite("resolve"), async () => {
      await this.page.getByRole("menuitem", { name: outcome, exact: true }).click();
    });
  }

  /** Puts the report back in the queue, and returns once the server answered. */
  async reopen(): Promise<void> {
    await answered(this.page, reportWrite("reopen"), async () => {
      await this.button("Reopen").click();
    });
  }

  /** The way back from a report to the queue. */
  async back(): Promise<void> {
    await this.button("Reports").click();
    await expect(this.queue).toBeVisible();
  }
}
