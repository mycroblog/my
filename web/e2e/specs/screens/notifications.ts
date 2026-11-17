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

  /** Opens what the notification named `what` tells of, by its line's link. */
  async open(what: string): Promise<void> {
    await this.notification(what).getByRole("link", { name: what, exact: true }).click();
  }

  /** Opens the post by `author` that mentions the reader and holds `text`, by its time ("2m ago"), a link to it. */
  async openMention(author: string, text: string): Promise<void> {
    await this.mention(author, text)
      .getByRole("link", { name: /^(?:.+ ago|now)$/ })
      .click();
  }

  /** Dismisses the notification named `what`, and returns once the server has answered. */
  async dismiss(what: string): Promise<void> {
    await answered(this.page, { methods: ["POST"], path: /\/notifications\/[^/]+\/dismiss$/ }, async () => {
      await this.notification(what).getByRole("button", { name: "Dismiss notification", exact: true }).click();
    });
  }

  /** The follow request of `from` (a display name). */
  followRequest(from: string): Locator {
    return this.notification(`${from} requested to follow you`);
  }

  /** The row over the notifications that counts the follow requests ("1 follow request"), a link to their screen. */
  followRequestsRow(count: number): Locator {
    return this.page
      .getByRole("main")
      .getByRole("link", { name: `${String(count)} follow ${count === 1 ? "request" : "requests"}`, exact: true });
  }

  /** Opens the screen of the follow requests from the row that counts `count` of them. */
  async openFollowRequests(count: number): Promise<FollowRequests> {
    await this.followRequestsRow(count).click();
    return new FollowRequests(this.page);
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

  /**
   * The row over the notifications that counts those the notification policy holds back ("2 held-back notifications ·
   * from 1 person"), a link to their screen.
   */
  heldRow(count: number, people: number): Locator {
    const notes = `${String(count)} held-back ${count === 1 ? "notification" : "notifications"}`;
    const from = `from ${String(people)} ${people === 1 ? "person" : "people"}`;
    return this.page.getByRole("main").getByRole("link", { name: new RegExp(`^${notes} · ${from}.*$`) });
  }

  /** Opens the screen of the held-back notifications from the row that counts them. */
  async openHeld(count: number, people: number): Promise<Held> {
    await this.heldRow(count, people).click();
    return new Held(this.page);
  }

  /** Opens the screen of what waits for the reader's approval from the row that counts `count` of them. */
  async openWaiting(count: number): Promise<WaitingRequests> {
    await this.waitingRow(count).click();
    return new WaitingRequests(this.page);
  }
}

/**
 * The screen of the follow requests, under Notifications: each account that asks, a row with its name, and Decline and
 * Approve until it is answered, its answer after. A fresh person has one request at a time here.
 */
export class FollowRequests {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  get heading(): Locator {
    return this.page.getByRole("main").getByRole("heading", { level: 2, name: "Follow requests", exact: true });
  }

  /** The name of `from` (a display name) on its request's row, a link to their profile. */
  requester(from: string): Locator {
    return this.page.getByRole("main").getByRole("link", { name: from });
  }

  /** The button that answers the one request on the screen; gone once it is answered. */
  button(name: "Approve" | "Decline"): Locator {
    return this.page.getByRole("main").getByRole("button", { name, exact: true });
  }

  /** What the reader answered, in place of the buttons. */
  answer(said: "Approved" | "Declined"): Locator {
    return this.page.getByRole("main").getByText(said, { exact: true });
  }

  /** Approves (`Approve`) or declines (`Decline`) the one request, and returns once the server has answered. */
  async decide(name: "Approve" | "Decline"): Promise<void> {
    const path = /\/follow_requests\/[^/]+\/(?:accept|reject)$/;
    await answered(this.page, { methods: ["POST"], path }, async () => {
      await this.button(name).click();
    });
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

/** The write of an answer to the held-back notifications of an account. */
const heldAnswer = /\/notifications\/requests\/[^/]+\/(?:accept|dismiss)$/;

/**
 * The screen of the notifications the reader's notification policy holds back, under Notifications: a row per account
 * that sent them, its name a link to them, with Dismiss and Accept until it is answered, and its answer after.
 */
export class Held {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** Opens the held-back notifications of `name` (a display name) from its row. */
  async open(name: string): Promise<HeldOf> {
    await this.page.getByRole("main").getByRole("link", { name, exact: true }).click();
    return new HeldOf(this.page);
  }
}

/** The screen of one account's held-back notifications: the account with its answer, over its notifications. */
export class HeldOf {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** The held-back notification named `what`, as the screen says it ("Fresh 1a2b3c4d favourited your post"). */
  notification(what: string): Locator {
    return this.page.getByRole("feed", { name: "Held-back notifications", exact: true }).getByRole("article", {
      name: what,
      exact: true,
    });
  }

  /** Accepts (`Accept`) or dismisses (`Dismiss`) the account's notifications, once the server has answered. */
  async answer(button: "Accept" | "Dismiss"): Promise<void> {
    await answered(this.page, { methods: ["POST"], path: heldAnswer }, async () => {
      await this.page.getByRole("main").getByRole("button", { name: button, exact: true }).click();
    });
  }

  /** What the reader answered, in place of the buttons. */
  said(answer: "Accepted" | "Dismissed"): Locator {
    return this.page.getByRole("main").getByText(answer, { exact: true });
  }
}
