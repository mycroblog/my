// SPDX-License-Identifier: AGPL-3.0-only

// The reader's private note on a profile (FR-310): a line that opens its editor while there is none, the note itself
// as the button that edits it once written, and the editor, a box with Save.

import type { Locator, Page } from "@playwright/test";
import { answered } from "./write";

const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

export class AccountNote {
  readonly page: Page;
  readonly card: Locator;

  /** The note on the profile whose card is `card` (Profile.card). */
  constructor(page: Page, card: Locator) {
    this.page = page;
    this.card = card;
  }

  /** The line that opens the editor while the reader has no note. */
  get add(): Locator {
    return this.card.getByRole("button", { name: "Add a note only you can see", exact: true });
  }

  /** The note `text` as the profile shows it: the button that edits it. */
  written(text: string): Locator {
    return this.card.getByRole("button", { name: new RegExp(`^Edit note ${escape(text)}$`) });
  }

  /** The editor's box. */
  get box(): Locator {
    return this.card.getByRole("textbox", { name: "Private note", exact: true });
  }

  /** Saves what the box holds, and returns once the server has answered. */
  async save(): Promise<void> {
    await answered(this.page, { methods: ["PUT", "DELETE"], path: /\/accounts\/[^/]+\/note$/ }, async () => {
      await this.card.getByRole("button", { name: "Save", exact: true }).click();
    });
  }
}
