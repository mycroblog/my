// SPDX-License-Identifier: AGPL-3.0-only

// Apps, the settings' entry of the apps that can use an account, at /settings/apps, and the screens under it: an app
// the account authorized, with Revoke and its confirmation; the form that registers an app, then its client ID and
// its secret, shown once; and an app of the reader's own, with the form that issues a token of it. What the server
// decided (a client ID, a secret, a token) is taken from the answer the screen asked for itself, and the screen is
// held to it.

import { expect, type Locator, type Page } from "@playwright/test";
import { answered } from "./write";

/** `text` as a part of a pattern that matches it and nothing else. */
const literal = (text: string) => text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

/** What the server answers a registration with: the app, and its secret when it is confidential. */
type Registered = { clientId: string; secret: string | undefined };

/** What the server answers a token's issue with. */
type Issued = { accessToken: string; refreshToken: string; scope: string };

/** The answer to the POST at a path that matches `path`, begun before the press that sends it. */
function answerTo(page: Page, path: RegExp) {
  const asked = page.waitForResponse((r) => r.request().method() === "POST" && path.test(new URL(r.url()).pathname));
  // A press that fails leaves the wait unawaited: its timeout then rejects nothing that is unhandled.
  asked.catch(() => undefined);
  return asked;
}

/** A value a screen shows to copy (a client ID, a secret, a token), in the list named `name`. */
const shown = (page: Page, name: string): Locator =>
  page.getByRole("main").getByRole("list", { name, exact: true }).getByRole("listitem");

/** An app the account authorized, at /settings/apps/authorized/<id>. */
class AuthorizedApp {
  readonly page: Page;
  readonly name: string;

  constructor(page: Page, name: string) {
    this.page = page;
    this.name = name;
  }

  /** The screen's heading, the app's name. */
  get heading(): Locator {
    return this.page.getByRole("main").getByRole("heading", { level: 1, name: this.name, exact: true });
  }

  /** What the screen knows of the app, a fact an item: where it comes from, then when it was authorized and used. */
  get about(): Locator {
    return this.page.getByRole("main").getByRole("list", { name: "About this app", exact: true }).getByRole("listitem");
  }

  /** Revokes the app with the button and its confirmation, and returns once the server has answered. */
  async revoke(): Promise<void> {
    await this.page
      .getByRole("main")
      .getByRole("region", { name: "Revoke access", exact: true })
      .getByRole("button", { name: "Revoke", exact: true })
      .click();
    const confirmation = this.page.getByRole("alertdialog", { name: `Revoke ${this.name}?`, exact: true });
    await answered(this.page, { methods: ["DELETE"], path: /\/authorized_apps\/[^/]+$/ }, async () => {
      await confirmation.getByRole("button", { name: "Revoke", exact: true }).click();
    });
  }
}

/** An app the reader registered, at /settings/apps/<id>, and the form that issues a token of it under it. */
class OwnApp {
  readonly page: Page;
  readonly name: string;

  constructor(page: Page, name: string) {
    this.page = page;
    this.name = name;
  }

  /** The screen's heading, the app's name. */
  get heading(): Locator {
    return this.page.getByRole("main").getByRole("heading", { level: 1, name: this.name, exact: true });
  }

  /** The app's client ID, which stays on its screen. */
  get clientId(): Locator {
    return shown(this.page, "Client ID");
  }

  /**
   * Follows Issue token and sends the form with the app's scopes, and returns the pair the server answered, once it
   * has.
   */
  async issueToken(): Promise<Issued> {
    const main = this.page.getByRole("main");
    await main.getByRole("link", { name: /^Issue token .+$/ }).click();
    await expect(main.getByRole("heading", { level: 1, name: "Issue token", exact: true })).toBeVisible();
    const asked = answerTo(this.page, /\/oauth_apps\/[^/]+\/tokens$/);
    await main.getByRole("button", { name: "Issue token", exact: true }).click();
    const issued = (await (await asked).json()) as { access_token: string; refresh_token: string; scope: string };
    return { accessToken: issued.access_token, refreshToken: issued.refresh_token, scope: issued.scope };
  }

