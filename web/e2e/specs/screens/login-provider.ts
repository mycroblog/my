// SPDX-License-Identifier: AGPL-3.0-only

// Signing in with the scenario's login provider: its link on the server's sign-in screen, the page of the fake OpenID
// Connect provider it leads to (`my dev oidc-provider`), which asks for no password and signs in whoever its form
// names, and the server's screens for a person the provider names for the first time: the one that creates their
// account, and the one that says they are signed in. The provider's certificate is signed by the development CA,
// which the browser does not trust: a spec that opens its page sets `ignoreHTTPSErrors`.

import type { Locator, Page } from "@playwright/test";
import type { ProviderIdentity } from "../../src/fresh";
import { loginProvider } from "../../src/scenario";

export class LoginProvider {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** Follows the provider's link on the sign-in screen, among the other ways to sign in. */
  async start(): Promise<void> {
    await this.page
      .getByRole("region", { name: "Or sign in with", exact: true })
      .getByRole("link", { name: loginProvider.name, exact: true })
      .click();
  }

  /** The heading of the provider's page. */
  get heading(): Locator {
    return this.page
      .getByRole("main")
      .getByRole("heading", { level: 1, name: "Sign in to the fake provider", exact: true });
  }

  /**
   * Signs in at the provider as `who`: with all it knows of them, their email address verified, or with their subject
   * alone, which is all that names a person it has signed in before.
   */
  async signInAs(who: ProviderIdentity, o: { subjectOnly?: boolean } = {}): Promise<void> {
    await this.page.getByRole("textbox", { name: "Subject", exact: true }).fill(who.subject);
    if (o.subjectOnly !== true) {
      await this.page.getByRole("textbox", { name: "Email address", exact: true }).fill(who.email);
      await this.page
        .getByRole("checkbox", { name: "The provider has verified the email address", exact: true })
        .click();
      await this.page.getByRole("textbox", { name: "Name", exact: true }).fill(who.name);
      await this.page.getByRole("textbox", { name: "Username", exact: true }).fill(who.username);
    }
    await this.page.getByRole("main").getByRole("button", { name: "Sign in", exact: true }).click();
  }

  /** Refuses the sign-in at the provider. */
  async deny(): Promise<void> {
    await this.page.getByRole("main").getByRole("button", { name: "Deny", exact: true }).click();
  }

  /** The heading of the server's screen that creates the account of a person the provider names for the first time. */
  get createHeading(): Locator {
    return this.page
      .getByRole("main")
      .getByRole("heading", { level: 1, name: `Create an account with ${loginProvider.name}`, exact: true });
  }

  /** That screen's field of the username, filled in from what the provider said. */
  get username(): Locator {
    return this.page.getByRole("textbox", { name: "Username", exact: true });
  }

  /** That screen's field of the email address, filled in from what the provider said. */
  get email(): Locator {
    return this.page.getByRole("textbox", { name: "Email address", exact: true });
  }

  /** That screen's words for an email address the provider has verified. */
  get emailConfirmedByProvider(): Locator {
    return this.page
      .getByRole("main")
      .getByText(
        `${loginProvider.name} confirmed this address. If you change it, we send a link to confirm the new one.`,
        { exact: true },
      );
  }

  /** Creates the account as the screen proposes it. */
  async createAccount(): Promise<void> {
    await this.page.getByRole("main").getByRole("button", { name: "Create an account", exact: true }).click();
  }

  /** The heading of the server's screen that follows the new account. */
  get signedIn(): Locator {
    return this.page.getByRole("main").getByRole("heading", { level: 1, name: "You are signed in", exact: true });
  }

  /** Follows that screen's link into the app. */
  async toApp(): Promise<void> {
    await this.page.getByRole("main").getByRole("link", { name: "Continue", exact: true }).click();
  }
}
