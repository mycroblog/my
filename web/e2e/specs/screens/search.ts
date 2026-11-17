// SPDX-License-Identifier: AGPL-3.0-only

// Search, at /search: a field (a searchbox named Search), then what it finds by kind, the hashtags a list named
// Hashtags; and the suggested people over the home of a reader who follows no one.

import type { Locator, Page } from "@playwright/test";

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

  /** The hashtags found. */
  get hashtags(): Locator {
    return this.page.getByRole("main").getByRole("list", { name: "Hashtags", exact: true });
  }

  /** The link of the hashtag `#name` among those found, which names it with the count of its posts. */
  hashtag(name: string, posts: string): Locator {
    return this.hashtags.getByRole("link", { name: new RegExp(`^#${name}\\s*${posts}$`, "u") });
  }

  /** Writes `words` in the field. */
  async find(words: string): Promise<void> {
    await this.field.fill(words);
  }
}

/** The suggested people over the home of a reader who follows no one, a list named Suggested for you. */
export function homeSuggestions(page: Page): Locator {
  return page.getByRole("main").getByRole("list", { name: "Suggested for you", exact: true });
}
