// SPDX-License-Identifier: AGPL-3.0-only

// Search, at /search: a field (a searchbox named Search), then what it finds by kind, the accounts a list named People,
// each row with its follow; and finding people, at /people: the suggested accounts, a list named Find people, each row
// with its follow.

import type { Locator, Page } from "@playwright/test";
import { answered } from "./write";

/** The row of the account named `name` (a display name) in `list`, and its follow. */
function rowOf(list: Locator, name: string): { row: Locator; follow: Locator; following: Locator } {
  const row = list.getByRole("listitem").filter({ has: list.page().getByRole("link", { name }) });
  return {
    row,
    follow: row.getByRole("button", { name: "Follow", exact: true }),
    following: row.getByText("Following", { exact: true }),
  };
}

/** Follows with `follow`, and returns once the server has answered. */
async function follow(page: Page, button: Locator): Promise<void> {
  await answered(page, { methods: ["POST"], path: /\/accounts\/[^/]+\/follow$/ }, async () => {
    await button.click();
  });
}

export class Search {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** Opens the search at its address. */
  async goTo(): Promise<void> {
    await this.page.goto("/search");
  }

  /** The field. */
  get field(): Locator {
    return this.page.getByRole("main").getByRole("searchbox", { name: "Search", exact: true });
  }

  /** The accounts found. */
  get people(): Locator {
    return this.page.getByRole("main").getByRole("list", { name: "People", exact: true });
  }

  /** Writes `words` in the field. */
  async find(words: string): Promise<void> {
    await this.field.fill(words);
  }

  /** The row of the account named `name` among those found, and its follow. */
  account(name: string): ReturnType<typeof rowOf> {
    return rowOf(this.people, name);
  }

  /** Follows the account named `name` from the results, and returns once the server has answered. */
  async follow(name: string): Promise<void> {
    await follow(this.page, this.account(name).follow);
  }
}

export class People {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** Opens finding people at its address. */
  async goTo(): Promise<void> {
    await this.page.goto("/people");
  }

  /** The suggested accounts. */
  get suggested(): Locator {
    return this.page.getByRole("main").getByRole("list", { name: "Find people", exact: true });
  }

  /** The row of the suggested account named `name`, and its follow. */
  account(name: string): ReturnType<typeof rowOf> {
    return rowOf(this.suggested, name);
  }

  /** Follows the suggested account named `name`, and returns once the server has answered. */
  async follow(name: string): Promise<void> {
    await follow(this.page, this.account(name).follow);
  }
}
