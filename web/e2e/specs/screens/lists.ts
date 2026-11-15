// SPDX-License-Identifier: AGPL-3.0-only

// The reader's lists, which the menu leads to, each a link to its timeline; and a list's screen: its name, and the
// posts of its members in one feed named "Posts from the list {name}".

import type { Locator, Page } from "@playwright/test";
import { Timeline } from "./timeline";

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
}

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
}
