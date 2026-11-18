// SPDX-License-Identifier: AGPL-3.0-only

// Security, the settings' entry of how an account signs in, at /settings/security, and the screens under it: the
// setup of two-factor authentication (the key for the authenticator app, its code, then the recovery codes), the
// screen that adds a passkey, and the dialog that asks a reader to confirm it's them before either. A passkey lives
// in a virtual authenticator of Chromium (CDP WebAuthn), which creates it and signs in with it without a touch.

import { expect, type Locator, type Page } from "@playwright/test";
import { answered } from "./write";

/** The dialog that asks a reader to confirm it's them, with their password. */
class Confirmation {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  get dialog(): Locator {
    return this.page.getByRole("dialog", { name: "Confirm it's you", exact: true });
  }

  /** Enters the password and confirms. */
  async withPassword(password: string): Promise<void> {
    await this.dialog.getByLabel("Current password", { exact: true }).fill(password);
    await this.dialog.getByRole("button", { name: "Confirm", exact: true }).click();
  }
}

/** The setup of two-factor authentication, then the recovery codes it ends with. */
export class TwoFactorSetup {
  readonly page: Page;
  /** The key the server made for the authenticator app, as it answered it. */
  readonly secret: string;

  constructor(page: Page, secret: string) {
    this.page = page;
    this.secret = secret;
  }

  /** The key as the screen writes it, for a reader who cannot scan the QR code. */
  get key(): Locator {
    return this.page.getByRole("main").getByRole("list", { name: "Setup key", exact: true }).getByRole("listitem");
  }

  /** The QR code of the same key. */
  get qrCode(): Locator {
    return this.page.getByRole("main").getByRole("img", { name: "QR code of the setup key", exact: true });
  }

  /** Enters the code of the authenticator app and turns it on, and returns once the server has answered. */
  async turnOn(code: string): Promise<void> {
    await this.page.getByRole("main").getByRole("textbox", { name: "Authentication code", exact: true }).fill(code);
    await answered(this.page, { methods: ["POST"], path: /\/two_factor\/totp$/ }, async () => {
      await this.page.getByRole("main").getByRole("button", { name: "Turn on", exact: true }).click();
    });
  }

  /** The recovery codes, shown once two-factor authentication is on. */
  get recoveryCodes(): Locator {
    return this.page.getByRole("main").getByRole("list", { name: "Recovery codes", exact: true }).getByRole("listitem");
  }

  /** Leaves the recovery codes for Security. */
  async finish(): Promise<void> {
    await this.page.getByRole("main").getByRole("button", { name: "Finish", exact: true }).click();
  }
}

export class Security {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** The screen's heading, the entry's name. */
  get heading(): Locator {
    return this.page.getByRole("main").getByRole("heading", { level: 1, name: "Security", exact: true });
  }

  /** The section of the password and of two-factor authentication, drawn once the entry is read. */
  get signIn(): Locator {
    return this.page.getByRole("main").getByRole("region", { name: "Sign-in", exact: true });
  }

  /** The section of the passkeys, whose title counts them. */
  get passkeys(): Locator {
    return this.page.getByRole("main").getByRole("region", { name: /^Passkeys(?: \d+)?$/ });
  }

  /** The section of the accounts of login providers. */
  get linkedAccounts(): Locator {
    return this.page.getByRole("main").getByRole("region", { name: "Linked accounts", exact: true });
  }

  /** The section of the browsers signed in, whose title counts them. */
  get browsers(): Locator {
    return this.page.getByRole("main").getByRole("region", { name: /^Signed-in browsers \d+$/ });
  }

  /** Opens the entry at its address, and returns once it is read. */
  async goTo(): Promise<void> {
    await this.page.goto("/settings/security");
    await expect(this.heading).toBeVisible();
    await expect(this.signIn).toBeVisible();
  }

