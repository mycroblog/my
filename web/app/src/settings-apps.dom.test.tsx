// SPDX-License-Identifier: AGPL-3.0-only

// Apps in the app (#3206), what its routes decide: the settings' menu leads to the entry, which stands beside the
// menu; the entry leads to the screens under it (an authorized app, the form that registers one, an app of the
// reader's, and under that the form that issues a token), each at an address of its own, titled by what it shows,
// with its way back; an address that names no app keeps the name of its place; a change made on one of those screens
// shows on the entry once the reader is back (an app revoked, registered or deleted, and a server found to have
// turned registration off); and revoking the server's own web client, which ends the session, makes the app leave as
// after signing out, once, with no token to revoke, and tells the reader's other tabs (that the device then forgets
// the account's language and zone is preferences.dom.test.tsx). What each screen shows and does with a press is the feature's test (app-features,
// settings/apps/*.dom.test.tsx), and that the menu lists the entry the menu's own (settings.dom.test.tsx).

import { describe, expect, test } from "bun:test";
import { onSignedOutElsewhere } from "@my/app-data";
import { settingsFindings } from "@my/design-system/test/settings";
import { screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, words, type TestApp } from "./test/app";
import { browserCaches } from "./test/device";
import { fakeApp, fakeTabs } from "./test/server";

const apps = "/settings/apps";
const register = `${apps}/new`;
const authorized = (id: string) => `${apps}/authorized/${id}`;
const own = (id: string) => `${apps}/${id}`;
const token = (id: string) => `${apps}/${id}/token`;

/** An ID the server gave no app. */
const unknown = "01900000-0000-7000-8000-000000000000";

const registrationOff =
  "The server's administrators have turned off app registration. Apps already registered keep working.";

/** The title of the screen in place, as its heading says it. */
const heading = () =>
  main()
    .queryAllByRole("heading", { level: 1 })
    .map((title) => title.textContent)
    .join();

/** The settings' menus outside the screen: the one the frame places beside an entry. */
const beside = () =>
  screen.queryAllByRole("navigation", { name: "Settings" }).filter((menu) => !screen.getByRole("main").contains(menu));

/**
 * The rows of the menu beside the screen: each link's words, where it leads, and whether the reader is at it. A row's
 * count is left out: it comes with a read of its own (settings.dom.test.tsx holds it).
 */
const menu = () =>
  beside().flatMap((part) =>
    within(part)
      .getAllByRole("link")
      .map((link) => [
        link.textContent.replace(/\s*\d+\s*\d+ items? to check$/, ""),
        link.getAttribute("href"),
        link.getAttribute("aria-current"),
      ]),
  );

/** The menu beside the entry or a screen under it, at `path`: it marks Apps. */
const marksApps = (path: string) => [
  ["Profile", "/settings/profile", null],
  ["Display and posting", "/settings/display", null],
  ["Notifications", "/settings/notifications", null],
  ["Voice chat", "/settings/voice", null],
  ["Events", "/settings/events", null],
  ["Relationships", "/settings/relationships", null],
  ["Keyword filters", "/settings/filters", null],
  ["Security", "/settings/security", null],
  ["Apps", apps, "page"],
  ["Invites", "/settings/invites", null],
  ["Account and data", "/settings/account", null],
  ["Sign out…", `${path}?sign_out=true`, null],
];

/** Waits until the screen titled `title` is in place at `path`. */
const at = (app: TestApp, path: string, title: string) =>
  waitFor(() => {
    if (app.path() !== path || heading() !== title)
      throw new Error(`test: the app is at ${app.path()}, on the screen "${heading()}"`);
  });

/** The section of the screen whose title is `name`, with the count beside it when it has one, once it is drawn. */
const section = (name: string, count?: number) =>
  main().findByRole("region", { name: new RegExp(`^${name}${count === undefined ? "" : `\\s*${String(count)}`}$`) });

