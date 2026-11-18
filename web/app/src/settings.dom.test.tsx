// SPDX-License-Identifier: AGPL-3.0-only

// The settings (#3198), which the navigation's Settings leads to: the menu at /settings, its entries in their groups
// and Sign out… apart at its end; an entry's screen at an address of its own, with the menu beside it marking it and
// the way back to the menu; an address that names no entry; and signing out, from the menu's row, from an address, and
// from the page of a server that closed. With the first entry, Display and posting (#3199), the app reads in the
// language chosen there at once. What each control of the entry saves and says is the feature's test (app-features,
// settings/display.dom.test.tsx), what the confirmation does with a sign-out that fails, its own
// (settings/sign-out.dom.test.tsx); what the device keeps of a reader who signed out is with the preferences
// (preferences.dom.test.tsx). Whether the menu shows beside the screen or before it is the width's, a layout test.

import { describe, expect, test } from "bun:test";
import { onSignedOutElsewhere, preferencesQuery } from "@my/app-data";
import { settingsFindings } from "@my/design-system/test/settings";
import { screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, type TestApp } from "./test/app";
import { browserCaches, ownDevice } from "./test/device";
import { fakeApp, fakeTabs, withoutToken } from "./test/server";
import { rail } from "./test/viewport";

/** The title of the screen in place, as its heading says it. */
const heading = () =>
  main()
    .queryAllByRole("heading", { level: 1 })
    .map((title) => title.textContent)
    .join();

/** The rows of a settings menu: each link's words, where it leads, and whether it is the entry the reader is at. */
const rows = (menu: HTMLElement) =>
  within(menu)
    .getAllByRole("link")
    .map((link) => [link.textContent, link.getAttribute("href"), link.getAttribute("aria-current")]);

/** The groups of a settings menu, by their headings. */
const groups = (menu: HTMLElement) =>
  within(menu)
    .getAllByRole("heading")
    .map((group) => group.textContent);

/** The settings' menus outside the screen, named `name` in the reader's language: the one the frame places beside an entry. */
const beside = (name = "Settings") =>
  screen.queryAllByRole("navigation", { name }).filter((menu) => !screen.getByRole("main").contains(menu));

const profileHint = "Name, bio, images, badges, approving followers, appearing in search";
const displayHint = "Language, time zone, link previews, defaults for new posts, timeline languages";
const notificationsHint = "What you are notified of, push, email, sound, notifications from people you do not know";
const relationshipsHint = "Blocked accounts, muted accounts, muted servers, followed hashtags";
const filtersHint = "Words and phrases that hide posts";
const accountHint =
  "Export and import, automatic deletion of old posts, moving the account, moderation actions and appeals, deactivation and deletion";

/**
 * The count of the entry Account and data in `menu`, there once the actions on the reader's account are read: one of
 * the fake's is in effect and can be appealed.
 */
const counted = (menu: HTMLElement, label = "1 item to check") => within(menu).findByText(label);

