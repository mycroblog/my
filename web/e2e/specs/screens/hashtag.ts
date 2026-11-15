// SPDX-License-Identifier: AGPL-3.0-only

// A hashtag's screen, reached from a hashtag in a post: its title with Follow at the end, then its posts in one feed
// named "Posts with #{name}". A visitor reads it too.

import type { Locator, Page } from "@playwright/test";
import { Timeline } from "./timeline";
import { answered } from "./write";

/** Following or unfollowing a hashtag. */
const follow = { methods: ["POST", "DELETE"], path: /\/tags\/[^/]+\/follow$/ } as const;

export class Hashtag extends Timeline {
  /** The hashtag without `#`, as the screen writes it. */
  readonly name: string;

  constructor(page: Page, name: string) {
    super(page, page.getByRole("feed", { name: `Posts with #${name}`, exact: true }));
    this.name = name;
  }

  /** The screen's heading, the hashtag with its `#`. */
  get heading(): Locator {
    return this.page.getByRole("main").getByRole("heading", { level: 1, name: `#${this.name}`, exact: true });
  }

  /** The button that follows the hashtag, while the reader does not. */
  get followButton(): Locator {
    return this.page.getByRole("main").getByRole("button", { name: "Follow", exact: true });
  }

  /** The button that stops following the hashtag, while the reader follows it. */
  get followingButton(): Locator {
    return this.page.getByRole("main").getByRole("button", { name: "Following", exact: true });
  }

  /** Opens the hashtag's screen at its address. */
  async open(): Promise<void> {
    await this.page.goto(`/tags/${encodeURIComponent(this.name)}`);
  }

  /** Follows the hashtag, and returns once the server has answered. */
  async follow(): Promise<void> {
    await answered(this.page, follow, async () => {
      await this.followButton.click();
    });
  }

  /** Stops following the hashtag, and returns once the server has answered. */
  async unfollow(): Promise<void> {
    await answered(this.page, follow, async () => {
      await this.followingButton.click();
    });
  }
}
