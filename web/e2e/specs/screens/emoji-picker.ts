// SPDX-License-Identifier: AGPL-3.0-only

// The emoji picker, in the popover a post's React or the composer's Add emoji button opens: a search field, the skin
// tone, a toolbar of categories, and one grid of the emoji, a rowgroup per section. An emoji is a gridcell named by
// its CLDR name or `:shortcode:`, and the reader's own reaction by "{name}, your reaction". The section an emoji is
// found in matters: Frequently used changes with what the reader picked, and the grid draws only the rows in view.

import type { Locator } from "@playwright/test";

export class EmojiPicker {
  readonly dialog: Locator;

  constructor(dialog: Locator) {
    this.dialog = dialog;
  }

  /** The search field, which has the focus when the picker opens. */
  get search(): Locator {
    return this.dialog.getByRole("searchbox", { name: "Search emoji", exact: true });
  }

  /** The emoji named `name` in the section `section` ("Custom emoji", "Smileys & emotion", "Search results"). */
  emoji(section: string, name: string): Locator {
    return this.dialog
      .getByRole("grid", { name: "Emoji", exact: true })
      .getByRole("rowgroup", { name: section, exact: true })
      .getByRole("gridcell", { name, exact: true });
  }
}