/** Where the rows of the list in `list` lead, in their order: each app's own address. */
const listed = (list: HTMLElement) =>
  within(list)
    .queryAllByRole("listitem")
    .map((item) => within(item).getByRole("link").getAttribute("href"));

/** Presses `button` of the confirmation named `name`, once it is open. */
async function confirm(app: TestApp, name: string, button: string) {
  const dialog = await screen.findByRole("alertdialog", { name });
  await app.user.click(within(dialog).getByRole("button", { name: button }));
}

/** Writes `text` in the field labelled `name` of the screen, as one paste. */
async function write(app: TestApp, name: string, text: string) {
  await app.user.click(main().getByRole("textbox", { name }));
  await app.user.paste(text);
}

describe("the entry", () => {
  test("opens from the settings' menu, titled by its name, with the menu beside it marking it, and its way back leads to the menu", async () => {
    const app = await renderApp("/settings");

    await app.user.click(main().getByRole("link", { name: /^Apps/ }));

    await section("Authorized apps", 4);
    expect(app.path()).toBe(apps);
    expect(heading()).toBe("Apps");
    expect(app.title()).toContain("Apps");
    expect(menu()).toEqual(marksApps(apps));

    await app.user.click(main().getByRole("button", { name: "Settings" }));

    await main().findByRole("navigation", { name: "Settings" });
    expect(app.path()).toBe("/settings");
  });

  for (const path of [apps, register, authorized(unknown), own(unknown), token(unknown)])
    test(`asks a reader without a session to sign in, at ${path}`, async () => {
      await renderApp(path, { visitor: true });

      expect(main().getByRole("status").textContent).toMatch(/^Sign in to see this screen\.\s*Sign in$/);
      expect(main().getByRole("link", { name: "Sign in" }).getAttribute("href")).toBe("/login");
      expect(main().queryAllByRole("region")).toHaveLength(0);
      expect(main().queryAllByRole("textbox")).toHaveLength(0);
    });

  // The title of an app's screen is the app's name, which a reader with a session has the server asked for as the
  // screen opens: a reader without one is asked to sign in, and the server is asked for nothing of an account.
  for (const [what, path] of [
    ["an app of the reader's", own(unknown)],
    ["an authorized app", authorized(unknown)],
  ] as const)
    test(`asks the server for no app and no list of apps for a reader without a session, at the address of ${what}`, async () => {
      const on = fakeApp({ visitor: true });
      const asked = (["GET /oauth_apps/{id}", "GET /oauth_apps", "GET /authorized_apps"] as const).map(
        (route) => [route, on.fake.script(route)] as const,
      );

      const app = await renderApp(path, { on });

      expect(main().getByRole("status").textContent).toMatch(/^Sign in to see this screen\.\s*Sign in$/);
      await waitFor(() => {
        if (app.data.queryClient.isFetching() > 0) throw new Error("test: a read is still on its way");
      });
      expect(asked.filter(([, script]) => script.seen.length > 0).map(([route]) => route)).toEqual([]);
    });
});

