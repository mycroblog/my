// SPDX-License-Identifier: AGPL-3.0-only

// Keyword filters in the settings: the list at /settings/filters, whose rows lead to a filter's editor; the editor of a
// new filter at /settings/filters/new, where the name, what the filter does to a post, its words and its places are
// saved together with Add; and a post of a feed that a filter folds, under the filter's name with Show.

import { expect, type Locator, type Page } from "@playwright/test";
import type { Timeline } from "./timeline";
import { answered } from "./write";

const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

/** The list of the reader's filters. */
export class KeywordFilters {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** The screen's heading, the entry's name. */
  get heading(): Locator {
    return this.page.getByRole("main").getByRole("heading", { level: 1, name: "Keyword filters", exact: true });
  }

  /** The row of the filter named `name`, a link to its editor whose name goes on with what the filter does. */
  row(name: string): Locator {
    return this.page
      .getByRole("main")
      .getByRole("list", { name: "Filters", exact: true })
      .getByRole("link", { name: new RegExp(`^${escape(name)}\\s.+$`) });
  }
}

/** The editor of a new filter. */
export class FilterEditor {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** The screen's heading. */
  get heading(): Locator {
    return this.page.getByRole("main").getByRole("heading", { level: 1, name: "New filter", exact: true });
  }

  /** Opens the editor of a new filter at its address, and returns once it is drawn. */
  async goTo(): Promise<void> {
    await this.page.goto("/settings/filters/new");
    await expect(this.heading).toBeVisible();
    await expect(this.name).toBeVisible();
  }

  /** The field of the filter's name, which a post it folds shows. */
  get name(): Locator {
    return this.page.getByRole("main").getByRole("textbox", { name: "Name", exact: true });
  }

  /** The field of the word or phrase numbered `n` (from 1); a new filter starts with one. */
  word(n: number): Locator {
    return this.page.getByRole("main").getByRole("textbox", { name: `Word or phrase ${String(n)}`, exact: true });
  }

  /** The choice of what the filter does to a post that matches. */
  get action(): Locator {
    return this.page.getByRole("main").getByRole("combobox", { name: "Posts that match", exact: true });
  }

  /** Adds the filter with Add, and returns once the server has answered; the app then shows the list of filters. */
  async add(): Promise<void> {
    await answered(this.page, { methods: ["POST"], path: /\/filters$/ }, async () => {
      await this.page.getByRole("main").getByRole("button", { name: "Add", exact: true }).click();
    });
  }
}

/**
 * The posts of `timeline` that the filter named `filter` folds: each an article named by the line that stands in the
 * post's place, with Show to open it.
 */
export function foldedBy(timeline: Timeline, filter: string): Locator {
  return timeline.feed.getByRole("article", { name: new RegExp(`^Matches filter “\\s?${escape(filter)}\\s?”$`) });
}
