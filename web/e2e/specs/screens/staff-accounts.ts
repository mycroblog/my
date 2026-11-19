// SPDX-License-Identifier: AGPL-3.0-only

// The moderators' screens of accounts: the search of accounts at /moderation/accounts and an account's Review, where a
// moderator takes an action; the account's actions; the queue of appeals at /moderation/appeals and an appeal, upheld
// or overturned; and the registrations that wait at /moderation/registrations, approved or rejected. Each write
// returns once the server has answered it.

import { expect, type Locator, type Page } from "@playwright/test";
import { answered } from "./write";

const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

/** A row of the table `table` whose head starts with `head` (an account's handle). */
const rowOf = (page: Page, table: Locator, head: string): Locator =>
  table
    .getByRole("row")
    .filter({ has: page.getByRole("rowheader", { name: new RegExp(`^${escape(head)}(?: .+)?$`) }) });

export class StaffAccounts {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  private get main(): Locator {
    return this.page.getByRole("main");
  }

  /** The root's link to the screen of `name` (Accounts, Appeals, Registrations), its hint after its name. */
  entry(name: string): Locator {
    return this.main
      .getByRole("navigation", { name: "Moderation screens", exact: true })
      .getByRole("link", { name: new RegExp(`^${escape(name)} .+$`) });
  }

  /** What the screen says a press did, in its words. */
  said(words: string): Locator {
    return this.main.getByText(words, { exact: true });
  }

  private button(name: string): Locator {
    return this.main.getByRole("button", { name, exact: true });
  }

  /** Searches the accounts by the start of `username`, and waits for the server's answer. */
  async search(username: string): Promise<void> {
    const read = this.page.waitForResponse(
      (r) =>
        r.request().method() === "GET" &&
        new URL(r.url()).pathname.endsWith("/admin/accounts") &&
        new URL(r.url()).searchParams.get("username") === username,
    );
    await this.main.getByRole("searchbox", { name: "Search by username", exact: true }).fill(username);
    await read;
  }

  /** The table of the accounts found. */
  get accounts(): Locator {
    return this.main.getByRole("table", { name: "Accounts", exact: true });
  }

  /** The row of the account of `handle` (`@user`) in the accounts found. */
  account(handle: string): Locator {
    return rowOf(this.page, this.accounts, handle);
  }

  /** Opens the account of `handle` from the accounts found. */
  async openAccount(handle: string): Promise<void> {
    await this.account(handle)
      .getByRole("link", { name: new RegExp(`^${escape(handle)}(?: .+)?$`) })
      .click();
    await expect(this.main.getByRole("heading", { level: 1, name: handle, exact: true })).toBeVisible();
  }

  /** Chooses `option` of the choice labelled `label` in a form. */
  private async choose(label: string, option: string): Promise<void> {
    await this.main.getByRole("combobox", { name: label, exact: true }).selectOption({ label: option });
  }

  /** Takes an action of `kind` (its words: No posting) for `lasts` (7 days), told with `reason`; returns once answered. */
  async takeAction(o: { kind: string; lasts: string; reason: string }): Promise<void> {
    await this.choose("Action", o.kind);
    await this.choose("Lasts", o.lasts);
    await this.main.getByRole("textbox", { name: "Reason the account is told", exact: true }).fill(o.reason);
    await answered(this.page, { methods: ["POST"], path: /\/admin\/accounts\/[^/]+\/actions$/ }, async () => {
      await this.button("Take this action").click();
    });
  }

  /** Opens the tab of the account's page of `name` (Review, Actions, Management). */
  async tab(name: string): Promise<void> {
    await this.main.getByRole("tab", { name, exact: true }).click();
  }

  /** The table of an account's actions. */
  get actions(): Locator {
    return this.main.getByRole("table", { name: "Actions", exact: true });
  }

  /** The row of the action of `kind` (its words) among the account's actions. */
  action(kind: string): Locator {
    return rowOf(this.page, this.actions, kind);
  }

  /** The table of the appeals. */
  get appeals(): Locator {
    return this.main.getByRole("table", { name: "Appeals", exact: true });
  }

  /** The row of the appeal of the account of `handle`. */
  appeal(handle: string): Locator {
    return rowOf(this.page, this.appeals, handle);
  }

  /** Opens the appeal of the account of `handle` from the queue. */
  async openAppeal(handle: string): Promise<void> {
    await this.appeal(handle)
      .getByRole("link", { name: new RegExp(`^${escape(handle)}(?: .+)?$`) })
      .click();
    await expect(this.main.getByRole("heading", { level: 1, name: `Appeal of ${handle}`, exact: true })).toBeVisible();
  }

  /** Decides the appeal shown (`Uphold` or `Overturn`) with `reason`, asked first; returns once the server answered. */
  async decideAppeal(decision: "Uphold" | "Overturn", reason: string): Promise<void> {
    await this.main.getByRole("textbox", { name: "Reason you tell them", exact: true }).fill(reason);
    await this.button(decision).click();
    const dialog = this.page.getByRole("alertdialog");
    await answered(this.page, { methods: ["POST"], path: /\/admin\/appeals\/[^/]+\/(uphold|overturn)$/ }, async () => {
      await dialog.getByRole("button", { name: decision, exact: true }).click();
    });
  }

  /** The table of the registrations that wait. */
  get registrations(): Locator {
    return this.main.getByRole("table", { name: "Registrations", exact: true });
  }

  /** The row of the registration of `handle`. */
  registration(handle: string): Locator {
    return rowOf(this.page, this.registrations, handle);
  }

  /** Approves the registration of `handle`, asked first; returns once the server answered. */
  async approve(handle: string): Promise<void> {
    const pick = this.registration(handle).getByRole("checkbox", { name: `Select ${handle}`, exact: true });
    await pick.click();
    await expect(pick).toBeChecked();
    await this.button("Approve").click();
    const dialog = this.page.getByRole("alertdialog");
    await answered(this.page, { methods: ["POST"], path: /\/admin\/registrations\/[^/]+\/approve$/ }, async () => {
      await dialog.getByRole("button", { name: "Approve", exact: true }).click();
    });
  }
}