  /** The access token, shown once it is issued. */
  get accessToken(): Locator {
    return shown(this.page, "Access token");
  }

  /** The refresh token, shown beside it. */
  get refreshToken(): Locator {
    return shown(this.page, "Refresh token");
  }

  /** Leaves the issued token for the app's screen. */
  async finish(): Promise<void> {
    await this.page.getByRole("main").getByRole("button", { name: "Finish", exact: true }).click();
    await expect(this.heading).toBeVisible();
  }
}

export class Apps {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** The screen's heading, the entry's name. */
  get heading(): Locator {
    return this.page.getByRole("main").getByRole("heading", { level: 1, name: "Apps", exact: true });
  }

  /** The section of the apps the account authorized, whose title counts them. */
  get authorized(): Locator {
    return this.page.getByRole("main").getByRole("region", { name: /^Authorized apps(?: \d+)?$/ });
  }

  /** The section of the apps the reader registered, whose title counts them. */
  get own(): Locator {
    return this.page.getByRole("main").getByRole("region", { name: /^Your apps(?: \d+)?$/ });
  }

  /** Opens the entry at its address, and returns once its lists are read. */
  async goTo(): Promise<void> {
    await this.page.goto("/settings/apps");
    await expect(this.heading).toBeVisible();
    await expect(this.authorized).toBeVisible();
  }

  /**
   * The row of the authorized app named `name` that comes from `origin` ("An app registered by @sam"): its name, where
   * it comes from, then its last use or that none is recorded.
   */
  authorizedApp(name: string, origin: string): Locator {
    return this.authorized.getByRole("link", {
      name: new RegExp(`^${literal(name)} ${literal(origin)} (?:Last used .+|No use recorded)$`),
    });
  }

  /** Follows the row of the authorized app named `name` that comes from `origin` to its screen. */
  async openAuthorized(name: string, origin: string): Promise<AuthorizedApp> {
    await this.authorizedApp(name, origin).click();
    const screen = new AuthorizedApp(this.page, name);
    await expect(screen.heading).toBeVisible();
    return screen;
  }

  /** The row of the reader's own app named `name`: its name, its type, then the day it was registered. */
  ownApp(name: string): Locator {
    return this.own.getByRole("link", {
      name: new RegExp(`^${literal(name)} (?:Confidential|Public) Registered .+$`),
    });
  }

  /** Follows the row of the reader's own app named `name` to its screen. */
  async openOwn(name: string): Promise<OwnApp> {
    await this.ownApp(name).click();
    const screen = new OwnApp(this.page, name);
    await expect(screen.heading).toBeVisible();
    return screen;
  }

  /**
   * Follows Register an app, fills the form for a confidential app (the type it starts with) and sends it, and
   * returns what the server answered, once it has: the screen then shows the client ID and the secret, once.
   */
  async register(app: { name: string; redirectUri: string; scopes: string }): Promise<Registered> {
    await this.own.getByRole("link", { name: "Register an app", exact: true }).click();
    const main = this.page.getByRole("main");
    await main.getByRole("textbox", { name: "Name", exact: true }).fill(app.name);
    await main.getByRole("textbox", { name: "Redirect URIs", exact: true }).fill(app.redirectUri);
    await main.getByRole("textbox", { name: "Scopes", exact: true }).fill(app.scopes);
    const asked = answerTo(this.page, /\/oauth_apps$/);
    await main.getByRole("button", { name: "Register an app", exact: true }).click();
    const made = (await (await asked).json()) as { app: { client_id: string }; client_secret?: string };
    return { clientId: made.app.client_id, secret: made.client_secret };
  }

  /** The client ID of the app just registered. */
  get clientId(): Locator {
    return shown(this.page, "Client ID");
  }

  /** The secret of the confidential app just registered, shown this once. */
  get secret(): Locator {
    return shown(this.page, "Client secret");
  }

  /** Leaves what a registration showed for the entry. */
  async finish(): Promise<void> {
    await this.page.getByRole("main").getByRole("button", { name: "Finish", exact: true }).click();
    await expect(this.heading).toBeVisible();
  }
}
