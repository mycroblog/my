// SPDX-License-Identifier: AGPL-3.0-only

// The consent screen of the server's OAuth authorization (/oauth/authorize), where a signed-in person allows or denies
// an app. The app's redirect URI is on a reserved `.test` host that does not resolve, so the parameters the redirect
// carries back to the app are read from the browser's request to it.

import { createHash, randomBytes } from "node:crypto";
import { expect, type Locator, type Page } from "@playwright/test";
import type { App } from "../../src/scenario";

export class Consent {
  readonly page: Page;
  readonly app: App;
  /** The app's PKCE verifier, whose challenge the authorization request carries, and the client ID it asked as. */
  readonly #verifier = randomBytes(32).toString("base64url");
  #clientId = "";

  constructor(page: Page, app: App) {
    this.page = page;
    this.app = app;
  }

  /** Asks the server, as the app would, for an authorization code with PKCE (S256) and `state`. */
  async open(clientId: string, state: string): Promise<void> {
    const redirectURI = this.app.redirectUris[0] ?? "";
    this.#clientId = clientId;
    const query = new URLSearchParams({
      response_type: "code",
      client_id: clientId,
      redirect_uri: redirectURI,
      scope: this.app.scope,
      state,
      code_challenge: createHash("sha256").update(this.#verifier).digest("base64url"),
      code_challenge_method: "S256",
    });
    await this.page.clock.install();
    await this.page.goto(`/oauth/authorize?${query.toString()}`);
  }

  /**
   * Trades the authorization `code` for tokens at the server's token endpoint, as the app does once the browser is
   * back at its redirect URI (a public app: PKCE and no secret), and returns the access token. The account holds a
   * grant of the app from then on, not from the consent alone.
   */
  async exchange(code: string): Promise<string> {
    const answer = await this.page.request.post("/oauth/token", {
      form: {
        grant_type: "authorization_code",
        code,
        redirect_uri: this.app.redirectUris[0] ?? "",
        client_id: this.#clientId,
        code_verifier: this.#verifier,
      },
    });
    if (!answer.ok()) throw new Error(`e2e: the token endpoint answered ${String(answer.status())} to the app's code`);
    const tokens = (await answer.json()) as { access_token: string };
    return tokens.access_token;
  }

  get heading(): Locator {
    return this.page
      .getByRole("main")
      .getByRole("heading", { level: 1, name: `Allow ${this.app.name} to use your account`, exact: true });
  }

  /** The list of the permissions the app asks for that the screen asks the person to read. */
  get permissions(): Locator {
    return this.page
      .getByRole("region", { name: "Permissions that need your attention", exact: true })
      .getByRole("listitem");
  }

  /** The paragraph of the screen that holds `text`. */
  paragraph(text: string): Locator {
    return this.page.getByRole("main").getByRole("paragraph").filter({ hasText: text });
  }

  /** Allows the app, once the screen lets the person choose; returns the parameters of the redirect to the app. */
  async allow(): Promise<URLSearchParams> {
    return this.choose("Allow");
  }

  /** Denies the app, once the screen lets the person choose; returns the parameters of the redirect to the app. */
  async deny(): Promise<URLSearchParams> {
    return this.choose("Deny");
  }

  // The screen holds Allow and Deny for a few seconds after it opens, so that nobody allows without reading, and counts
  // them down ("you can choose in 5 s"). The part runs the page's clock, which open() installed, a minute ahead, far
  // past the count, instead of waiting the seconds out; when the count is already over, that changes nothing.
  private async choose(name: "Allow" | "Deny"): Promise<URLSearchParams> {
    const button = this.page.getByRole("button", { name, exact: true });
    await this.page.clock.runFor(60_000);
    await expect(this.paragraph("you can choose in")).toBeHidden();
    await expect(button).toBeEnabled();
    const redirectURI = this.app.redirectUris[0] ?? "";
    const [request] = await Promise.all([
      this.page.waitForRequest((r) => r.url().startsWith(`${redirectURI}?`)),
      button.click(),
    ]);
    return new URL(request.url()).searchParams;
  }
}
