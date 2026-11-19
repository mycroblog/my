// SPDX-License-Identifier: AGPL-3.0-only

// The report panel (FR-701, FR-709), the same from a post's menu, a profile's, a notification's, a voice chat room's
// and an event's: why, the server's rules for a break of them, the account's posts to include, a comment, sending it
// to a remote account's server too, and Send report, which returns once the server answered; then what it offers
// once sent (mute, block, Done).

import { expect, type Locator, type Page } from "@playwright/test";
import type { Profile } from "./profile";
import type { Post } from "./timeline";
import { answered } from "./write";

/** The filing of a report. */
const filing = { methods: ["POST"], path: /\/reports$/ } as const;

const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

/** Why a report is made, by its English words. */
export type ReportReason = "Spam" | "Illegal content" | "Breaks the server's rules" | "Something else";

export class ReportPanel {
  readonly page: Page;
  /** The panel, named by its title (Report @name, Report this voice chat room, Report this event, Report sent). */
  readonly dialog: Locator;

  constructor(page: Page, title: string | RegExp = /^Report /) {
    this.page = page;
    this.dialog = page.getByRole("dialog", { name: title });
  }

  /** Opens the panel from Report… of a post's menu: the post's author, the post chosen. */
  static async fromPost(post: Post): Promise<ReportPanel> {
    await post.menuItem("Report…");
    const panel = new ReportPanel(post.page);
    await expect(panel.dialog).toBeVisible();
    return panel;
  }

  /** Opens the panel from Report… of the More of a profile's card. */
  static async fromProfile(profile: Profile): Promise<ReportPanel> {
    await (await profile.moreItem("Report…")).click();
    const panel = new ReportPanel(profile.page);
    await expect(panel.dialog).toBeVisible();
    return panel;
  }

  /** The choice of why, a radio each. */
  reason(reason: ReportReason): Locator {
    return this.dialog
      .getByRole("radiogroup", { name: "Why are you reporting this?", exact: true })
      .getByRole("radio", { name: new RegExp(`^${escape(reason)} .+$`) });
  }

  /** Chooses why, as a press on it. */
  async choose(reason: ReportReason): Promise<void> {
    await this.reason(reason).click();
    await expect(this.reason(reason)).toBeChecked();
  }

  /** A rule of the server, once a break of the rules is chosen, by its text. */
  rule(text: string): Locator {
    return this.dialog
      .getByRole("group", { name: "Which rules does this break?", exact: true })
      .getByRole("checkbox", { name: new RegExp(`^${escape(text)}(?: .+)?$`) });
  }

  /** The account's posts to include, a checkbox each. */
  get posts(): Locator {
    return this.dialog.getByRole("group", { name: "Posts to include", exact: true }).getByRole("checkbox");
  }

  /** The post to include whose words are `text`, named by when it was posted and its words. */
  post(text: string): Locator {
    return this.dialog
      .getByRole("group", { name: "Posts to include", exact: true })
      .getByRole("checkbox", { name: new RegExp(`^.+ ${escape(text)}$`) });
  }

  /** Includes the post whose words are `text`, left out so far, as a press on it. */
  async include(text: string): Promise<void> {
    const post = this.post(text);
    await expect(post).not.toBeChecked();
    await post.click();
    await expect(post).toBeChecked();
  }

  get comment(): Locator {
    return this.dialog.getByRole("textbox", { name: "Comment for the moderators", exact: true });
  }

  /** The choice of sending a remote account's report to its server too. */
  get forward(): Locator {
    return this.dialog.getByRole("checkbox", { name: /^Also send the report to the moderators of \S+$/ });
  }

  get send(): Locator {
    return this.dialog.getByRole("button", { name: "Send report", exact: true });
  }

  /** Sends the report, and returns once the server has answered. */
  async submit(): Promise<void> {
    await answered(this.page, filing, async () => {
      await this.send.click();
    });
  }

  /** The panel once sent. */
  get sent(): Locator {
    return this.page.getByRole("dialog", { name: "Report sent", exact: true });
  }

  /** Mutes or blocks the account from the panel once sent, and returns once the server has answered. */
  async after(action: "Mute" | "Block"): Promise<void> {
    await answered(this.page, { methods: ["POST"], path: /\/accounts\/[^/]+\/(?:mute|block)$/ }, async () => {
      await this.sent.getByRole("button", { name: new RegExp(`^${action} @\\S+$`) }).click();
    });
  }

  /** Closes the panel once sent. */
  async done(): Promise<void> {
    await this.sent.getByRole("button", { name: "Done", exact: true }).click();
    await expect(this.sent).toBeHidden();
  }
}
