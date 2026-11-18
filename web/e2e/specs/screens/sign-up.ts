// SPDX-License-Identifier: AGPL-3.0-only

// The registration screen of the server, which a person without an account reaches from the sign-in screen or from an
// invite's link, and the screen that keeps a person out of the app while their account is not ready.

import type { Locator, Page } from "@playwright/test";
import type { Newcomer } from "../../src/fresh";

export class SignUp {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** Opens the registration screen from the sign-in screen, which the client sends a signed-out person to. */
  async open(): Promise<void> {
    await this.page.goto("/login");
    await this.page.getByRole("main").getByRole("link", { name: "Create an account", exact: true }).click();
  }

  /** Opens the registration screen at the address of an invite. */
  async openInvite(url: string): Promise<void> {
    await this.page.goto(url);
  }

  get heading(): Locator {
    return this.page.getByRole("main").getByRole("heading", { level: 1, name: "Create an account", exact: true });
  }

  /** The field of the invite code. */
  get inviteCode(): Locator {
    return this.page.getByRole("textbox", { name: "Invite code", exact: true });
  }

  /** Fills in the form as `who`, with why they want to join where the server asks, and sends it. */
  async as(who: Newcomer, o: { reason?: string } = {}): Promise<void> {
    await this.page.getByRole("textbox", { name: "Username", exact: true }).fill(who.username);
    await this.page.getByRole("textbox", { name: "Email address", exact: true }).fill(who.email);
    await this.page.getByLabel("Password", { exact: true }).fill(who.password);
    if (o.reason !== undefined)
      await this.page.getByRole("textbox", { name: "Why do you want to join? (optional)", exact: true }).fill(o.reason);
    await this.page.getByRole("main").getByRole("button", { name: "Create an account", exact: true }).click();
  }

  /** The screen a signed-in person meets instead of the app while their account is not ready. */
  get notReady(): Locator {
    return this.page
      .getByRole("main")
      .getByRole("heading", { level: 1, name: "Your account is not ready yet", exact: true });
  }

  /** The words of that screen for a registration that waits for a moderator. */
  get waitsForApproval(): Locator {
    return this.page
      .getByRole("main")
      .getByText(
        "Your registration is waiting for a moderator's approval. You can sign in, but you can use the app only after it is approved.",
        { exact: true },
      );
  }
}
