// SPDX-License-Identifier: AGPL-3.0-only

// The screens under the settings' entry Account and data that a person's data and account go through: Import, where a
// CSV file is chosen, previewed and imported; Aliases, where the account names one it moves from; Move account, where
// it moves to another and cancels the move; and Deactivate account. Each is at an address of its own; an act that
// writes returns once the server has answered it (write.ts).

import { expect, type Locator, type Page } from "@playwright/test";
import { answered } from "./write";

const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

export class AccountData {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** The screen's own part. */
  get main(): Locator {
    return this.page.getByRole("main");
  }

  /** The heading of the screen in place, named `name`. */
  heading(name: "Import" | "Aliases" | "Move account" | "Deactivate account"): Locator {
    return this.main.getByRole("heading", { level: 1, name, exact: true });
  }

  /** The section of the screen titled `name`. */
  section(name: string | RegExp): Locator {
    return this.main.getByRole("region", { name, exact: typeof name === "string" });
  }

  // ---- Import -------------------------------------------------------------------------------------------------------

  /** Opens Import at its address. */
  async openImport(): Promise<void> {
    await this.page.goto("/settings/account/import");
    await expect(this.heading("Import")).toBeVisible();
  }

  /** Chooses a CSV file named `name` that holds `text`, as the reader picks it in the browser's file chooser. */
  async chooseFile(name: string, text: string): Promise<void> {
    const chooser = this.page.waitForEvent("filechooser");
    await this.section("File")
      .getByRole("button", { name: /^Choose (a|another) file$/ })
      .click();
    await (await chooser).setFiles({ name, mimeType: "text/csv", buffer: Buffer.from(text) });
  }

  /** The lines of the preview of the file chosen. */
  get preview(): Locator {
    return this.section("Preview").getByRole("listitem");
  }

  /** Starts the import of the file chosen, and returns once the server has answered. */
  async startImport(): Promise<void> {
    await answered(this.page, { methods: ["POST"], path: /\/imports$/ }, async () => {
      await this.main.getByRole("button", { name: "Import", exact: true }).click();
    });
  }

  /** The rows of the recent imports, the newest first. */
  get recentImports(): Locator {
    return this.section("Recent imports")
      .getByRole("list", { name: "Recent imports", exact: true })
      .getByRole("listitem");
  }

  // ---- Aliases ------------------------------------------------------------------------------------------------------

  /** Opens Aliases at its address. */
  async openAliases(): Promise<void> {
    await this.page.goto("/settings/account/aliases");
    await expect(this.heading("Aliases")).toBeVisible();
  }

  /** Adds the account of `handle` as an alias, and returns once the server has answered and the screen says so. */
  async addAlias(handle: string): Promise<void> {
    await this.main.getByRole("textbox", { name: "Account you are moving from", exact: true }).fill(handle);
    await answered(this.page, { methods: ["POST"], path: /\/accounts\/aliases$/ }, async () => {
      await this.section("Add an alias").getByRole("button", { name: "Add", exact: true }).click();
    });
    await expect(this.section("Add an alias").getByRole("status")).toHaveText("The alias was added.");
  }

  // ---- Move account -------------------------------------------------------------------------------------------------

  /** Opens Move account at its address. */
  async openMove(): Promise<void> {
    await this.page.goto("/settings/account/move");
    await expect(this.heading("Move account")).toBeVisible();
  }

  /** The field that names the account to move to. */
  get target(): Locator {
    return this.main.getByRole("textbox", { name: "Account you are moving to", exact: true });
  }

  /** The confirmation of a move to the account of `username`, whatever the domain its handle shows. */
  moveConfirmation(username: string): Locator {
    return this.page.getByRole("dialog", {
      name: new RegExp(`^Move this account to @${escape(username)}(@[^?]+)?\\?$`),
    });
  }

  /**
   * Moves to the account of `username`, confirmed with `password`, and returns once the server has answered the move,
   * taken or refused.
   */
  async move(username: string, password: string): Promise<void> {
    await this.target.fill(username);
    await this.main.getByRole("button", { name: "Move account", exact: true }).click();
    const dialog = this.moveConfirmation(username);
    await dialog.getByLabel("Current password", { exact: true }).fill(password);
    await answered(this.page, { methods: ["POST"], path: /\/accounts\/move$/ }, async () => {
      await dialog.getByRole("button", { name: "Move account", exact: true }).click();
    });
  }

  /** The summary of an account that moved to the account of `username`. */
  moved(username: string): Locator {
    return this.section(new RegExp(`^This account moved to @${escape(username)}(@.+)?$`));
  }

  /** Cancels the move with its confirmation, and returns once the server has answered. */
  async cancelMove(): Promise<void> {
    await this.section("Cancelling the move").getByRole("button", { name: "Cancel the move", exact: true }).click();
    const asking = this.page.getByRole("alertdialog");
    await answered(this.page, { methods: ["DELETE"], path: /\/accounts\/move$/ }, async () => {
      await asking.getByRole("button", { name: "Cancel the move", exact: true }).click();
    });
  }

  // ---- Deactivate account -------------------------------------------------------------------------------------------

  /** Opens Deactivate account at its address. */
  async openDeactivate(): Promise<void> {
    await this.page.goto("/settings/account/deactivate");
    await expect(this.heading("Deactivate account")).toBeVisible();
  }

  /** Deactivates the account, confirmed with `password`, and returns once the server has answered. */
  async deactivate(password: string): Promise<void> {
    await this.main.getByRole("button", { name: "Deactivate account", exact: true }).click();
    const dialog = this.page.getByRole("dialog", { name: "Deactivate this account?", exact: true });
    await dialog.getByLabel("Current password", { exact: true }).fill(password);
    await answered(this.page, { methods: ["POST"], path: /\/accounts\/deactivation$/ }, async () => {
      await dialog.getByRole("button", { name: "Deactivate account", exact: true }).click();
    });
  }
}
