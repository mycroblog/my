// SPDX-License-Identifier: AGPL-3.0-only

// The server's administration of its settings, rules and login providers: its root at /admin, a page of Server
// settings saved as a whole, the rules and a rule's page, and the login providers and a provider's page. Each write
// returns once the server has answered it.

import type { Locator, Page } from "@playwright/test";
import { answered } from "./write";

const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

export class Administration {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  private get main(): Locator {
    return this.page.getByRole("main");
  }

  /** Opens the root of the administration, /admin. */
  async open(): Promise<void> {
    await this.page.goto("/admin");
  }

  /** The list of the administration screens the reader's permissions open, at /admin and Server settings. */
  menu(name: "Administration screens" | "Server settings"): Locator {
    return this.main.getByRole("navigation", { name, exact: true });
  }

  /** Opens the screen of the link of `name` (its words, then its hint) of the list `menu`. */
  async go(menu: "Administration screens" | "Server settings", name: string): Promise<void> {
    await this.menu(menu)
      .getByRole("link", { name: new RegExp(`^${escape(name)} .+$`) })
      .click();
  }

  /** A field of the screen by its name: a text field, a number or a select. */
  field(name: string): Locator {
    return this.main.getByLabel(name, { exact: true });
  }

  /** A text field of the screen by its name, where a section is named as the field is. */
  textbox(name: string): Locator {
    return this.main.getByRole("textbox", { name, exact: true });
  }

  /** A button of the screen by its name. */
  button(name: string): Locator {
    return this.main.getByRole("button", { name, exact: true });
  }

  /** What the submit bar says once the form was saved. */
  get saved(): Locator {
    return this.page.getByRole("status").getByText("Saved", { exact: true });
  }

  /** Saves the settings' form, and returns once the server answered. */
  async saveSettings(): Promise<void> {
    await answered(this.page, { methods: ["PATCH"], path: /\/admin\/settings$/ }, async () => {
      await this.button("Save").click();
    });
  }

  /** A rule in the list of the rules in effect, by its text, at its place (`2. Be kind.`). */
  rule(text: string): Locator {
    return this.main.getByRole("link", { name: new RegExp(`^\\d+\\. ${escape(text)}(?: .+)?$`) });
  }

  /** The list of the rules in effect, in their order. */
  get inEffect(): Locator {
    return this.main.getByRole("list", { name: "In effect", exact: true });
  }

  /** What the list of the rules in effect holds where the rule `first` comes before the rule `then`, any between. */
  inOrder(first: string, then: string): RegExp {
    return new RegExp(`^[\\s\\S]*${escape(first)}[\\s\\S]*${escape(then)}[\\s\\S]*$`);
  }

  /** A rule in the list of the retired rules, by its text. */
  retired(text: string): Locator {
    return this.main
      .getByRole("region", { name: /^Retired \d+$/ })
      .getByRole("link", { name: new RegExp(`^${escape(text)}$`) });
  }

  /** Writes a new rule from the rules' screen, and returns once the server answered. */
  async addRule(text: string): Promise<void> {
    await this.main.getByRole("link", { name: "Add a rule", exact: true }).click();
    await this.textbox("Rule").fill(text);
    await answered(this.page, { methods: ["POST"], path: /\/admin\/rules$/ }, async () => {
      await this.button("Add a rule").click();
    });
  }

  /** Moves the open rule to the place `ordinal` (`1st`), saves, and returns once the server answered. */
  async moveRule(ordinal: string): Promise<void> {
    await this.field("Place").selectOption({ label: ordinal });
    await answered(this.page, { methods: ["POST"], path: /\/admin\/rules\/[^/]+\/move$/ }, async () => {
      await this.button("Save").click();
    });
  }

  /** Retires the open rule, confirming, and returns once the server answered. */
  async retireRule(): Promise<void> {
    await this.button("Retire").click();
    await answered(this.page, { methods: ["POST"], path: /\/admin\/rules\/[^/]+\/retire$/ }, async () => {
      await this.page.getByRole("alertdialog").getByRole("button", { name: "Retire", exact: true }).click();
    });
  }

  /** Sets up a GitHub login provider of `name` with a client ID and secret, and returns once the server answered. */
  async addGitHubProvider(name: string, clientId: string, secret: string): Promise<void> {
    await this.main.getByRole("link", { name: "Add a login provider", exact: true }).click();
    await this.main.getByRole("radio", { name: /^GitHub .+$/ }).click();
    await this.field("Name").fill(name);
    await this.field("Client ID").fill(clientId);
    await this.field("Client secret").fill(secret);
    await answered(this.page, { methods: ["POST"], path: /\/admin\/login_providers$/ }, async () => {
      await this.button("Add a login provider").click();
    });
  }

  /** The callback URL a provider's page gives to copy. */
  get callbackUrl(): Locator {
    return this.main.getByRole("list", { name: "Callback URL", exact: true }).getByRole("listitem");
  }

  /** A login provider in the list, by its name. */
  provider(name: string): Locator {
    return this.main.getByRole("link", { name: new RegExp(`^${escape(name)} .+$`) });
  }

  /** Deletes the open login provider, typing its name, and returns once the server answered. */
  async deleteProvider(name: string): Promise<void> {
    await this.button("Delete").click();
    const dialog = this.page.getByRole("alertdialog");
    await dialog.getByRole("textbox", { name: `To confirm, enter ${name}`, exact: true }).fill(name);
    await answered(this.page, { methods: ["DELETE"], path: /\/admin\/login_providers\/[^/]+$/ }, async () => {
      await dialog.getByRole("button", { name: "Delete", exact: true }).click();
    });
  }
}
