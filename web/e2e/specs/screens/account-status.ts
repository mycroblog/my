// SPDX-License-Identifier: AGPL-3.0-only

// The account's status in the settings: the entry Account and data at /settings/account, whose summary says that a
// moderation action is in effect and leads to Account status at /settings/account/status; the actions listed there,
// each a row that leads to its own screen; and that screen, where the reader appeals the action.

import { expect, type Locator, type Page } from "@playwright/test";
import { answered } from "./write";

export class AccountStatus {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** The row of the settings' menu `menu` that leads to the entry, whose name its count follows while there is one. */
  row(menu: Locator): Locator {
    return menu.getByRole("link", { name: /^Account and data(?: .+)?$/ });
  }

  /**
   * What a settings menu counts on its rows, as assistive technology reads it ("1 item to check"): the actions in
   * effect the reader can still appeal are the only thing a member's menu counts.
   */
  counts(menu: Locator): Locator {
    return menu.getByText(/^\d+ items? to check$/);
  }

  /** The settings' menus of the page: the menu's own screen, or the one beside an entry's screen. */
  get menus(): Locator {
    return this.page.getByRole("navigation", { name: "Settings", exact: true });
  }

  /** The heading of the screen in place, named `name`. */
  heading(name: "Account and data" | "Account status" | "Moderation action"): Locator {
    return this.page.getByRole("main").getByRole("heading", { level: 1, name, exact: true });
  }

  /** Follows the row of `menu` to the entry, and returns once the entry shows. */
  async open(menu: Locator): Promise<void> {
    await this.row(menu).click();
    await expect(this.heading("Account and data")).toBeVisible();
  }

  /** The entry's summary while one moderation action is in effect. */
  get summary(): Locator {
    return this.page
      .getByRole("main")
      .getByRole("region", { name: "A moderation action is in effect on your account", exact: true });
  }

  /** Follows the summary's way to Account status, and returns once it shows. */
  async seeStatus(): Promise<void> {
    await this.summary.getByRole("link", { name: "See account status", exact: true }).click();
    await expect(this.heading("Account status")).toBeVisible();
  }

  /** The row of Account status of the action in effect that the moderators explained with `reason`. */
  inEffect(reason: string): Locator {
    return this.page
      .getByRole("main")
      .getByRole("list", { name: "Actions in effect", exact: true })
      .getByRole("link")
      .filter({ has: this.page.getByText(reason, { exact: true }) });
  }

  /** Follows the row of the action explained with `reason` to its screen, and returns once it shows. */
  async openAction(reason: string): Promise<void> {
    await this.inEffect(reason).click();
    await expect(this.heading("Moderation action")).toBeVisible();
  }

  /** The section of an action's screen titled `name`. */
  section(name: "What this action does" | "The moderators' reason" | "Appeal"): Locator {
    return this.page.getByRole("main").getByRole("region", { name, exact: true });
  }

  /** The field of the appeal, there while the action can be appealed. */
  get appeal(): Locator {
    return this.page.getByRole("main").getByRole("textbox", { name: "Your appeal", exact: true });
  }

  /** The status of an action's screen that says the appeal was sent. */
  get sent(): Locator {
    return this.page
      .getByRole("main")
      .getByRole("status")
      .filter({ hasText: /^Your appeal was sent\.$/ });
  }

  /** Writes `text` as the appeal and sends it, and returns once the server has answered and the screen says so. */
  async sendAppeal(text: string): Promise<void> {
    await this.appeal.fill(text);
    await answered(this.page, { methods: ["POST"], path: /\/moderation\/actions\/[^/]+\/appeal$/ }, async () => {
      await this.page.getByRole("main").getByRole("button", { name: "Send appeal", exact: true }).click();
    });
    await expect(this.sent).toBeVisible();
  }

  /** Follows an action's way back to Account status, and returns once it shows. */
  async backToStatus(): Promise<void> {
    await this.page.getByRole("main").getByRole("button", { name: "Account status", exact: true }).click();
    await expect(this.heading("Account status")).toBeVisible();
  }
}
