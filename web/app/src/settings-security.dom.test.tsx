// SPDX-License-Identifier: AGPL-3.0-only

// Security in the app (#3205), what its routes decide: the settings' menu leads to the entry, which stands beside
// the menu; the entry leads to the screens under it (the password, a new passkey, two-factor authentication), each
// at an address of its own with its way back to the entry; a change made on one of them shows on the entry once the
// reader is back; linking an account leaves the app; the address a login provider sends the browser back to,
// /settings/linked-accounts, leads to the entry with what happened; and the fragment a provider came back with is
// taken off the address once it is read. What each screen shows of an account and does with a press is the feature's
// test (app-features, settings/security/*.dom.test.tsx), and the dialog that confirms it's the reader its own
// (settings/confirmation.dom.test.tsx).

import { describe, expect, test } from "bun:test";
import { settingsFindings } from "@my/design-system/test/settings";
import { fakeExternalConfirmation, fakeTotpCode } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, words, type TestApp } from "./test/app";
import { fakeApp } from "./test/server";

const security = "/settings/security";
const password = `${security}/password`;
const passkey = `${security}/passkey`;
const twoFactor = `${security}/two-factor`;
const yuki = "demo-password-yuki";

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

/** The menu beside a screen under Security, at `path`: it marks Security. */
const marksSecurity = (path: string) => [
  ["Profile", "/settings/profile", null],
  ["Display and posting", "/settings/display", null],
  ["Notifications", "/settings/notifications", null],
  ["Voice chats you hosted", "/settings/voice", null],
  ["Event reminders", "/settings/events", null],
  ["Blocks, mutes, and hashtags", "/settings/relationships", null],
  ["Keyword filters", "/settings/filters", null],
  ["Security", security, "page"],
  ["Apps", "/settings/apps", null],
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

/** The titles of the screen's sections, without their counts. */
const sections = () =>
  main()
    .queryAllByRole("region")
    .map((region) => document.getElementById(region.getAttribute("aria-labelledby") ?? "")?.textContent ?? "");

/** Confirms the dialog that asks whether it's the reader, with yuki's password. */
async function confirm(app: TestApp) {
  const dialog = await screen.findByRole("dialog", { name: "Confirm it's you" });
  await app.user.type(within(dialog).getByLabelText("Current password"), yuki);
  await app.user.click(within(dialog).getByRole("button", { name: "Confirm" }));
}

describe("the entry", () => {
  test("opens from the settings' menu, titled by its name, with the menu beside it marking it, and its way back leads to the menu", async () => {
    const app = await renderApp("/settings");

    await app.user.click(main().getByRole("link", { name: /^Security/ }));

    await section("Sign-in");
    expect(app.path()).toBe(security);
    expect(heading()).toBe("Security");
    expect(app.title()).toContain("Security");
    expect(menu()).toEqual(marksSecurity(security));

    await app.user.click(main().getByRole("button", { name: "Settings" }));

    await main().findByRole("navigation", { name: "Settings" });
    expect(app.path()).toBe("/settings");
  });

  for (const [what, link, path, title] of [
    ["the row Password", /^Password/, password, "Password"],
    ["Turn on of two-factor authentication", /^Turn on$/, twoFactor, "Two-factor authentication"],
    ["Add a passkey", /^Add a passkey$/, passkey, "Add a passkey"],
  ] as const)
    test(`leads by ${what} to a screen of its own, beside the menu that still marks Security, whose way back leads to the entry`, async () => {
      const app = await renderApp(security);
      await section("Sign-in");

      await app.user.click(main().getByRole("link", { name: link }));

      await at(app, path, title);
      expect(app.title()).toContain(title);
      expect(menu()).toEqual(marksSecurity(path));

      await app.user.click(main().getByRole("button", { name: "Security" }));

      await at(app, security, "Security");
      await section("Sign-in");
    });

  for (const path of [security, password, twoFactor, passkey])
    test(`asks a reader without a session to sign in, at ${path}`, async () => {
      await renderApp(path, { visitor: true });

      expect(main().getByRole("status").textContent).toMatch(/^Sign in to see this screen\.\s*Sign in$/);
      expect(main().getByRole("link", { name: "Sign in" }).getAttribute("href")).toBe("/login");
      expect(sections()).toEqual([]);
      expect(main().queryAllByRole("textbox")).toHaveLength(0);
    });
});

// What the rules are and how a tree breaks each is the check's own test (design-system, test/settings.dom.test.tsx).
describe("as the app draws them, the screens keep the rules of a settings screen (ADR-0110 §6)", () => {
  for (const [what, path, access, shows] of [
    ["the entry", security, {}, "Sign-in"],
    ["the entry with two-factor authentication on", security, { twoFactor: true }, "Sign-in"],
    ["the password's screen", password, {}, "Change password"],
    ["the screen that adds a passkey", passkey, {}, "New passkey"],
    ["the setup of two-factor authentication", twoFactor, {}, "Enter the code"],
    ["two-factor authentication on", twoFactor, { twoFactor: true }, "Turn off two-factor authentication"],
  ] as const)
    test(what, async () => {
      await renderApp(path, { access });
      await section(shows);

      expect(beside()).toHaveLength(1);
      expect(settingsFindings(document.body)).toEqual([]);
    });
});

describe("a change made on a screen under the entry shows on the entry", () => {
  test("a password changed ended the other browsers: back on the entry, this browser alone is listed", async () => {
    const app = await renderApp(security);
    await section("Signed-in browsers", 4);
    await app.user.click(main().getByRole("link", { name: /^Password/ }));
    await at(app, password, "Password");
    await app.user.click(main().getByLabelText("New password"));
    await app.user.paste("a-new-password");

    await app.user.click(main().getByRole("button", { name: "Change password" }));
    await confirm(app);

    await main().findByText("Your password is changed, and your other browsers and apps are signed out.");
    await app.user.click(main().getByRole("button", { name: "Security" }));
    await at(app, security, "Security");
    const browsers = await section("Signed-in browsers", 1);
    expect(within(browsers).getAllByRole("listitem")).toHaveLength(1);
    expect(within(browsers).queryAllByRole("button")).toHaveLength(0);
  });

  test("two-factor authentication turned on: Finish leads to the entry, which has its row and no summary; turned off from there, the summary is back, and no setup was asked for on the way", async () => {
    const app = await renderApp(security);
    await section("Two-factor authentication is off");
    await app.user.click(main().getByRole("link", { name: "Turn on" }));
    await at(app, twoFactor, "Two-factor authentication");
    await app.user.type(await main().findByRole("textbox", { name: "Authentication code" }), fakeTotpCode);

    await app.user.click(main().getByRole("button", { name: "Turn on" }));
    await app.user.click(await main().findByRole("button", { name: "Finish" }));

    await at(app, security, "Security");
    const row = await main().findByRole("link", { name: /^Two-factor authentication\s*On since / });
    expect(row.getAttribute("href")).toBe(twoFactor);
    expect(sections()).not.toContain("Two-factor authentication is off");

    const setups = app.fake.script("POST /two_factor/totp/setup");
    await app.user.click(row);
    await at(app, twoFactor, "Two-factor authentication");
    await app.user.click(await main().findByRole("button", { name: "Turn off" }));
    const dialog = await screen.findByRole("dialog", { name: "Enter a code" });
    await app.user.type(within(dialog).getByRole("textbox", { name: "Authentication code" }), fakeTotpCode);
    await app.user.click(within(dialog).getByRole("button", { name: "Turn off" }));

    await at(app, security, "Security");
    await section("Two-factor authentication is off");
    await waitFor(() => {
      if (app.data.queryClient.isFetching() > 0) throw new Error("test: a read is still on its way");
    });
    expect(setups.seen).toHaveLength(0);
  });
});

test("linking an account leaves the app for the address the server answers, by a load of the page", async () => {
  const app = await renderApp(security);
  const sent = app.fake.script("POST /linked_identities");

  await app.user.click(within(await section("Linked accounts")).getByRole("button", { name: "Link" }));
  await confirm(app);

  await waitFor(() => {
    if (app.left().length === 0) throw new Error("test: the app did not leave yet");
  });
  expect(sent.seen.map((s) => s.body.return_to)).toEqual([security]);
  expect(app.left()).toHaveLength(1);
  expect(app.left()[0]).toContain(`/auth/external/${app.fake.loginProviderId("oidc")}/link?`);
  expect(app.path()).toBe(security);
});

describe("back from a login provider", () => {
  const linked = "Your GitHub account is linked. You can sign in with it now.";

  test("the fragment is taken off the entry's address once it is read, and what happened stays on the screen", async () => {
    const on = fakeApp();
    const app = await renderApp(`${security}#linked=${on.fake.loginProviderId("github")}`, { on });

    expect(words(await section("Linked accounts"))).toContain(linked);
    await waitFor(() => {
      if (app.path() !== security) throw new Error(`test: the app is at ${app.path()}`);
    });
    expect(words(await section("Linked accounts"))).toContain(linked);
  });

  test("an error in the fragment is said as an alert, and the fragment is taken off", async () => {
    const on = fakeApp();
    const app = await renderApp(
      `${security}#external_error=linked_elsewhere&provider=${on.fake.loginProviderId("oidc")}`,
      { on },
    );

    expect(words(await within(await section("Linked accounts")).findByRole("alert"))).toBe(
      "That account of the other service is linked to another account on this server. Unlink it there first, or link a different one.",
    );
    await waitFor(() => {
      if (app.path() !== security) throw new Error(`test: the app is at ${app.path()}`);
    });
  });

  test("the address the server sends the browser back to by default, /settings/linked-accounts, leads to the entry with what happened", async () => {
    const on = fakeApp();
    const app = await renderApp(`/settings/linked-accounts#linked=${on.fake.loginProviderId("github")}`, { on });

    expect(words(await section("Linked accounts"))).toContain(linked);
    await at(app, security, "Security");
    expect(menu()).toEqual(marksSecurity(security));
  });

  test("/settings/linked-accounts gives way to the entry: the step back leads to where the reader came from", async () => {
    const app = await renderApp("/lists");

    await app.navigate("/settings/linked-accounts");

    await at(app, security, "Security");
    await app.back();
    expect(app.path()).toBe("/lists");
  });

  test("on the password's screen, a provider's confirmation is read from the fragment, which is taken off, and Change then asks nothing", async () => {
    const on = fakeApp();
    const app = await renderApp(
      `${password}#external_confirmation=${fakeExternalConfirmation}&operation=password_change&provider=${on.fake.loginProviderId("github")}`,
      { on },
    );
    const sent = on.fake.script("POST /accounts/password");

    await main().findByText(
      "You confirmed it's you with GitHub. Repeat what you started: this time you are not asked to confirm.",
    );
    await waitFor(() => {
      if (app.path() !== password) throw new Error(`test: the app is at ${app.path()}`);
    });
    await app.user.click(main().getByLabelText("New password"));
    await app.user.paste("a-new-password");
    await app.user.click(main().getByRole("button", { name: "Change password" }));

    await main().findByText("Your password is changed, and your other browsers and apps are signed out.");
    expect(sent.seen.map((s) => s.body)).toEqual([
      { new_password: "a-new-password", external_confirmation: fakeExternalConfirmation },
    ]);
    expect(screen.queryAllByRole("dialog")).toHaveLength(0);
  });
});
