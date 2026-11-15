// SPDX-License-Identifier: AGPL-3.0-only

// The composer, a dialog that writes a new post or a reply, keeps it as a draft on the server, or schedules it.

import { expect, type Locator } from "@playwright/test";
import { Drafts } from "./drafts";
import { EmojiPicker } from "./emoji-picker";
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

  /** The button at the end of the text that opens the emoji picker, which writes the emoji picked at the caret. */
  get emojiButton(): Locator {
    return this.dialog.getByRole("button", { name: "Add emoji", exact: true });
  }

  /** Opens the emoji picker from the Add emoji button, and returns it. */
  async emoji(): Promise<EmojiPicker> {
    await this.emojiButton.click();
    return new EmojiPicker(this.dialog.page().getByRole("dialog", { name: "Add emoji", exact: true }));
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

  /** Closes the composer with its Close button. */
  async close(): Promise<void> {
    await this.dialog.getByRole("button", { name: "Close", exact: true }).click();
    await expect(this.dialog).toBeHidden();
  }

  /** Chooses `item` of the drafts' menu in the composer's head. */
  async draftsMenu(item: "Save as draft" | "Drafts and scheduled posts"): Promise<void> {
    await this.dialog.getByRole("button", { name: "Drafts", exact: true }).click();
    const menu = this.dialog.page().getByRole("menu", { name: "Drafts", exact: true });
    await menu.getByRole("menuitem", { name: item, exact: true }).click();
  }

  /** Keeps the post as a draft on the server, and returns once the server has answered and the composer has closed. */
  async saveAsDraft(): Promise<void> {
    await answered(this.dialog.page(), { methods: ["POST", "PUT"], path: /\/drafts(?:\/[^/]+)?$/ }, async () => {
      await this.draftsMenu("Save as draft");
    });
    await expect(this.dialog).toBeHidden();
  }

  /**
   * Saves a draft opened from the server as a draft when another device saved a newer one since: returns once the
   * server has refused it, with the composer open and its alert shown.
   */
  async saveAsDraftRefused(): Promise<void> {
    await answered(this.dialog.page(), { methods: ["PUT"], path: /\/drafts\/[^/]+$/ }, async () => {
      await this.draftsMenu("Save as draft");
    });
    await expect(this.alert).toBeVisible();
  }

  /** Waits until the text of the post is `text`, all of it. */
  async holds(text: string): Promise<void> {
    await expect(this.text).toHaveValue(text);
  }

  /** Opens the reader's drafts and scheduled posts in the composer's place. */
  async openDrafts(): Promise<Drafts> {
    await this.draftsMenu("Drafts and scheduled posts");
    return new Drafts(this.dialog.page());
  }

  /** The alert over the row: why the post was not sent, or why a draft was not saved. */
  get alert(): Locator {
    return this.dialog.getByRole("alert");
  }

  /** Opens the newer draft that another device saved, from the alert of a refused save. */
  async openNewer(): Promise<void> {
    await this.alert.getByRole("button", { name: "Open the newer one", exact: true }).click();
  }

  /** The schedule tool of the row: "Schedule" while no time is set, "Change the scheduled time" while one is. */
  scheduleTool(name: "Schedule" | "Change the scheduled time"): Locator {
    return this.dialog.getByRole("button", { name, exact: true });
  }

  /** The form of the time the post goes out, once open. */
  get scheduleForm(): Locator {
    return this.dialog.getByRole("dialog", { name: "Schedule", exact: true });
  }

  /** The field of the date and time in the schedule's form. */
  get scheduleField(): Locator {
    return this.scheduleForm.getByLabel("Date and time", { exact: true });
  }

  /**
   * Opens the schedule's form from the schedule tool and sets the time it starts with: the first whole hour an hour or
   * more from now.
   */
  async setTime(): Promise<void> {
    await this.scheduleTool("Schedule").click();
    await this.scheduleForm.getByRole("button", { name: "Set time", exact: true }).click();
    await expect(this.scheduleForm).toBeHidden();
  }

  /**
   * Moves the time in the open schedule's form a minute later, from the keyboard, and sets it: the field's segments read
   * month, day, year, hour, minute in en.
   */
  async nextMinute(): Promise<void> {
    await this.scheduleField.focus();
    for (let i = 0; i < 4; i += 1) await this.scheduleField.press("ArrowRight");
    await this.scheduleField.press("ArrowUp");
    await this.scheduleForm.getByRole("button", { name: "Set time", exact: true }).click();
    await expect(this.scheduleForm).toBeHidden();
  }

  /**
   * The line over the row while a time is set: "Scheduled for <time> (<zone>)", the zone stated while the form is
   * closed, then Change time at its end.
   */
  get timeLine(): Locator {
    return this.dialog.getByRole("paragraph").filter({ hasText: /^Scheduled for .+\s*Change time$/ });
  }

  /** Opens the schedule's form from the time line's Change time. */
  async changeTime(): Promise<void> {
    await this.timeLine.getByRole("button", { name: "Change time", exact: true }).click();
    await expect(this.scheduleForm).toBeVisible();
  }

  /**
   * Schedules the post with the row's Schedule, which takes Post's place while a time is set, and returns once the
   * server has answered and the composer has closed.
   */
  async schedule(): Promise<void> {
    await answered(this.dialog.page(), { methods: ["PUT"], path: /\/drafts\/[^/]+\/schedule$/ }, async () => {
      await this.dialog.getByRole("button", { name: "Schedule", exact: true }).click();
    });
    await expect(this.dialog).toBeHidden();
  }

  /** Posts a draft opened from the server, and returns once the server has published it and the composer has closed. */
  async postDraft(): Promise<void> {
    await answered(this.dialog.page(), { methods: ["POST"], path: /\/drafts\/[^/]+\/publish$/ }, async () => {
      await this.dialog.getByRole("button", { name: "Post", exact: true }).click();
    });
    await expect(this.dialog).toBeHidden();
  }
}
