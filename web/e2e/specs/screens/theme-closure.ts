// SPDX-License-Identifier: AGPL-3.0-only

// The server's theme and its closure in the administration: the theme's screen at /admin/theme with its draft's form,
// its preview and its publications, and the closure's screen at /admin/closure with the announcement's form. Each
// write returns once the server has answered it.

import type { Locator, Page } from "@playwright/test";
import { answered } from "./write";

export class ThemeAdmin {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  private get main(): Locator {
    return this.page.getByRole("main");
  }

  /** Opens the theme's screen. */
  async open(): Promise<void> {
    await this.page.goto("/admin/theme");
  }

  /** A button of the screen by its name. */
  button(name: string): Locator {
    return this.main.getByRole("button", { name, exact: true });
  }

  /** Opens the draft's form from the theme's screen, whether a draft is saved or not. */
  async editDraft(): Promise<void> {
    await this.main.getByRole("link", { name: /^(Edit the draft|Start a draft)$/ }).click();
  }

  /** The field of the color `name` in `scheme`, as #rrggbb, on the draft's form. */
  color(name: string, scheme: "Light" | "Dark"): Locator {
    return this.main
      .getByRole("group", { name, exact: true })
      .getByRole("textbox", { name: `${name} ${scheme}`, exact: true });
  }

  /** A field of a token that is no color, by its name, on the draft's form. */
  textbox(name: string): Locator {
    return this.main.getByRole("textbox", { name, exact: true });
  }

  /** What the submit bar says once the draft was saved, and what a press of the theme's screen said. */
  get saved(): Locator {
    return this.page.getByRole("status").getByText("Saved", { exact: true });
  }

  /** What a section of the theme's screen says once its press was done (`Published.`, `Restored.`). */
  said(words: "Published." | "Restored."): Locator {
    return this.main.getByRole("status").getByText(new RegExp(`^${words.replace(".", "\\.")} .+$`));
  }

  /** Saves the draft, and returns once the server answered. */
  async saveDraft(): Promise<void> {
    await answered(this.page, { methods: ["PUT"], path: /\/admin\/theme\/draft$/ }, async () => {
      await this.button("Save").click();
    });
  }

  /** Publishes the draft, and returns once the server answered. */
  async publish(): Promise<void> {
    await answered(this.page, { methods: ["POST"], path: /\/admin\/theme\/publish$/ }, async () => {
      await this.button("Publish").click();
    });
  }

  /** Restores the theme published before the one in effect, and returns once the server answered. */
  async restore(): Promise<void> {
    await answered(this.page, { methods: ["POST"], path: /\/admin\/theme\/restore$/ }, async () => {
      await this.button("Restore the previous theme").click();
    });
  }

  /** The notice on every screen while the draft is previewed. */
  get previewNotice(): Locator {
    return this.page
      .getByRole("region", { name: "Server and connection status", exact: true })
      .getByText("This tab shows the draft theme.", { exact: true });
  }

  /** Goes to the home timeline by the app's own navigation, as a reader would. */
  async goHome(): Promise<void> {
    await this.page
      .getByRole("navigation", { name: "Main", exact: true })
      .getByRole("link", { name: "Home", exact: true })
      .click();
  }

  /** Goes back to the theme's screen by the link of the preview's notice. */
  async backToTheme(): Promise<void> {
    await this.page
      .getByRole("region", { name: "Server and connection status", exact: true })
      .getByRole("link", { name: "Theme", exact: true })
      .click();
  }

  /** The value of the custom property `name` on the page's root, as the stylesheets in effect give it. */
  async property(name: string): Promise<string> {
    return (
      await this.page.evaluate((p) => getComputedStyle(document.documentElement).getPropertyValue(p), name)
    ).trim();
  }
}

export class ClosureAdmin {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  private get main(): Locator {
    return this.page.getByRole("main");
  }

  /** Opens the closure's screen. */
  async open(): Promise<void> {
    await this.page.goto("/admin/closure");
  }

  /** A button of the screen by its name. */
  button(name: string): Locator {
    return this.main.getByRole("button", { name, exact: true });
  }

  /** Opens the announcement's form, to announce the closure or change its dates. */
  async openAnnouncement(): Promise<void> {
    await this.main.getByRole("link", { name: /^(Announce the closure|Change the dates)$/ }).click();
  }

  /** The field of the closing date and time, as the browser holds it (2027-01-31T15:00). */
  get closes(): Locator {
    return this.main.getByLabel("Closing date", { exact: true });
  }

  /** The field of the message to the users. */
  get message(): Locator {
    return this.main.getByRole("textbox", { name: "Message to the users", exact: true });
  }

  /**
   * Sends the announcement and confirms it, and returns once the server answered: Announce for a new one, Save where
   * one is in effect already (a run before this one that did not withdraw it).
   */
  async announce(): Promise<void> {
    const send = /^(Announce|Save)$/;
    await this.main.getByRole("button", { name: send }).click();
    await answered(this.page, { methods: ["PUT"], path: /\/admin\/closure\/announcement$/ }, async () => {
      await this.page.getByRole("alertdialog").getByRole("button", { name: send }).click();
    });
  }

  /** Withdraws the announcement and confirms it, and returns once the server answered. */
  async withdraw(): Promise<void> {
    await this.button("Withdraw").click();
    await answered(this.page, { methods: ["DELETE"], path: /\/admin\/closure\/announcement$/ }, async () => {
      await this.page.getByRole("alertdialog").getByRole("button", { name: "Withdraw", exact: true }).click();
    });
  }

  /** The facts of the announcement in effect. */
  get announcement(): Locator {
    return this.main.getByRole("region", { name: "Announcement", exact: true });
  }

  /** The notice of the closure on every screen. */
  get banner(): Locator {
    return this.page
      .getByRole("region", { name: "Server and connection status", exact: true })
      .getByText(/^This server closes on .+$/);
  }
}
