// SPDX-License-Identifier: AGPL-3.0-only

// Profile in the settings: the entry at /settings/profile, where each choice of how the account is found saves as it
// is made, and the form it leads to at /settings/profile/edit, where the images, the name, the bio and the fields are
// saved together with Save.

import { expect, type Locator, type Page } from "@playwright/test";
import type { Upload } from "./article-editor";
import { answered } from "./write";

/** The entry: the reader's profile as others see it, the hashtags it features, and the choices of how it is found. */
export class ProfileSettings {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** The screen's heading, the entry's name. */
  get heading(): Locator {
    return this.page.getByRole("main").getByRole("heading", { level: 1, name: "Profile", exact: true });
  }

  /** Opens the entry at its address, and returns once its settings are read. */
  async goTo(): Promise<void> {
    await this.page.goto("/settings/profile");
    await expect(this.heading).toBeVisible();
    await expect(this.tagField).toBeVisible();
  }

  /** The switch named `name`. */
  toggle(name: string): Locator {
    return this.page.getByRole("main").getByRole("switch", { name, exact: true });
  }

  /**
   * Flips the switch named `name`, a member of the account's credentials, by its label, and returns once the server
   * has answered.
   */
  async flip(name: string): Promise<void> {
    await answered(this.page, { methods: ["PATCH"], path: /\/accounts\/update_credentials$/ }, async () => {
      await this.page.getByRole("main").getByText(name, { exact: true }).click();
    });
  }

  /** The field that adds a featured hashtag. */
  get tagField(): Locator {
    return this.page.getByRole("main").getByRole("textbox", { name: "Featured hashtags", exact: true });
  }

  /** The button on the featured hashtag `name` that takes it off: there while the account features it. */
  featured(name: string): Locator {
    return this.page.getByRole("main").getByRole("button", { name: `Remove #${name}`, exact: true });
  }

  /** Features the hashtag `name` with Enter, and returns once the server has answered. */
  async feature(name: string): Promise<void> {
    await this.tagField.fill(name);
    await answered(this.page, { methods: ["PUT"], path: /\/featured_tags$/ }, async () => {
      await this.tagField.press("Enter");
    });
  }
}

/** The form: Edit profile. */
export class ProfileForm {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** The screen's heading. */
  get heading(): Locator {
    return this.page.getByRole("main").getByRole("heading", { level: 1, name: "Edit profile", exact: true });
  }

  get displayName(): Locator {
    return this.page.getByRole("main").getByRole("textbox", { name: "Display name", exact: true });
  }

  /** The group of the avatar: its picture, and the buttons that choose, replace and remove it. */
  get avatar(): Locator {
    return this.page.getByRole("main").getByRole("group", { name: "Avatar", exact: true });
  }

  /** Picks `file` as the avatar in the browser's file chooser; the form shows it at once and sends nothing yet. */
  async chooseAvatar(file: Upload): Promise<void> {
    const chooser = this.page.waitForEvent("filechooser");
    await this.avatar.getByRole("button", { name: /^(Choose an image|Replace)$/ }).click();
    await (await chooser).setFiles(file);
    await expect(this.avatar.getByRole("button", { name: "Remove", exact: true })).toBeVisible();
  }

  /**
   * Answers the confirmation the app opens when the reader leaves the form with unsaved changes with Keep editing, and
   * returns once it closed.
   */
  async keepEditing(): Promise<void> {
    const asking = this.page.getByRole("alertdialog", { name: "Leave without saving?", exact: true });
    await asking.getByRole("button", { name: "Keep editing", exact: true }).click();
    await expect(asking).toBeHidden();
  }

  /**
   * Saves the form, and returns once the server has answered the change of the profile, which the form sends after
   * the server processed the images picked, and the form says that it saved.
   */
  async save(): Promise<void> {
    await answered(this.page, { methods: ["PATCH"], path: /\/accounts\/update_credentials$/ }, async () => {
      await this.page.getByRole("main").getByRole("button", { name: "Save", exact: true }).click();
    });
    await expect(
      this.page
        .getByRole("main")
        .getByRole("status")
        .filter({ hasText: /^Saved$/ }),
    ).toBeVisible();
  }
}