describe("the menu", () => {
  test("opens from the navigation's Settings, with its entries in their groups and Sign out… last", async () => {
    const app = await renderApp("/lists");

    await app.user.click(within(rail()).getByRole("link", { name: "Settings" }));

    await waitFor(() => {
      if (heading() !== "Settings") throw new Error(`test: the screen is "${heading()}"`);
    });
    expect(app.path()).toBe("/settings");
    expect(app.title()).toContain("Settings");
    const menu = main().getByRole("navigation", { name: "Settings" });
    await counted(menu);
    // Three groups have entries; a group without one is not drawn.
    expect(groups(menu)).toEqual(["General", "People and filters", "Account"]);
    const [profile, display, notifications, relationships, filters, account, ...others] = rows(menu);
    // An entry's row says its name, then what it holds.
    expect(profile?.[0]).toMatch(new RegExp(`^Profile\\s*${profileHint}$`));
    expect(profile?.slice(1)).toEqual(["/settings/profile", null]);
    expect(display?.[0]).toMatch(new RegExp(`^Display and posting\\s*${displayHint}$`));
    expect(display?.slice(1)).toEqual(["/settings/display", null]);
    expect(notifications?.[0]).toMatch(new RegExp(`^Notifications\\s*${notificationsHint}$`));
    expect(notifications?.slice(1)).toEqual(["/settings/notifications", null]);
    expect(relationships?.[0]).toMatch(new RegExp(`^Relationships\\s*${relationshipsHint}$`));
    expect(relationships?.slice(1)).toEqual(["/settings/relationships", null]);
    expect(filters?.[0]).toMatch(new RegExp(`^Keyword filters\\s*${filtersHint}$`));
    expect(filters?.slice(1)).toEqual(["/settings/filters", null]);
    // Then how many things wait there, as its badge and as what the badge means.
    expect(account?.[0]).toMatch(new RegExp(`^Account and data\\s*${accountHint}\\s*1\\s*1 item to check$`));
    expect(account?.slice(1)).toEqual(["/settings/account", null]);
    expect(others).toEqual([["Sign out…", "/settings?sign_out=true", null]]);
    // The menu is the screen: none beside it.
    expect(beside()).toHaveLength(0);
    expect(within(rail()).getByRole("link", { name: "Settings" }).getAttribute("aria-current")).toBe("page");
  });

  test("leads to an entry's screen, whose way back leads to the menu", async () => {
    const app = await renderApp("/settings");

    await app.user.click(main().getByRole("link", { name: /^Display and posting/ }));
    await main().findByRole("combobox", { name: "Display language" });
    expect(app.path()).toBe("/settings/display");
    expect(heading()).toBe("Display and posting");

    await app.user.click(main().getByRole("button", { name: "Settings" }));
    await main().findByRole("navigation", { name: "Settings" });
    expect(app.path()).toBe("/settings");
    expect(heading()).toBe("Settings");
  });

  for (const at of ["/settings", "/settings/display"])
    test(`asks a reader without a session to sign in, at ${at}`, async () => {
      const app = await renderApp(at, { visitor: true });

      expect(heading()).toBe("Settings");
      expect(app.title()).toContain("Settings");
      expect(main().getByRole("status").textContent).toMatch(/^Sign in to see this screen\.\s*Sign in$/);
      expect(main().getByRole("link", { name: "Sign in" }).getAttribute("href")).toBe("/login");
      expect(screen.queryAllByRole("navigation", { name: "Settings" })).toHaveLength(0);
    });
});

describe("an entry", () => {
  test("opens at its address, titled by its name, with the menu beside it marking it and the navigation's Settings", async () => {
    const app = await renderApp("/settings/display");

    await main().findByRole("combobox", { name: "Display language" });
    expect(heading()).toBe("Display and posting");
    expect(app.title()).toContain("Display and posting");
    expect(app.path()).toBe("/settings/display");
    const [menu, ...others] = beside();
    if (menu === undefined) throw new Error("test: no settings menu beside the screen");
    expect(others).toHaveLength(0);
    await counted(menu);
    expect(groups(menu)).toEqual(["General", "People and filters", "Account"]);
    expect(rows(menu)).toEqual([
      ["Profile", "/settings/profile", null],
      ["Display and posting", "/settings/display", "page"],
      ["Notifications", "/settings/notifications", null],
      ["Relationships", "/settings/relationships", null],
      ["Keyword filters", "/settings/filters", null],
      ["Account and data 11 item to check", "/settings/account", null],
      ["Sign out…", "/settings/display?sign_out=true", null],
    ]);
    expect(within(rail()).getByRole("link", { name: "Settings" }).getAttribute("aria-current")).toBe("page");
  });

  test("that the address names but the settings do not have says so under the place's name, and leads to all settings", async () => {
    const app = await renderApp("/settings/nothing");

    // The screen keeps the name of the place the reader is in, as a missing announcement keeps Announcements.
    expect(heading()).toBe("Settings");
    expect(app.title()).toContain("Settings");
    expect(main().getByRole("alert").textContent).toMatch(
      /^This settings page does not exist\. Check the address\.\s*See all settings$/,
    );
    expect(main().queryAllByRole("combobox")).toHaveLength(0);
    // The menu beside it marks no entry.
    for (const menu of beside()) await counted(menu);
    expect(beside().flatMap(rows)).toEqual([
      ["Profile", "/settings/profile", null],
      ["Display and posting", "/settings/display", null],
      ["Notifications", "/settings/notifications", null],
      ["Relationships", "/settings/relationships", null],
      ["Keyword filters", "/settings/filters", null],
      ["Account and data 11 item to check", "/settings/account", null],
      ["Sign out…", "/settings/nothing?sign_out=true", null],
    ]);
    const all = main().getByRole("link", { name: "See all settings" });
    expect(all.getAttribute("href")).toBe("/settings");

    await app.user.click(all);

    await main().findByRole("navigation", { name: "Settings" });
    expect(app.path()).toBe("/settings");
  });
});

