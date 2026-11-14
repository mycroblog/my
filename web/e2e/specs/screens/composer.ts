// SPDX-License-Identifier: AGPL-3.0-only

// The composer, a dialog that writes a new post or a reply.

import { expect, type Locator } from "@playwright/test";
import { answered } from "./write";

export class Composer {
  readonly dialog: Locator;

  constructor(dialog: Locator) {
    this.dialog = dialog;
  }

  /** The text of the post; a reply starts with the mention of the person it replies to. */
  get text(): Locator {
    return this.dialog.getByRole("textbox", { name: "Text", exact: true });
  }

  /** Writes `text` after what the composer already holds. */
  async write(text: string): Promise<void> {
    await this.text.click();
    await this.text.press("ControlOrMeta+End");
    await this.text.pressSequentially(text);
  }

  /** Sends the post, and returns once the server has answered and the composer has closed. */
  async post(): Promise<void> {
    await answered(this.dialog.page(), { methods: ["POST"], path: /\/statuses$/ }, async () => {
      await this.dialog.getByRole("button", { name: "Post", exact: true }).click();
    });
    await expect(this.dialog).toBeHidden();
  }
}
