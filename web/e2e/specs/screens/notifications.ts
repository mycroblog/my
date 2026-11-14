// SPDX-License-Identifier: AGPL-3.0-only

// The notifications screen. A notification's article is named by what happened ("Yuki 雪 followed you"); a mention is
// the post that mentions the reader, named by its author.

import type { Locator, Page } from "@playwright/test";

export class Notifications {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  get feed(): Locator {
    return this.page.getByRole("feed", { name: "Notifications" });
  }

  /** The notification named `what`, as the screen says it ("Yuki 雪 favourited your post"). */
  notification(what: string): Locator {
    return this.feed.getByRole("article", { name: what, exact: true });
  }

  /** The notification of the post by `author` (a display name) that mentions the reader and holds `text`. */
  mention(author: string, text: string): Locator {
    return this.feed
      .getByRole("article", { name: author, exact: true })
      .filter({ hasText: `${author} mentioned you` })
      .filter({ hasText: text });
  }

  /** The follow request of `from` (a display name). */
  followRequest(from: string): Locator {
    return this.notification(`${from} requested to follow you`);
  }

  /** The button of the follow request of `from` that accepts it; gone once the request is decided. */
  acceptButton(from: string): Locator {
    return this.followRequest(from).getByRole("button", { name: "Accept", exact: true });
  }

  /** Accepts the follow request of `from`. */
  async accept(from: string): Promise<void> {
    await this.acceptButton(from).click();
  }
}