describe("a screen under the entry", () => {
  for (const [what, link, path, title, shows] of [
    [
      "the row of an authorized app",
      /^Tuskr for Android/,
      (app: TestApp) => authorized(app.fake.appId("authorized", 1)),
      "Tuskr for Android",
      "About this app",
    ],
    ["Register an app", /^Register an app$/, () => register, "Register an app", "Name and website"],
    [
      "the row of an app of the reader's",
      /^Home dashboard/,
      (app: TestApp) => own(app.fake.appId("own", 1)),
      "Home dashboard",
      "Credentials",
    ],
  ] as const)
    test(`opens by ${what} at an address of its own, titled by what it shows, beside the menu that still marks Apps, and its way back leads to the entry`, async () => {
      const app = await renderApp(apps);
      await section("Authorized apps", 4);

      await app.user.click(main().getByRole("link", { name: link }));

      await at(app, path(app), title);
      await section(shows);
      expect(app.title()).toContain(title);
      expect(menu()).toEqual(marksApps(path(app)));

      await app.user.click(main().getByRole("button", { name: "Apps" }));

      await at(app, apps, "Apps");
      await section("Authorized apps", 4);
    });

  test("the token's screen opens by Issue token of an app's screen, at an address under the app's, and its way back, named by the app, leads to the app's screen", async () => {
    const on = fakeApp();
    const id = on.fake.appId("own", 1);
    const app = await renderApp(own(id), { on });
    await section("Credentials");

    await app.user.click(main().getByRole("link", { name: /^Issue token/ }));

    await at(app, token(id), "Issue token");
    await section("Token for your own account");
    expect(app.title()).toContain("Issue token");
    expect(menu()).toEqual(marksApps(token(id)));

    await app.user.click(main().getByRole("button", { name: "Home dashboard" }));

    await at(app, own(id), "Home dashboard");
    await section("Credentials");
  });

  test("a token issued, Finish leads to the app's screen", async () => {
    const on = fakeApp();
    const id = on.fake.appId("own", 0);
    const app = await renderApp(token(id), { on });
    await section("Token for your own account");

    await app.user.click(main().getByRole("button", { name: "Issue token" }));
    await section("The token is issued");
    await app.user.click(main().getByRole("button", { name: "Finish" }));

    await at(app, own(id), "Morning bot");
    await section("Credentials");
  });

  for (const [what, path, title] of [
    ["authorized app", authorized(unknown), "Authorized apps"],
    ["app of the reader's", own(unknown), "Your apps"],
    ["app to issue a token of", token(unknown), "Issue token"],
  ] as const)
    test(`at an address that names no ${what}, keeps the name of its place, says that the app is not among the reader's, and See your apps leads to the entry`, async () => {
      const app = await renderApp(path);

      const missing = await main().findByRole("alert");
      expect(heading()).toBe(title);
      expect(words(missing)).toMatch(
        /^This app is not among your apps\. It may have been revoked or deleted\.\s*See your apps$/,
      );
      expect(menu()).toEqual(marksApps(path));

      await app.user.click(within(missing).getByRole("link", { name: "See your apps" }));

      await at(app, apps, "Apps");
      await section("Authorized apps", 4);
    });
});

// What the rules are and how a tree breaks each is the check's own test (design-system, test/settings.dom.test.tsx).
describe("as the app draws them, opened by their addresses, the screens keep the rules of a settings screen (ADR-0110 §6)", () => {
  /** The IDs of an app the fake's reader authorized and of one they registered. */
  type Ids = { authorized: string; own: string };

  for (const [what, path, access, shows, count] of [
    ["the entry", () => apps, {}, "Your apps", 3],
    ["the entry without an app", () => apps, { authorizedApps: "none", oauthApps: "none" }, "Your apps", undefined],
    ["the entry where registration is off", () => apps, { oauthApps: "disabled" }, "Your apps", undefined],
    ["an authorized app", (ids: Ids) => authorized(ids.authorized), {}, "Revoke access", undefined],
    ["the form that registers an app", () => register, {}, "Type", undefined],
    ["the same form where registration is off", () => register, { oauthApps: "disabled" }, "Your apps", undefined],
    ["an app of the reader's", (ids: Ids) => own(ids.own), {}, "Delete app", undefined],
    ["the form that issues a token", (ids: Ids) => token(ids.own), {}, "Token for your own account", undefined],
  ] as const)
    test(what, async () => {
      const on = fakeApp({ access });
      await renderApp(path({ authorized: on.fake.appId("authorized", 1), own: on.fake.appId("own", 0) }), { on });
      await section(shows, count);

      expect(beside()).toHaveLength(1);
      expect(settingsFindings(document.body)).toEqual([]);
    });
});

