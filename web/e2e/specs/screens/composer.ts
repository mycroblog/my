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

  /** Writes `text` in place of what the composer holds. */
  async replace(text: string): Promise<void> {
    await this.text.fill(text);
  }

  /** Saves the edit of the reader's post, and returns once the server has answered and the composer has closed. */
  async save(): Promise<void> {
    await answered(this.dialog.page(), { methods: ["PUT"], path: /\/statuses\/[^/]+$/ }, async () => {
      await this.dialog.getByRole("button", { name: "Save", exact: true }).click();
    });
    await expect(this.dialog).toBeHidden();
  }

  /** Sends the post, and returns once the server has answered and the composer has closed. */
  async post(): Promise<void> {
    await answered(this.dialog.page(), { methods: ["POST"], path: /\/statuses$/ }, async () => {
      await this.dialog.getByRole("button", { name: "Post", exact: true }).click();
    });
    await expect(this.dialog).toBeHidden();
  }
}
