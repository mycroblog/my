// SPDX-License-Identifier: AGPL-3.0-only

// Notifications, the entry of the settings at /settings/notifications: the switch of Web Push on this device, the
// tables of kinds by channel, where a cell is a checkbox named by its row and its column ("Mentions Email"), and the
// policy for notifications from people the reader may not know, a choice per condition. Each control saves at once: an
// action here returns once the server has answered its write (write.ts).

import { expect, type Locator, type Page } from "@playwright/test";
import { answered } from "./write";

/** The tables of the entry, by their names. */
type Table = "From other people" | "For moderators" | "From this server";

/** A channel a notification reaches the reader by, a column of the tables. */
type Channel = "In the app" | "Push" | "Email" | "Sound";

/** What the policy does with the notifications of a condition, by the words of its choice and the server's value. */
const actions = { Accept: "accept", "Hold back": "filter", Drop: "drop" } as const;

export class NotificationSettings {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** The screen's heading, the entry's name. */
  get heading(): Locator {
    return this.page.getByRole("main").getByRole("heading", { level: 1, name: "Notifications", exact: true });
  }

  /** The table named `name`. */
  table(name: Table): Locator {
    return this.page.getByRole("main").getByRole("table", { name, exact: true });
  }

  /** Opens the entry at its address, and returns once its settings are read: the first table shows. */
  async goTo(): Promise<void> {
    await this.page.goto("/settings/notifications");
    await expect(this.heading).toBeVisible();
    await expect(this.table("From other people")).toBeVisible();
  }

  /**
   * The column of `table` named `channel`, by its header: one while the table has the column, none otherwise. Count
   * it, never ask whether it is visible: in a narrow column the headers give way to each cell's own channel name.
   */
  column(table: Table, channel: Channel): Locator {
    return this.table(table).getByRole("columnheader", { name: channel, exact: true });
  }

  /** The cell of `table` in the row of the kind named `kind` under `channel`: a checkbox. */
  cell(table: Table, kind: string, channel: Channel): Locator {
    return this.table(table).getByRole("checkbox", { name: `${kind} ${channel}`, exact: true });
  }

  /**
   * Presses the cell of the kind named `kind` under `channel`, which is at the other state, so that it is on (`on`) or
   * off, and returns once the server has answered. The app and the email are the account's; Push is this device's
   * subscription, and Sound this device's alone.
   */
  async setCell(table: Table, kind: string, channel: "In the app" | "Email", on: boolean): Promise<void> {
    const cell = this.cell(table, kind, channel);
    await expect(cell).toBeChecked({ checked: !on });
    await answered(this.page, { methods: ["PATCH"], path: /\/preferences\/notifications$/ }, async () => {
      await cell.click();
    });
    await expect(cell).toBeChecked({ checked: on });
  }

  /** The choice of what the policy does with the notifications of the condition named `condition`. */
  policy(condition: string): Locator {
    return this.page.getByRole("main").getByRole("combobox", { name: condition, exact: true });
  }

  /** The server's value of the action a choice of the policy reads as. */
  static valueOf(action: keyof typeof actions): string {
    return actions[action];
  }

  /** Chooses what the policy does with the condition named `condition`, and returns once the server has answered. */
  async choose(condition: string, action: keyof typeof actions): Promise<void> {
    await answered(this.page, { methods: ["PATCH"], path: /\/preferences\/notification_policy$/ }, async () => {
      await this.policy(condition).selectOption({ label: action });
    });
  }

  /** The switch of Web Push on this device. */
  get push(): Locator {
    return this.page.getByRole("main").getByRole("switch", { name: "Push notifications", exact: true });
  }

  /**
   * Turns Web Push on this device on (the server registers the subscription) or off (it deletes it), and returns once
   * the server has answered. The switch is pressed by its label: its input is not what a pointer meets.
   */
  async setPush(on: boolean): Promise<void> {
    await answered(this.page, { methods: [on ? "PUT" : "DELETE"], path: /\/push\/subscription$/ }, async () => {
      await this.page.getByRole("main").getByText("Push notifications", { exact: true }).click();
    });
  }
}
