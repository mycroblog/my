// SPDX-License-Identifier: AGPL-3.0-only

// The reader's lists, which the menu leads to, each a link to its timeline, and New list at their top; and a list's
// screen: its name, the menu of its title row, List options (Rename, Members, Delete list…), and the posts of its
// members in one feed named "Posts from the list {name}"; and a list's members, a list of rows, each with a Remove
// named by whom it takes out ("Remove Sam").

import { expect, type Locator, type Page } from "@playwright/test";
import { Timeline } from "./timeline";
import { answered } from "./write";

const escaped = (text: string) => text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

export class ListIndex {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** The screen's heading. */
  get heading(): Locator {
    return this.page.getByRole("main").getByRole("heading", { level: 1, name: "Lists", exact: true });
  }

  /** The index of the reader's lists. */
  get index(): Locator {
    return this.page.getByRole("navigation", { name: "Your lists", exact: true });
  }

  /** The link to the list named `name`, which says how many members it has ("Bread and coffee 2 members"). */
  list(name: string): Locator {
    return this.index.getByRole("link", { name: new RegExp(`^${escaped(name)} \\d+ members?$`) });
  }

  /** Opens the list named `name`. */
  async open(name: string): Promise<ListScreen> {
    await this.list(name).click();
    return new ListScreen(this.page, name);
  }

  /** Opens the index at its address. */
  async goTo(): Promise<void> {
    await this.page.goto("/lists");
    await expect(this.heading).toBeVisible();
  }

  /**
   * Makes a list named `name` with New list, whose name's field opens in place at the index's top, and returns once
   * the server has answered.
   */
  async create(name: string): Promise<void> {
    const main = this.page.getByRole("main");
    await main.getByRole("button", { name: "New list", exact: true }).click();
    await main.getByRole("textbox", { name: "List name", exact: true }).fill(name);
    await answered(this.page, { methods: ["POST"], path: /\/lists$/ }, async () => {
      await main.getByRole("button", { name: "Create", exact: true }).click();
    });
  }
}

/** An item of a list's menu. */
type ListAction = "Rename" | "Members" | "Delete list…";

export class ListScreen extends Timeline {
  readonly name: string;

  constructor(page: Page, name: string) {
    super(page, page.getByRole("feed", { name: `Posts from the list ${name}`, exact: true }));
    this.name = name;
  }

  /** The screen's heading, the list's name. */
  get heading(): Locator {
    return this.page.getByRole("main").getByRole("heading", { level: 1, name: this.name, exact: true });
  }

  /** Opens the screen of the list with the ID given, at its address. */
  async open(id: string): Promise<void> {
    await this.page.goto(`/lists/${id}`);
  }

  /** What the screen says in place of a list that does not exist. */
  get gone(): Locator {
    return this.page.getByRole("main").getByText("This list no longer exists.", { exact: true });
  }

  /** The list's own menu button at the end of its title, named apart from every post's More. */
  get options(): Locator {
    return this.page.getByRole("main").getByRole("button", { name: "List options", exact: true });
  }

  /** Chooses `action` of the list's menu, List options. */
  async menu(action: ListAction): Promise<void> {
    await this.options.click();
    await this.page
      .getByRole("menu", { name: "List options", exact: true })
      .getByRole("menuitem", { name: action, exact: true })
      .click();
  }

  /** Renames the list in place to `name` with Rename and Save, and returns its screen under the new name once saved. */
  async rename(name: string): Promise<ListScreen> {
    await this.menu("Rename");
    const main = this.page.getByRole("main");
    const field = main.getByRole("textbox", { name: "List name", exact: true });
    await expect(field).toHaveValue(this.name);
    await field.fill(name);
    await answered(this.page, { methods: ["PATCH"], path: /\/lists\/[^/]+$/ }, async () => {
      await main.getByRole("button", { name: "Save", exact: true }).click();
    });
    return new ListScreen(this.page, name);
  }

  /** Deletes the list after its confirmation, and returns once the server has answered. */
  async delete(): Promise<void> {
    await this.menu("Delete list…");
    const confirm = this.page.getByRole("alertdialog", { name: `Delete the list ${this.name}?`, exact: true });
    await answered(this.page, { methods: ["DELETE"], path: /\/lists\/[^/]+$/ }, async () => {
      await confirm.getByRole("button", { name: "Delete", exact: true }).click();
    });
  }
}

/** A list's members, at /lists/{id}/members: a list named Members, each row with its Remove. */
export class ListMembers {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** The list of the members. */
  get members(): Locator {
    return this.page.getByRole("main").getByRole("list", { name: "Members", exact: true });
  }

  /** The button that takes `name` (a display name) out of the list. */
  removeButton(name: string): Locator {
    return this.members.getByRole("button", { name: `Remove ${name}`, exact: true });
  }

  /** Takes `name` out of the list, and returns once the server has answered. */
  async remove(name: string): Promise<void> {
    await answered(this.page, { methods: ["DELETE"], path: /\/lists\/[^/]+\/accounts\/[^/]+$/ }, async () => {
      await this.removeButton(name).click();
    });
  }
}