describe("a change made on a screen under the entry shows on the entry", () => {
  test("an app revoked is gone from the entry as the reader comes back to it, before the list is read again", async () => {
    const on = fakeApp();
    const id = on.fake.appId("authorized", 1);
    const app = await renderApp(apps, { on });
    expect(listed(await section("Authorized apps", 4))).toContain(authorized(id));
    await app.user.click(main().getByRole("link", { name: /^Tuskr for Android/ }));
    await at(app, authorized(id), "Tuskr for Android");
    await section("Revoke access");
    // Whatever the app asks of the list from here on gets no answer before the test has looked.
    const reread = on.fake.script("GET /authorized_apps", { hold: "request" });

    await app.user.click(main().getByRole("button", { name: "Revoke" }));
    await confirm(app, "Revoke Tuskr for Android?", "Revoke");

    await at(app, apps, "Apps");
    const list = await section("Authorized apps", 3);
    expect(listed(list)).not.toContain(authorized(id));
    expect(listed(list)).toHaveLength(3);
    reread.release();
    await waitFor(() => {
      if (app.data.queryClient.isFetching() > 0) throw new Error("test: a read is still on its way");
    });
    expect(listed(await section("Authorized apps", 3))).not.toContain(authorized(id));
  });

  test("an app registered: Finish leads to the entry, which lists it and counts one more", async () => {
    const app = await renderApp(register);
    await section("Name and website");
    await write(app, "Name", "Reading list");
    await write(app, "Redirect URIs", "https://reading.example/callback");
    await write(app, "Scopes", "read");

    await app.user.click(main().getByRole("button", { name: "Register an app" }));
    await section("“Reading list” is registered");
    await app.user.click(main().getByRole("button", { name: "Finish" }));

    await at(app, apps, "Apps");
    const mine = await section("Your apps", 4);
    expect(within(mine).getAllByRole("link", { name: /^Reading list\s*Confidential\s*Registered / })).toHaveLength(1);
  });

  test("an app deleted: the reader is back on the entry, which no longer lists it", async () => {
    const on = fakeApp();
    const id = on.fake.appId("own", 1);
    const app = await renderApp(own(id), { on });
    await section("Delete app");

    await app.user.click(main().getByRole("button", { name: "Delete" }));
    await confirm(app, "Delete Home dashboard?", "Delete");

    await at(app, apps, "Apps");
    const mine = await section("Your apps", 2);
    expect(listed(mine)).not.toContain(own(id));
    expect(main().queryAllByRole("alert")).toHaveLength(0);
  });

  // The list said the reader could register; the administrators turned registration off since, and the refusal of
  // the registration is how the app learns it.
  test("a registration refused because registration was turned off: the form says so, and back on the entry the reader's apps say it too, with no way to register", async () => {
    const app = await renderApp(apps);
    const mine = await section("Your apps", 3);
    await app.user.click(within(mine).getByRole("link", { name: "Register an app" }));
    await at(app, register, "Register an app");
    app.fake.script("POST /oauth_apps", {
      fail: {
        type: "/api/v1/problems/oauth-app-registration-disabled",
        title: "App registration is turned off",
        status: 403,
      },
    });
    await write(app, "Name", "Reading list");
    await write(app, "Redirect URIs", "https://reading.example/callback");
    await write(app, "Scopes", "read");

    await app.user.click(main().getByRole("button", { name: "Register an app" }));

    expect(words(await section("Your apps"))).toContain(registrationOff);
    expect(main().queryAllByRole("textbox")).toHaveLength(0);

    await app.user.click(main().getByRole("button", { name: "Apps" }));

    await at(app, apps, "Apps");
    const after = await section("Your apps", 3);
    expect(words(after)).toContain(registrationOff);
    expect(main().queryAllByRole("link", { name: "Register an app" })).toHaveLength(0);
    // The apps registered before keep their rows.
    expect(listed(after)).toHaveLength(3);
  });
});