  /**
   * Follows Turn on of the summary that says two-factor authentication is off, and returns its setup, with the key
   * the server answered as the screen opened.
   */
  async setUpTwoFactor(): Promise<TwoFactorSetup> {
    const asked = this.page.waitForResponse(
      (r) => r.request().method() === "POST" && new URL(r.url()).pathname.endsWith("/two_factor/totp/setup"),
    );
    // A click that fails leaves the wait unawaited: its timeout then rejects nothing that is unhandled.
    asked.catch(() => undefined);
    await this.page
      .getByRole("main")
      .getByRole("region", { name: "Two-factor authentication is off", exact: true })
      .getByRole("link", { name: "Turn on", exact: true })
      .click();
    const setup = (await (await asked).json()) as { secret: string };
    return new TwoFactorSetup(this.page, setup.secret);
  }

  /** The row of Sign-in that says two-factor authentication is on, and since when. */
  get twoFactorOn(): Locator {
    return this.signIn.getByRole("link", { name: /^Two-factor authentication On since .+$/ });
  }

  /**
   * Adds a passkey named `name`, confirming with the password, and returns once the server keeps it: the browser's
   * authenticator creates it between the two.
   */
  async addPasskey(name: string, password: string): Promise<void> {
    await this.passkeys.getByRole("link", { name: "Add a passkey", exact: true }).click();
    const main = this.page.getByRole("main");
    await main.getByRole("textbox", { name: "Name", exact: true }).fill(name);
    await main.getByRole("button", { name: "Add a passkey", exact: true }).click();
    await answered(this.page, { methods: ["POST"], path: /\/passkeys$/ }, async () => {
      await new Confirmation(this.page).withPassword(password);
    });
  }

  /** What a row of a list offers: the group of its buttons, which the row's name names. */
  private offered(section: Locator, name: string): Locator {
    return section.getByRole("group", { name, exact: true });
  }

  /** The button that removes the passkey named `name`, which its row offers. */
  removePasskey(name: string): Locator {
    return this.offered(this.passkeys, name).getByRole("button", { name: "Remove", exact: true });
  }

  /** The button that signs out the browser the list names `name` ("Safari on macOS"). */
  signOutOf(name: string): Locator {
    return this.offered(this.browsers, name).getByRole("button", { name: "Sign out", exact: true });
  }

  /** Signs out the browser named `name` with its row's button and the confirmation, once the server has answered. */
  async signOutBrowser(name: string): Promise<void> {
    await this.signOutOf(name).click();
    const confirmation = this.page.getByRole("alertdialog", { name: `Sign out ${name}?`, exact: true });
    await answered(this.page, { methods: ["DELETE"], path: /\/sessions\/[^/]+$/ }, async () => {
      await confirmation.getByRole("button", { name: "Sign out", exact: true }).click();
    });
  }

  /**
   * Links an account at the login provider named `provider`, confirming with the password: the browser then leaves
   * for the provider's page.
   */
  async link(provider: string, password: string): Promise<void> {
    await this.offered(this.linkedAccounts, provider).getByRole("button", { name: "Link", exact: true }).click();
    await new Confirmation(this.page).withPassword(password);
  }

  /** What Linked accounts says once the browser is back from `provider` with the account linked. */
  linked(provider: string): Locator {
    return this.linkedAccounts.getByText(`Your ${provider} account is linked. You can sign in with it now.`, {
      exact: true,
    });
  }

  /** The button that unlinks the account at `provider`, which only a linked account's row offers. */
  unlink(provider: string): Locator {
    return this.offered(this.linkedAccounts, provider).getByRole("button", { name: "Unlink", exact: true });
  }
}

/** Gives the browser of `page` a virtual authenticator without a passkey, which makes and uses one without a touch. */
export async function virtualAuthenticator(page: Page): Promise<void> {
  const cdp = await page.context().newCDPSession(page);
  await cdp.send("WebAuthn.enable");
  await cdp.send("WebAuthn.addVirtualAuthenticator", {
    options: {
      protocol: "ctap2",
      transport: "internal",
      hasResidentKey: true,
      hasUserVerification: true,
      isUserVerified: true,
      automaticPresenceSimulation: true,
    },
  });
}
