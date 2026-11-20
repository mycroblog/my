// SPDX-License-Identifier: AGPL-3.0-only

// The settings, which the navigation's Settings leads to: the menu at /settings, a navigation whose rows lead to the
// entries' screens and whose last row, Sign out…, asks before it signs out of this browser; Display and posting, the
// entry where a reader chooses the language the app reads in; and the landing page a reader who signed out is left
// on. The app reads in the reader's language, so a part is made for the language its words are in.

import { expect, type Locator, type Page } from "@playwright/test";
import { answered } from "./write";

/** A language the app has words in. */
type Language = "en" | "ja";

/** The words of the settings a spec finds its parts by, in each language. */
const words = {
  en: {
    settings: "Settings",
    display: "Display and posting",
    language: "Display language",
    signOutRow: "Sign out…",
    asks: "Sign out of this browser?",
    signOut: "Sign out",
  },
  ja: {
    settings: "設定",
    display: "表示と投稿",
    language: "表示言語",
    signOutRow: "ログアウト…",
    asks: "このブラウザからログアウトしますか？",
    signOut: "ログアウトする",
  },
} as const satisfies Record<Language, Record<string, string>>;

/** The display languages by their own names, each with the tag the server keeps. */
const tags = { English: "en", 日本語: "ja" } as const;

/** Display and posting: the language, the time zone, how posts show and how a new post starts. */
export class DisplaySettings {
  readonly page: Page;
  readonly in: Language;

  constructor(page: Page, language: Language = "en") {
    this.page = page;
    this.in = language;
  }

  /** The screen's heading, the entry's name. */
  get heading(): Locator {
    return this.page.getByRole("main").getByRole("heading", { level: 1, name: words[this.in].display, exact: true });
  }

  /** The choice of the language the app reads in; its value is the language's tag, or empty for the browser's. */
  get language(): Locator {
    return this.page.getByRole("main").getByRole("combobox", { name: words[this.in].language, exact: true });
  }

  /**
   * Chooses the display language by its own name ("日本語"), and returns once the server has answered; the app reads
   * in it from then on, so the parts after it are made for that language.
   */
  async chooseLanguage(name: keyof typeof tags): Promise<void> {
    // The answer to the reader's own choice: the account has the language and no longer follows the browser, which
    // the answer to the app's own update of an account that follows the browser never says.
    const chosen = (body: unknown) =>
      typeof body === "object" &&
      body !== null &&
      "locale" in body &&
      body.locale === tags[name] &&
      "locale_follows_browser" in body &&
      body.locale_follows_browser === false;
    await answered(this.page, { methods: ["PATCH"], path: /\/preferences$/, answer: chosen }, async () => {
      await this.language.selectOption({ label: name });
    });
  }
}

export class Settings {
  readonly page: Page;
  readonly in: Language;

  constructor(page: Page, language: Language = "en") {
    this.page = page;
    this.in = language;
  }

  /** The heading of the menu's screen. */
  get heading(): Locator {
    return this.page.getByRole("main").getByRole("heading", { level: 1, name: words[this.in].settings, exact: true });
  }

  /** The menu, a navigation of the entries in their groups. */
  get menu(): Locator {
    return this.page.getByRole("main").getByRole("navigation", { name: words[this.in].settings, exact: true });
  }

  /** Opens the menu at its address. */
  async goTo(): Promise<void> {
    await this.page.goto("/settings");
    await expect(this.heading).toBeVisible();
  }

  /** Follows the menu's row to Display and posting. */
  async openDisplay(): Promise<DisplaySettings> {
    await this.menu.getByRole("link", { name: words[this.in].display, exact: true }).click();
    const display = new DisplaySettings(this.page, this.in);
    await expect(display.heading).toBeVisible();
    return display;
  }

  /** The confirmation that the menu's last row opens. */
  get confirmation(): Locator {
    return this.page.getByRole("alertdialog", { name: words[this.in].asks, exact: true });
  }

  /**
   * Signs out with the menu's last row and its confirmation, and returns once the server has answered the revocation
   * of the session's token; the app then leaves for the server's address by a load of the page.
   */
  async signOut(): Promise<void> {
    await this.menu.getByRole("link", { name: words[this.in].signOutRow, exact: true }).click();
    await answered(this.page, { methods: ["POST"], path: /\/oauth\/revoke$/ }, async () => {
      await this.confirmation.getByRole("button", { name: words[this.in].signOut, exact: true }).click();
    });
  }
}

/** The landing page of the server, which a reader without a session meets at its address. */
export class Landing {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** Opens the server's address, as a reader without a session. */
  async open(): Promise<void> {
    await this.page.goto("/");
  }

  /**
   * The way to the sign-in, which only a reader without a session is offered: at the page's head while the server
   * takes new accounts, in the account's part while it takes none.
   */
  get signIn(): Locator {
    return this.page.getByRole("link", { name: "Sign in", exact: true });
  }

  /** The account's part of the page. */
  get account(): Locator {
    return this.page.getByRole("region", { name: "Account", exact: true });
  }

  /** The way to the registration screen, in the account's part, while the server takes new accounts from anyone. */
  get signUp(): Locator {
    return this.account.getByRole("link", { name: "Create an account", exact: true });
  }
}