// What the rules are and how a tree breaks each is the check's own test (design-system, test/settings.dom.test.tsx).
test("the menu and an entry's screen, as the app draws them, keep the rules of a settings screen (ADR-0110 §6)", async () => {
  const app = await renderApp("/settings");
  // The menu has a group to count the entries of.
  expect(within(main().getByRole("navigation", { name: "Settings" })).getAllByRole("region")).not.toHaveLength(0);
  expect(settingsFindings(document.body)).toEqual([]);

  await app.navigate("/settings/display");
  await main().findByRole("combobox", { name: "Display language" });

  // The screen has sections to count the controls of, and the menu stands beside it.
  expect(main().getAllByRole("region")).not.toHaveLength(0);
  expect(beside()).toHaveLength(1);
  expect(settingsFindings(document.body)).toEqual([]);

  // An address that names no entry: what it says in the screen's place, and the menu beside it.
  await app.navigate("/settings/nothing");
  await main().findByRole("alert");

  expect(beside()).toHaveLength(1);
  expect(settingsFindings(document.body)).toEqual([]);
});

describe("the display language", () => {
  test("chosen in the settings, the app reads in it at once, before the server answers, and the page says so; back to the browser's the same", async () => {
    ownDevice();
    // The server holds the browser's language and the device's zone: the app has nothing of its own to send it, and
    // the one change sent is the reader's.
    const zone = new Intl.DateTimeFormat().resolvedOptions().timeZone;
    const on = fakeApp({ preferences: { locale: "en", time_zone: zone } });
    const app = await renderApp("/settings/display", { on });
    await main().findByRole("combobox", { name: "Display language" });
    const sent = on.fake.script("PATCH /preferences", { hold: "request" });

    await app.user.selectOptions(main().getByRole("combobox", { name: "Display language" }), "日本語");
    expect((await sent.arrived()).body).toEqual({ locale: "ja" });

    // The server has not read the request, and the app is in Japanese: its screen, its menu, its navigation, its title.
    await waitFor(() => {
      if (heading() !== "表示と投稿") throw new Error(`test: the screen is "${heading()}"`);
    });
    expect(document.documentElement.lang).toBe("ja");
    expect(app.title()).toContain("表示と投稿");
    for (const menu of beside("設定")) await counted(menu, "確認が必要な項目 1 件");
    expect(beside("設定").flatMap(rows)).toEqual([
      ["プロフィール", "/settings/profile", null],
      ["表示と投稿", "/settings/display", "page"],
      ["通知", "/settings/notifications", null],
      ["つながり", "/settings/relationships", null],
      ["キーワードフィルタ", "/settings/filters", null],
      ["アカウントとデータ 1確認が必要な項目 1 件", "/settings/account", null],
      ["ログアウト…", "/settings/display?sign_out=true", null],
    ]);
    expect(screen.getAllByRole("navigation", { name: "メイン" })).toHaveLength(2);
    sent.release();
    await sent.served();

    await app.user.selectOptions(main().getByRole("combobox", { name: "表示言語" }), "ブラウザと同じ");

    await waitFor(() => {
      if (heading() !== "Display and posting") throw new Error(`test: the screen is "${heading()}"`);
    });
    expect(document.documentElement.lang).toBe("en");
    expect((await sent.served(2)).body).toEqual({ locale_follows_browser: true });
    // The same app on the same screen: no page was loaded.
    expect(app.path()).toBe("/settings/display");
    expect(app.left()).toEqual([]);
  });
});