// The web client's tokens are the session of every browser of the account, this one too (ADR-0090 §5): once the
// server took the revocation, the app has no session to go on with.
test("revoking the server's own web client ends the session: the app removes the caches the browser kept of the account and leaves for the start by a load of the page, as after signing out", async () => {
  const caches = browserCaches("shell", "home-timeline");
  const on = fakeApp();
  const id = on.fake.appId("authorized", 0);
  const app = await renderApp(authorized(id), { on });
  await section("Revoke access");
  const sent = on.fake.script("DELETE /authorized_apps/{id}");

  await app.user.click(main().getByRole("button", { name: "Revoke" }));
  expect(app.left()).toEqual([]);
  await confirm(app, "Revoke mycroblog?", "Revoke");

  await waitFor(() => {
    if (app.left().length === 0) throw new Error("test: the app did not leave yet");
  });
  expect(sent.seen.map((s) => s.url.pathname.split("/").at(-1))).toEqual([id]);
  expect(app.left()).toEqual(["/"]);
  // What a service worker kept of the account for the next visit goes with the session, and the app stays where it
  // was until the page loads.
  expect(caches()).toEqual([]);
  expect(app.path()).toBe(authorized(id));
});

// The server ended the session when it took the revocation: nothing is left to revoke or to read. The app tells the
// reader's other tabs as a sign-out does and asks the browser for the start, once; the screen is still drawn until
// the page loads.
describe("once the server's own web client is revoked", () => {
  /** What the app reads of the account on a settings screen, which it could read again. */
  const reads = [
    "GET /authorized_apps",
    "GET /oauth_apps",
    "GET /accounts/verify_credentials",
    "GET /preferences",
    "GET /invites",
  ] as const;

  /**
   * The app in one tab of a browser, the same session in another, and the web client revoked from the first: the
   * app has left, the page has not loaded yet, and several turns later no read is on its way.
   */
  async function revoked() {
    const { on, tab } = await fakeTabs();
    const elsewhere: string[] = [];
    onSignedOutElsewhere(await tab(), () => elsewhere.push("the other tab hears the sign-out"));
    const app = await renderApp(authorized(on.fake.appId("authorized", 0)), { on });
    await section("Revoke access");
    const counted = reads.map((route) => [route, on.fake.script(route)] as const);
    await app.user.click(main().getByRole("button", { name: "Revoke" }));
    await confirm(app, "Revoke mycroblog?", "Revoke");
    await waitFor(() => {
      if (app.left().length === 0) throw new Error("test: the app did not leave yet");
    });
    await new Promise((turns) => setTimeout(turns, 50));
    await waitFor(() => {
      if (app.data.queryClient.isFetching() > 0) throw new Error("test: a read is still on its way");
    });
    return {
      app,
      elsewhere,
      /** What the app read of the account since the revocation. */
      readAgain: () => counted.filter(([, script]) => script.seen.length > 0).map(([route]) => route),
    };
  }

  test("no token is revoked, the reader's other tabs hear a sign-out, the app leaves once, and the confirmation stays over the screen until the page loads", async () => {
    const { app, elsewhere } = await revoked();

    expect(app.fake.revocations).toEqual([]);
    expect(elsewhere).toEqual(["the other tab hears the sign-out"]);
    expect(app.left()).toEqual(["/"]);
    expect(screen.getAllByRole("alertdialog", { name: "Revoke mycroblog?" })).toHaveLength(1);
  });

  // The screen and the shell around it are still drawn after the app emptied what it read: neither may read the
  // account again (the lists of apps, the account), which a server that revoked the session's tokens would refuse.
  test("the app reads nothing of the account again, and what it read stays forgotten", async () => {
    const { app, readAgain } = await revoked();

    expect({ read: readAgain(), cached: app.data.queryClient.getQueryCache().getAll().length }).toEqual({
      read: [],
      cached: 0,
    });
  });
});

test("revoking another app leaves the session as it is: the app does not leave", async () => {
  const on = fakeApp();
  const app = await renderApp(authorized(on.fake.appId("authorized", 2)), { on });
  await section("Revoke access");

  await app.user.click(main().getByRole("button", { name: "Revoke" }));
  await confirm(app, "Revoke Feedlet?", "Revoke");

  await at(app, apps, "Apps");
  await section("Authorized apps", 3);
  expect(app.left()).toEqual([]);
  expect(app.fake.revocations).toEqual([]);
});
