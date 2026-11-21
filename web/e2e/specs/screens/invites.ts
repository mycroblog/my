// SPDX-License-Identifier: AGPL-3.0-only

// Invites, the settings' entry of an account's own invites, at /settings/invites, and the screens under it: the form
// that creates an invite, which shows the invite it made in its place, and an invite's own screen. Only a reader who
// may create invites has the entry in the settings' menu; another one who opens its address reads why they may not.

import { expect, type Locator, type Page } from "@playwright/test";

/** An invite as the server answered its creation: the code a person enters, and the sign-up page's address with it. */
export type CreatedInvite = { code: string; url: string };

export class Invites {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** The menu's row to the entry. */
  get menuRow(): Locator {
    return this.page
      .getByRole("main")
      .getByRole("navigation", { name: "Settings", exact: true })
      .getByRole("link", { name: "Invites", exact: true });
  }

  /** The screen's heading, the entry's name. */
  get heading(): Locator {
    return this.page.getByRole("main").getByRole("heading", { level: 1, name: "Invites", exact: true });
  }

  /** The section of the reader's invites, whose title counts them once they are all read. */
  get list(): Locator {
    return this.page.getByRole("main").getByRole("region", { name: /^Your invites(?: \d+)?$/ });
  }

  /** Opens the entry at its address, and returns once its list is read. */
  async goTo(): Promise<void> {
    await this.page.goto("/settings/invites");
    await expect(this.heading).toBeVisible();
    await expect(this.list).toBeVisible();
  }

  /** The way to the form that creates an invite, which only a reader who may create one is offered. */
  get create(): Locator {
    return this.page.getByRole("main").getByRole("link", { name: "Create invite", exact: true });
  }

  /** The summary that tells a reader who may not create invites so, and why. */
  get cannotCreate(): Locator {
    return this.page.getByRole("main").getByRole("region", { name: "You cannot create invites", exact: true });
  }

  /**
   * Creates an invite with the form as it stands, at the server's defaults (one person, seven days), or with `uses`
   * typed into its number of people, and returns the invite as the server answered it; the screen then shows it in the
   * form's place.
   */
  async createOne(uses?: number): Promise<CreatedInvite> {
    await this.create.click();
    const main = this.page.getByRole("main");
    await expect(main.getByRole("heading", { level: 1, name: "Create invite", exact: true })).toBeVisible();
    if (uses !== undefined) {
      // Typed key by key, as a reader does: the field's spin button speaks each value.
      const field = main.getByRole("textbox", { name: "People who can sign up", exact: true });
      await field.clear();
      await field.pressSequentially(String(uses));
    }
    const answer = this.page.waitForResponse(
      (r) => r.request().method() === "POST" && new URL(r.url()).pathname.endsWith("/api/v1/invites"),
    );
    // A click that fails leaves the wait unawaited: its timeout then rejects nothing that is unhandled.
    answer.catch(() => undefined);
    await main.getByRole("button", { name: "Create invite", exact: true }).click();
    return (await (await answer).json()) as CreatedInvite;
  }

  /** The summary that says the invite is created, in the form's place. */
  get created(): Locator {
    return this.page.getByRole("main").getByRole("region", { name: "The invite is created", exact: true });
  }

  /** The link to send, as the screen writes it for the reader to select or copy. */
  get link(): Locator {
    return this.page.getByRole("main").getByRole("list", { name: "Invite link", exact: true }).getByRole("listitem");
  }

  /** Leaves the created invite for the list. */
  async finish(): Promise<void> {
    await this.page.getByRole("main").getByRole("button", { name: "Finish", exact: true }).click();
    await expect(this.heading).toBeVisible();
  }

  /** The row of the list of the invite whose code is `code`: a link named by the code, then what the invite's state is. */
  row(code: string): Locator {
    return this.list.getByRole("link", { name: new RegExp(`^${code} .+$`) });
  }

  /** What an invite's own screen says of its state: the one row of Status. */
  get status(): Locator {
    return this.page.getByRole("main").getByRole("list", { name: "Status", exact: true }).getByRole("listitem");
  }

  /** The heading of the screen of the invite whose code is `code`. */
  title(code: string): Locator {
    return this.page.getByRole("main").getByRole("heading", { level: 1, name: code, exact: true });
  }
}
