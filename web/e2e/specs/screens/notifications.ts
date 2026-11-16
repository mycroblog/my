// SPDX-License-Identifier: AGPL-3.0-only

// The notifications screen. A notification's article is named by what happened ("Yuki 雪 followed you"); a mention is
// the post that mentions the reader, named by its author.

import type { Locator, Page } from "@playwright/test";
import { answered } from "./write";

export class Notifications {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  get feed(): Locator {
    return this.page.getByRole("feed", { name: "Notifications", exact: true });
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

  /** Accepts the follow request of `from`, and returns once the server has answered. */
  async accept(from: string): Promise<void> {
    await answered(this.page, { methods: ["POST"], path: /\/follow_requests\/[^/]+\/accept$/ }, async () => {
      await this.acceptButton(from).click();
    });
  }

  /**
   * The row over the notifications that counts the replies, boosts and quotes waiting for the reader's approval ("2
   * waiting for your approval"), a link to their screen.
   */
  waitingRow(count: number): Locator {
    return this.page
      .getByRole("main")
      .getByRole("link", { name: `${String(count)} waiting for your approval`, exact: true });
  }

  /** Opens the screen of what waits for the reader's approval from the row that counts `count` of them. */
  async openWaiting(count: number): Promise<WaitingRequests> {
    await this.waitingRow(count).click();
    return new WaitingRequests(this.page);
  }
}

/** The write of an answer to a request: its approval or its refusal. */
const answer = /\/interaction_requests\/[^/]+\/(?:accept|reject)$/;

/**
 * The screen of the replies, boosts and quotes waiting for the reader's approval, under Notifications: each request an
 * article named by who asked for what ("Fresh 1a2b3c4d asked to reply to your post"), with Decline and Approve until it
 * is answered, and its answer after.
 */
export class WaitingRequests {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  get feed(): Locator {
    return this.page.getByRole("feed", { name: "Waiting for approval", exact: true });
  }

  /** The request of `name` (a display name) to do `what` ("reply to", "boost", "quote") with the reader's post. */
  request(name: string, what: "reply to" | "boost" | "quote"): Locator {
    return this.feed.getByRole("article", { name: new RegExp(`^⁨?${name}⁩? asked to ${what} your post$`) });
  }

  /** Approves (`Approve`) or declines (`Decline`) a request, and returns once the server has answered. */
  async answer(article: Locator, button: "Approve" | "Decline"): Promise<void> {
    await answered(this.page, { methods: ["POST"], path: answer }, async () => {
      await article.getByRole("button", { name: button, exact: true }).click();
    });
  }
}
