// SPDX-License-Identifier: AGPL-3.0-only

// The sign-in screens of the server: the password, the two-factor code after it, and the passkey, which the browser
// offers on the sign-in screen. A passkey lives in a virtual authenticator of Chromium (CDP WebAuthn).

import { expect, type Locator, type Page } from "@playwright/test";
import type { Person } from "../../src/scenario";
import type { SeedState } from "../../src/seed/seed";

/** A passkey as the seed made it, in the encoding of CDP's WebAuthn.addCredential. */
export type Passkey = SeedState["passkeys"][string];

export class SignIn {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** Opens the sign-in screen; the client sends a signed-out person to the server's. */
  async open(): Promise<void> {
    await this.page.goto("/login");
  }

  get heading(): Locator {
    return this.page.getByRole("main").getByRole("heading", { level: 1, name: "Sign in", exact: true });
  }

  /** The message of a sign-in that failed. */
  get error(): Locator {
    return this.page.getByRole("main").getByRole("alert");
  }

  /** Signs in with an email address and a password. */
  async withPassword(email: string, password: string): Promise<void> {
    await this.page.getByRole("textbox", { name: "Email address", exact: true }).fill(email);
    await this.page.getByLabel("Password", { exact: true }).fill(password);
    await this.page.getByRole("main").getByRole("button", { name: "Sign in", exact: true }).click();
  }

  /** The screen that asks for the code of an authenticator app after the password. */
  get twoFactorHeading(): Locator {
    return this.page
      .getByRole("main")
      .getByRole("heading", { level: 1, name: "Two-factor authentication", exact: true });
  }

  /** Enters the code of an authenticator app. */
  async enterCode(code: string): Promise<void> {
    await this.page.getByRole("textbox", { name: "Authentication code", exact: true }).fill(code);
    await this.page.getByRole("button", { name: "Verify", exact: true }).click();
  }

  /** Gives the browser `passkey` in a virtual authenticator, which answers without a touch. */
  async addPasskey(passkey: Passkey): Promise<void> {
    const cdp = await this.page.context().newCDPSession(this.page);
    await cdp.send("WebAuthn.enable");
    const { authenticatorId } = await cdp.send("WebAuthn.addVirtualAuthenticator", {
      options: {
        protocol: "ctap2",
        transport: "internal",
        hasResidentKey: true,
        hasUserVerification: true,
        isUserVerified: true,
        automaticPresenceSimulation: true,
      },
    });
    await cdp.send("WebAuthn.addCredential", {
      authenticatorId,
      credential: { ...passkey, isResidentCredential: true, signCount: 0 },
    });
  }
}

/** Signs in as `p` with the password on `page`, and waits for their home timeline. */
export async function signInAs(page: Page, p: Person): Promise<void> {
  const screen = new SignIn(page);
  await screen.open();
  await screen.withPassword(p.email, p.password);
  await expect(page.getByRole("feed", { name: "Home timeline", exact: true })).toBeVisible();
}