// The row that would say so is gone with its screen: the app says it over whatever screen the reader is on.
describe("a change whose save fails after the reader left the screen", () => {
  const notSaved = "“Link previews in posts” could not be saved. Open the settings and change it again.";

  /** The toasts that say a setting was not saved. */
  const toasts = () => screen.queryAllByRole("status").filter((toast) => toast.textContent.startsWith(notSaved));

  /** The reader chose small link previews and went to their lists; then the server failed the save. */
  async function leftBeforeTheAnswer() {
    // The server holds the browser's language and the device's zone: the one change sent is the reader's.
    const zone = new Intl.DateTimeFormat().resolvedOptions().timeZone;
    const on = fakeApp({ preferences: { locale: "en", time_zone: zone } });
    const app = await renderApp("/settings/display", { on });
    await main().findByRole("combobox", { name: "Link previews in posts" });
    const sent = on.fake.script("PATCH /preferences", { hold: "request", fail: 500 });
    await app.user.selectOptions(main().getByRole("combobox", { name: "Link previews in posts" }), "Small");
    await sent.arrived();
    await app.navigate("/lists");
    expect(toasts()).toHaveLength(0);
    sent.release();
    const [toast, ...others] = await waitFor(() => {
      if (toasts().length === 0) throw new Error("test: no toast says that the setting was not saved");
      return toasts();
    });
    if (toast === undefined) throw new Error("test: no toast");
    expect(others).toHaveLength(0);
    return { app, toast };
  }

  test("a toast names the setting that was not saved, and Open settings leads to its screen and takes the toast away", async () => {
    const { app, toast } = await leftBeforeTheAnswer();
    expect(app.path()).toBe("/lists");
    expect(
      within(toast)
        .getAllByRole("button")
        .map((button) => button.getAttribute("aria-label") ?? button.textContent),
    ).toEqual(["Open settings", "Close"]);

    await app.user.click(within(toast).getByRole("button", { name: "Open settings" }));

    await main().findByRole("combobox", { name: "Link previews in posts" });
    expect(app.path()).toBe("/settings/display");
    expect(toasts()).toHaveLength(0);
    // The control is at what the server holds: the change was not kept.
    expect(main().getByRole<HTMLSelectElement>("combobox", { name: "Link previews in posts" }).value).toBe("large");
  });

  test("while the reader is still on the screen, the row says so and no toast does", async () => {
    const app = await renderApp("/settings/display", { preferencesFail: "save" });
    const size = await main().findByRole("combobox", { name: "Link previews in posts" });

    await app.user.selectOptions(size, "Small");

    await main().findByRole("button", { name: "Try again" });
    expect(toasts()).toHaveLength(0);
  });

  test("Close takes the toast away and leaves the reader where they are", async () => {
    const { app, toast } = await leftBeforeTheAnswer();

    await app.user.click(within(toast).getByRole("button", { name: "Close" }));

    await waitFor(() => {
      if (toasts().length > 0) throw new Error("test: the toast is still there");
    });
    expect(app.path()).toBe("/lists");
  });
});

/** The confirmation of signing out, once it is open. */
const confirmation = () => screen.findByRole("alertdialog", { name: "Sign out of this browser?" });

/** Waits until no confirmation is open and the app is at `path`. */
const closedAt = (app: TestApp, path: string) =>
  waitFor(() => {
    if (screen.queryAllByRole("alertdialog").length > 0 || app.path() !== path)
      throw new Error(
        `test: the app is at ${app.path()}, with ${String(screen.queryAllByRole("alertdialog").length)} confirmations`,
      );
  });

/** Waits until the app has the reader's preferences: its cache holds something of the account. */
const read = ({ data }: Pick<TestApp, "data">) =>
  waitFor(() => {
    if (data.queryClient.getQueryData(preferencesQuery(data).queryKey) === undefined)
      throw new Error("test: the preferences are not read yet");
  });

