// SPDX-License-Identifier: AGPL-3.0-only

// The custom emoji: the settings' Custom emoji of a person who may ask for emoji (/settings/custom-emojis), where
// they ask for one and find their requests and their emoji; and the administration's Custom emoji, whose tab Requests
// (/admin/custom-emojis/requests) lists the requests that wait, each opened over the list to approve or reject. Each
// write returns once the server has answered it.

import { expect, type Locator, type Page } from "@playwright/test";
import type { Upload } from "./article-editor";
import { answered } from "./write";

const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

/** The settings' Custom emoji, and the form of a request under it. */
export class OwnEmoji {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** The menu's row to the entry, whose name the row's hint follows. */
  get menuRow(): Locator {
    return this.page
      .getByRole("main")
      .getByRole("navigation", { name: "Settings", exact: true })
      .getByRole("link", { name: /^Custom emoji.+$/ });
  }

  /** The entry's heading. */
  get heading(): Locator {
    return this.page.getByRole("main").getByRole("heading", { level: 1, name: "Custom emoji", exact: true });
  }

  /** Opens the entry from the settings' menu. */
  async open(): Promise<void> {
    await this.page.goto("/settings");
    await this.menuRow.click();
    await expect(this.heading).toBeVisible();
  }

  /** The section of the person's requests. */
  get requests(): Locator {
    return this.page.getByRole("main").getByRole("region", { name: "Your requests", exact: true });
  }

  /** The section of the emoji the person made. */
  get emojis(): Locator {
    return this.page.getByRole("main").getByRole("region", { name: "Your emoji", exact: true });
  }

  /** The row of the request for `shortcode` (without colons). */
  request(shortcode: string): Locator {
    return this.requests.getByRole("listitem").filter({ hasText: new RegExp(`:${escape(shortcode)}:`) });
  }

  /** The row of the person's emoji of `shortcode` (without colons). */
  emoji(shortcode: string): Locator {
    return this.emojis.getByRole("link", { name: new RegExp(`^:${escape(shortcode)}:(?: .+)?$`) });
  }

  /** Asks for an emoji of `shortcode` with `image`, and returns once the server took the request. */
  async ask(shortcode: string, image: Upload): Promise<void> {
    await this.page.getByRole("main").getByRole("link", { name: "Request an emoji", exact: true }).click();
    const main = this.page.getByRole("main");
    await main.getByRole("textbox", { name: "Shortcode", exact: true }).fill(shortcode);
    const chooser = this.page.waitForEvent("filechooser");
    await main.getByRole("button", { name: "Choose an image", exact: true }).click();
    await (await chooser).setFiles(image);
    await answered(this.page, { methods: ["POST"], path: /\/custom_emoji_requests$/ }, async () => {
      await main.getByRole("button", { name: "Send request", exact: true }).click();
    });
    await expect(this.heading).toBeVisible();
  }
}

/** The administration's Custom emoji, at its tab Requests. */
export class EmojiRequests {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** Opens Custom emoji from the administration's root, then its tab Requests. */
  async open(): Promise<void> {
    await this.page.goto("/admin");
    await this.page
      .getByRole("main")
      .getByRole("navigation", { name: "Administration screens", exact: true })
      .getByRole("link", { name: /^Custom emoji .+$/ })
      .click();
    await this.page
      .getByRole("main")
      .getByRole("tab", { name: /^Requests(?: \d+\+?)?$/ })
      .click();
    await expect(this.table).toBeVisible();
  }

  /** The table of the requests. */
  get table(): Locator {
    return this.page.getByRole("main").getByRole("table", { name: "Requests for custom emoji", exact: true });
  }

  /** The row of the request for `shortcode` (without colons). */
  row(shortcode: string): Locator {
    return this.table
      .getByRole("row")
      .filter({ has: this.page.getByRole("rowheader", { name: new RegExp(`^:${escape(shortcode)}:(?: .+)?$`) }) });
  }

  /** Opens the request for `shortcode` over the list, approves it, and returns once the server answered. */
  async approve(shortcode: string): Promise<void> {
    await this.row(shortcode)
      .getByRole("button", { name: new RegExp(`^:${escape(shortcode)}:(?: .+)?$`) })
      .click();
    const dialog = this.page.getByRole("dialog", { name: `:${shortcode}:`, exact: true });
    await answered(
      this.page,
      { methods: ["POST"], path: /\/admin\/custom_emoji_requests\/[^/]+\/approve$/ },
      async () => {
        await dialog.getByRole("button", { name: "Approve", exact: true }).click();
      },
    );
  }

  /** What the screen says a press did, in its words. */
  said(words: string): Locator {
    return this.page.getByRole("main").getByText(words, { exact: true });
  }
}