describe("signing out", () => {
  // The row Sign out… of each menu: the screen's own at /settings, and the one beside an entry's screen.
  for (const [menu, at, title] of [
    ["the menu", "/settings", "Settings"],
    ["the menu beside an entry", "/settings/display", "Display and posting"],
  ] as const)
    test(`Sign out… of ${menu} asks first, over the screen, and Cancel goes back to it`, async () => {
      const app = await renderApp("/lists");
      await app.navigate(at);
      const [own] = at === "/settings" ? [screen.getByRole("main")] : beside();
      if (own === undefined) throw new Error("test: no settings menu beside the screen");

      await app.user.click(within(own).getByRole("link", { name: "Sign out…" }));

      const asking = await confirmation();
      expect(app.path()).toBe(`${at}?sign_out=true`);
      expect(heading()).toBe(title);
      expect(app.fake.revocations).toEqual([]);

      await app.user.click(within(asking).getByRole("button", { name: "Cancel" }));

      await closedAt(app, at);
      expect(heading()).toBe(title);
      // Back, and no step added: the screen before the settings is one step behind.
      await app.back();
      expect(app.path()).toBe("/lists");
      expect(app.fake.revocations).toEqual([]);
      expect(app.left()).toEqual([]);
    });

  /** The app opened at Display and posting with the confirmation over it, and Cancel pressed: no confirmation is open. */
  async function cancelledAtAnEntry() {
    const app = await renderApp("/settings/display?sign_out=true");
    const asking = await confirmation();
    expect(heading()).toBe("Display and posting");
    await app.user.click(within(asking).getByRole("button", { name: "Cancel" }));
    await waitFor(() => {
      if (screen.queryAllByRole("alertdialog").length > 0) throw new Error("test: the confirmation is still open");
    });
    return app;
  }

  test("an address with sign_out=true asks over the screen it names, and Cancel takes the asking address out of the history", async () => {
    const app = await cancelledAtAnEntry();

    expect(app.path()).not.toContain("sign_out");
    // Nothing is behind the address the app opened at: going back does not ask again.
    await app.back();
    expect(screen.queryAllByRole("alertdialog")).toHaveLength(0);
    expect(app.fake.revocations).toEqual([]);
    expect(app.left()).toEqual([]);
  });

  // A reader who reloads Display and posting with the confirmation open and cancels stays on that screen: the
  // address that asked gives way to the screen's own, not to the menu's.
  test("Cancel of a confirmation opened by its address leaves the reader on the screen it was over", async () => {
    const app = await cancelledAtAnEntry();

    expect(app.path()).toBe("/settings/display");
  });

  // Signing out removes what the browser kept of the account and leaves by a load of the page: for the start once
  // the server took the token's revocation, and, when the session had no token left to revoke, for the server's own
  // sign-in page, which knows whether the browser still has a session there.
  /** Signs out from the menu's row and its confirmation, and returns once the app left by a load of the page. */
  async function signedOut(app: TestApp) {
    await read(app);
    await app.user.click(main().getByRole("link", { name: "Sign out…" }));
    await app.user.click(within(await confirmation()).getByRole("button", { name: "Sign out" }));
    await waitFor(() => {
      if (app.left().length === 0) throw new Error("test: the app did not leave yet");
    });
  }

  test("Sign out revokes the session's token, empties what the browser kept of the account, tells the reader's other tabs, and leaves for the start", async () => {
    const caches = browserCaches("shell", "home-timeline");
    // A browser with the app in one tab and the same session in another.
    const { on, tab } = await fakeTabs();
    const elsewhere: string[] = [];
    onSignedOutElsewhere(await tab(), () => elsewhere.push("the other tab hears the sign-out"));
    const app = await renderApp("/settings", { on });

    await signedOut(app);

    expect(app.left()).toEqual(["/"]);
    expect(app.fake.revocations).toEqual([{ clientId: "my-web", token: "a1" }]);
    expect(app.data.queryClient.getQueryCache().getAll()).toHaveLength(0);
    expect(caches()).toEqual([]);
    expect(elsewhere).toEqual(["the other tab hears the sign-out"]);
  });

  test("Sign out of a session the server ended already revokes nothing, empties what the browser kept all the same, and leaves for the server's sign-in page", async () => {
    const caches = browserCaches("shell", "home-timeline");
    // The session has no token left to revoke: only the server knows whether the browser is still signed in there.
    const app = await renderApp("/settings", { on: withoutToken(fakeApp()) });

    await signedOut(app);

    expect(app.left()).toEqual(["/auth/sign_in"]);
    expect(app.fake.revocations).toEqual([]);
    expect(app.data.queryClient.getQueryCache().getAll()).toHaveLength(0);
    expect(caches()).toEqual([]);
  });

  test("a revocation the server fails leaves the confirmation open, saying so, removes nothing, and the app stays", async () => {
    const caches = browserCaches("shell", "home-timeline");
    const app = await renderApp("/settings?sign_out=true", { revocation: "fails" });
    await read(app);
    const asking = await confirmation();

    await app.user.click(within(asking).getByRole("button", { name: "Sign out" }));

    const failed = await screen.findByRole("alert");
    expect(failed.textContent).toContain("Signing out did not finish");
    expect((await confirmation()).contains(failed)).toBe(true);
    expect(app.path()).toBe("/settings?sign_out=true");
    expect(app.left()).toEqual([]);
    expect(app.fake.revocations).toEqual([]);
    expect(app.data.queryClient.getQueryData(preferencesQuery(app.data).queryKey)).toBeDefined();
    expect(caches()).toEqual(["shell", "home-timeline"]);
  });

  test("the page of a server that closed signs out through the same confirmation: Cancel closes it, Sign out leaves", async () => {
    const app = await renderApp("/", { closure: "closed" });
    const closed = () => screen.getAllByRole("heading", { level: 1 }).map((title) => title.textContent);

    await app.user.click(await screen.findByRole("button", { name: "Sign out" }));
    await app.user.click(within(await confirmation()).getByRole("button", { name: "Cancel" }));

    await waitFor(() => {
      if (screen.queryAllByRole("alertdialog").length > 0) throw new Error("test: the confirmation is still open");
    });
    expect(closed()).toEqual(["mycroblog demo has closed"]);
    expect(app.fake.revocations).toEqual([]);
    expect(app.left()).toEqual([]);

    await app.user.click(screen.getByRole("button", { name: "Sign out" }));
    await app.user.click(within(await confirmation()).getByRole("button", { name: "Sign out" }));

    await waitFor(() => {
      if (app.left().length === 0) throw new Error("test: the app did not leave yet");
    });
    expect(app.left()).toEqual(["/"]);
    expect(app.fake.revocations).toEqual([{ clientId: "my-web", token: "" }]);
  });
});
