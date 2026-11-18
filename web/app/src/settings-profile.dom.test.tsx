// SPDX-License-Identifier: AGPL-3.0-only

// Profile in the settings (#3200), as the app places it: the entry at /settings/profile, which the menu's first row
// leads to; the form at /settings/profile/edit, under the same row of the menu, which the entry and the reader's own
// profile lead to and whose way back leads to the entry; a change saved there, which the entry and the profile show
// without a reload; the navigation the app holds while the form has unsaved changes, until the reader chose to leave
// or to keep editing, and the sign-out it does not hold; a choice of the entry whose save fails after the reader left,
// which a toast says over whatever screen they are on; and the rules of a settings screen, kept by both screens. What
// each control saves and says is the feature's (app-features, settings/profile.dom.test.tsx and
// profile-edit.dom.test.tsx); the menu itself and signing out are settings.dom.test.tsx.

import { describe, expect, test } from "bun:test";
import { settingsFindings } from "@my/design-system/test/settings";
import type { FakeOptions } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, type TestApp } from "./test/app";
import { chooseFiles, imageFile } from "./test/files";
import { fakeApp } from "./test/server";
import { rail } from "./test/viewport";

const entry = "/settings/profile";
const form = "/settings/profile/edit";

/** The title of the screen in place, as its heading says it. */
const heading = () =>
  main()
    .queryAllByRole("heading", { level: 1 })
    .map((title) => title.textContent)
    .join();

/**
 * The rows of a settings menu: each link's words, where it leads, and whether it is the entry the reader is at. A row's
 * count is left out: it comes with a read of its own (settings.dom.test.tsx holds it).
 */
const rows = (menu: HTMLElement) =>
  within(menu)
    .getAllByRole("link")
    .map((link) => [
      link.textContent.replace(/\s*\d+\s*\d+ items? to check$/, ""),
      link.getAttribute("href"),
      link.getAttribute("aria-current"),
    ]);

/** The settings' menus outside the screen: the one the frame places beside an entry. */
const beside = () =>
  screen.queryAllByRole("navigation", { name: "Settings" }).filter((menu) => !screen.getByRole("main").contains(menu));

/** The rows of the menu beside a screen of Profile at `at`: Profile is the entry the reader is at. */
const besideProfile = (at: string) => [
  ["Profile", entry, "page"],
  ["Display and posting", "/settings/display", null],
  ["Notifications", "/settings/notifications", null],
  ["Relationships", "/settings/relationships", null],
  ["Keyword filters", "/settings/filters", null],
  ["Account and data", "/settings/account", null],
  ["Sign out…", `${at}?sign_out=true`, null],
];

/** Waits until the entry's controls are drawn: its settings are read. */
const entryRead = () => main().findByRole("textbox", { name: "Featured hashtags" });
/** Waits until the form is drawn, and returns the field of the display name. */
const formRead = () => main().findByRole<HTMLInputElement>("textbox", { name: "Display name" });
/** The field of the display name. */
const nameField = () => main().getByRole<HTMLInputElement>("textbox", { name: "Display name" });

/** Puts `text` in the display name in place of what it holds. */
async function rename({ user }: TestApp, text: string) {
  await user.clear(nameField());
  await user.click(nameField());
  await user.paste(text);
}

/** Presses Save and returns once the form says that it saved. */
async function save({ user }: TestApp) {
  await user.click(main().getByRole("button", { name: "Save" }));
  await waitFor(() => {
    if (
      !main()
        .queryAllByRole("status")
        .some((status) => status.textContent === "Saved")
    )
      throw new Error("test: the form did not say Saved yet");
  });
}

/** The confirmations open over the screen, by their titles. */
const confirmations = () =>
  screen.queryAllByRole("alertdialog").map((dialog) => within(dialog).getAllByRole("heading")[0]?.textContent);

/** Waits until the app asks whether to leave without saving, and returns the confirmation. */
const asked = () => screen.findByRole("alertdialog", { name: "Leave without saving?" });

/** Waits until no confirmation is open and the app is at `path`. */
const settledAt = (app: TestApp, path: string) =>
  waitFor(() => {
    if (confirmations().length > 0 || app.path() !== path)
      throw new Error(`test: the app is at ${app.path()}, with the confirmations ${JSON.stringify(confirmations())}`);
  });

describe("the entry", () => {
  test("is the first row of the menu's General group, and opens at its address with the menu beside it marking it", async () => {
    const app = await renderApp("/settings");
    const general = within(main().getByRole("navigation", { name: "Settings" })).getByRole("region", {
      name: "General",
    });
    const [first] = within(general).getAllByRole("link");
    if (first === undefined) throw new Error("test: the General group has no row");
    expect(first.getAttribute("href")).toBe(entry);

    await app.user.click(first);

    await entryRead();
    expect(app.path()).toBe(entry);
    expect(heading()).toBe("Profile");
    expect(app.title()).toContain("Profile");
    expect(beside().flatMap(rows)).toEqual(besideProfile(entry));
  });

  test("leads to the form with Edit profile, and the form's way back, named Profile, leads to the entry", async () => {
    const app = await renderApp(entry);
    await entryRead();

    await app.user.click(main().getByRole("link", { name: "Edit profile" }));

    await formRead();
    expect(app.path()).toBe(form);
    expect(heading()).toBe("Edit profile");
    expect(app.title()).toContain("Edit profile");
    // The form is a screen of the same entry: the menu beside it still marks Profile.
    expect(beside().flatMap(rows)).toEqual(besideProfile(form));

    await app.user.click(main().getByRole("button", { name: "Profile" }));

    await entryRead();
    expect(app.path()).toBe(entry);
    expect(heading()).toBe("Profile");
    expect(confirmations()).toEqual([]);
  });

  for (const at of [entry, form])
    test(`asks a reader without a session to sign in, at ${at}`, async () => {
      await renderApp(at, { visitor: true });

      expect(main().getByRole("status").textContent).toMatch(/^Sign in to see this screen\.\s*Sign in$/);
      expect(main().queryAllByRole("textbox")).toHaveLength(0);
      expect(main().queryAllByRole("switch")).toHaveLength(0);
    });
});

describe("the form", () => {
  test("opens at its address with what the account has", async () => {
    const app = await renderApp(form);

    expect((await formRead()).value).toBe("Yuki 雪");
    expect(heading()).toBe("Edit profile");
    expect(app.path()).toBe(form);
    expect(beside().flatMap(rows)).toEqual(besideProfile(form));
  });

  for (const at of ["/profile", "/@yuki"])
    test(`is where Edit profile of the reader's own profile leads, at ${at}`, async () => {
      const app = await renderApp(at);
      const own = within(await screen.findByRole("region", { name: "Yuki 雪" }));
      const edit = await own.findByRole("link", { name: "Edit profile" });
      expect(edit.getAttribute("href")).toBe(form);

      await app.user.click(edit);

      expect((await formRead()).value).toBe("Yuki 雪");
      expect(app.path()).toBe(form);
    });

  test("is not offered on another person's profile", async () => {
    await renderApp("/@sam");
    const other = within(await screen.findByRole("region", { name: "Sam Rivera" }));
    // The card is whole: what a reader may do with another person is there.
    await other.findByRole("button", { name: /^(Follow|Following|Follow back)$/ });

    expect(other.queryAllByRole("link", { name: "Edit profile" })).toHaveLength(0);
  });
});

describe("a change saved in the form", () => {
  /** The addresses of the pictures the reader's profile shows: its header's and its avatar's. */
  const pictures = (name: string) =>
    within(screen.getByRole("region", { name }))
      .getAllByRole<HTMLImageElement>("presentation", { hidden: true })
      .map((picture) => picture.src);

  test("shows on the reader's profile and in the entry at once: the new name, and the new avatar in the old one's place", async () => {
    const app = await renderApp("/profile");
    const edit = await within(await screen.findByRole("region", { name: "Yuki 雪" })).findByRole("link", {
      name: "Edit profile",
    });
    const before = pictures("Yuki 雪");
    expect(before).toHaveLength(2);
    await app.user.click(edit);
    await formRead();

    await rename(app, "Yuki Snow");
    await chooseFiles(
      app,
      within(main().getByRole("group", { name: "Avatar" })).getByRole("button", { name: "Replace" }),
      imageFile("snow.png"),
    );
    await save(app);

    // The entry, by the form's way back: nothing is unsaved, so nothing asks.
    await app.user.click(main().getByRole("button", { name: "Profile" }));
    await entryRead();
    expect(main().getAllByRole("group", { name: "Yuki Snow" })).toHaveLength(1);
    expect(confirmations()).toEqual([]);

    // The profile: the same app, no page was loaded.
    await app.navigate("/profile");
    await main().findByRole("heading", { level: 1, name: /^Yuki Snow \(@yuki@demo\.test\)$/ });
    const after = pictures("Yuki Snow");
    expect(after).toHaveLength(2);
    // The header is the one it was, and the avatar is another picture.
    expect(after.filter((picture) => before.includes(picture))).toHaveLength(1);
    expect(app.left()).toEqual([]);
  });

  test("an avatar and a header removed are gone from the profile at once", async () => {
    const app = await renderApp(form);
    await formRead();
    for (const name of ["Header image", "Avatar"])
      await app.user.click(within(main().getByRole("group", { name })).getByRole("button", { name: "Remove" }));
    await save(app);

    await app.navigate("/profile");

    const card = within(await screen.findByRole("region", { name: "Yuki 雪" }));
    await card.findByRole("link", { name: "Edit profile" });
    expect(card.queryAllByRole("presentation", { hidden: true })).toHaveLength(0);
  });
});

describe("leaving the form with unsaved changes", () => {
  /** The form with a name written and not saved. */
  async function unsaved() {
    const app = await renderApp(form);
    await formRead();
    await rename(app, "Yuki Snow");
    return app;
  }

  // Each way out of the form: what the reader presses, and where it leads once they chose to leave.
  const ways = [
    {
      way: "the form's way back",
      leave: (app: TestApp) => app.user.click(main().getByRole("button", { name: "Profile" })),
      to: entry,
    },
    {
      way: "a row of the menu beside it",
      leave: async (app: TestApp) => {
        const [menu] = beside();
        if (menu === undefined) throw new Error("test: no settings menu beside the screen");
        await app.user.click(within(menu).getByRole("link", { name: "Display and posting" }));
      },
      to: "/settings/display",
    },
    {
      way: "a link of the navigation",
      leave: (app: TestApp) => app.user.click(within(rail()).getByRole("link", { name: "Bookmarks" })),
      to: "/bookmarks",
    },
  ] as const;

  for (const { way, leave, to } of ways) {
    test(`${way} asks first, and Keep editing stays on the form with what was written`, async () => {
      const app = await unsaved();

      await leave(app);

      const asking = await asked();
      expect(app.path()).toBe(form);
      expect(
        within(asking)
          .getAllByRole("button")
          .map((button) => button.getAttribute("aria-label") ?? button.textContent),
      ).toEqual(["Close", "Keep editing", "Leave"]);

      await app.user.click(within(asking).getByRole("button", { name: "Keep editing" }));

      await settledAt(app, form);
      expect(heading()).toBe("Edit profile");
      expect(nameField().value).toBe("Yuki Snow");
    });

    test(`${way} asks first, and Leave goes there without saving`, async () => {
      const app = await unsaved();
      const sent = app.fake.script("PATCH /accounts/update_credentials");

      await leave(app);
      await app.user.click(within(await asked()).getByRole("button", { name: "Leave" }));

      await settledAt(app, to);
      expect(main().queryAllByRole("textbox", { name: "Display name" })).toHaveLength(0);
      expect(sent.seen).toHaveLength(0);
    });
  }

  // The browser's own Back is held too, which a browser decides (web/e2e/specs/profile.spec.ts): on a simulation the
  // history goes back without the app being asked.

  test("what was written and left is gone: the form opened again holds the account's name", async () => {
    const app = await unsaved();
    await app.user.click(main().getByRole("button", { name: "Profile" }));
    await app.user.click(within(await asked()).getByRole("button", { name: "Leave" }));
    await settledAt(app, entry);
    await entryRead();

    await app.user.click(main().getByRole("link", { name: "Edit profile" }));

    expect((await formRead()).value).toBe("Yuki 雪");
  });

  test("a form with nothing changed is left without a question", async () => {
    const app = await renderApp(form);
    await formRead();

    await app.user.click(main().getByRole("button", { name: "Profile" }));

    await entryRead();
    expect(app.path()).toBe(entry);
    expect(confirmations()).toEqual([]);
  });

  test("a form whose changes are saved is left without a question", async () => {
    const app = await unsaved();
    await save(app);

    await app.user.click(main().getByRole("button", { name: "Profile" }));

    await entryRead();
    expect(app.path()).toBe(entry);
    expect(confirmations()).toEqual([]);
    expect(main().getAllByRole("group", { name: "Yuki Snow" })).toHaveLength(1);
  });

  test("Sign out… is not held: its own confirmation opens over the form, and Cancel leaves the form as it was written", async () => {
    const app = await unsaved();
    const [menu] = beside();
    if (menu === undefined) throw new Error("test: no settings menu beside the screen");

    await app.user.click(within(menu).getByRole("link", { name: "Sign out…" }));

    const signingOut = await screen.findByRole("alertdialog", { name: "Sign out of this browser?" });
    expect(app.path()).toBe(`${form}?sign_out=true`);
    expect(confirmations()).toEqual(["Sign out of this browser?"]);

    await app.user.click(within(signingOut).getByRole("button", { name: "Cancel" }));

    await settledAt(app, form);
    expect(nameField().value).toBe("Yuki Snow");
    expect(app.fake.revocations).toEqual([]);
  });
});

// The row that would say so is gone with its screen: the app says it over whatever screen the reader is on.
describe("a choice of the entry whose save fails after the reader left the screen", () => {
  // A control of each kind of save the entry makes: the operation it is sent with, what the reader does, and the
  // name the toast gives the setting.
  const kinds = [
    {
      setting: "Require approval to follow you",
      route: "PATCH /accounts/update_credentials",
      change: (app: TestApp) => app.user.click(main().getByRole("switch", { name: "Require approval to follow you" })),
    },
    {
      setting: "Featured hashtags",
      route: "PUT /featured_tags",
      change: async (app: TestApp) => {
        await app.user.click(main().getByRole("textbox", { name: "Featured hashtags" }));
        await app.user.keyboard("sourdough{Enter}");
      },
    },
    {
      setting: "Show “Supporter”",
      route: "PUT /preferences/badges/{role_id}",
      change: (app: TestApp) => app.user.click(main().getByRole("switch", { name: "Show “Supporter”" })),
    },
  ] as const;

  /** The toasts that say a setting was not saved. */
  const toasts = () =>
    screen
      .queryAllByRole("status")
      .filter((toast) => toast.textContent.includes("could not be saved. Open the settings and change it again."));

  for (const { setting, route, change } of kinds)
    test(`${setting}: a toast names the setting, and Open settings leads back to Profile, where the server's value shows`, async () => {
      const on = fakeApp();
      const app = await renderApp(entry, { on });
      await entryRead();
      const sent = on.fake.script(route, { hold: "request", fail: 500 });
      await change(app);
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
      expect(toast.textContent.startsWith(`“${setting}” could not be saved.`)).toBe(true);
      expect(app.path()).toBe("/lists");

      await app.user.click(within(toast).getByRole("button", { name: "Open settings" }));

      await entryRead();
      expect(app.path()).toBe(entry);
      expect(toasts()).toHaveLength(0);
      // The choice was not kept.
      expect(main().getByRole<HTMLInputElement>("switch", { name: "Require approval to follow you" }).checked).toBe(
        false,
      );
      expect(main().getByRole<HTMLInputElement>("switch", { name: "Show “Supporter”" }).checked).toBe(false);
      expect(main().queryAllByRole("button", { name: "Remove #sourdough" })).toHaveLength(0);
    });

  test("while the reader is still on the screen, the row says so and no toast does", async () => {
    const on = fakeApp();
    const app = await renderApp(entry, { on });
    await entryRead();
    on.fake.script("PATCH /accounts/update_credentials", { fail: 500 });

    await app.user.click(main().getByRole("switch", { name: "Require approval to follow you" }));

    await main().findByRole("button", { name: "Try again" });
    expect(toasts()).toHaveLength(0);
  });
});

// What the rules are and how a tree breaks each is the check's own test (design-system, test/settings.dom.test.tsx).
describe("the screens of Profile, as the app draws them, keep the rules of a settings screen (ADR-0110 §6)", () => {
  const eight = [
    "Administrator",
    "Moderator",
    "Supporter",
    "Translator",
    "Emoji maker",
    "Event host",
    "Founding member",
    "Volunteer of the year 2026",
  ].map((label) => ({ label, color: "neutral", displayed: false }) as const);

  // The screen, the server it is drawn on, and what shows that it is drawn.
  const drawn: readonly [string, string, FakeOptions, () => Promise<unknown>][] = [
    ["the entry", entry, {}, entryRead],
    ["the entry of an account with eight badges, one list of them", entry, { settings: { badges: eight } }, entryRead],
    [
      "the entry on a server that decides for every account",
      entry,
      { crawlers: { search: "deny", ai: "deny" } },
      entryRead,
    ],
    ["the form", form, {}, formRead],
    ["the form of an account with nothing yet", form, { profile: { start: "empty" } }, formRead],
  ];

  for (const [what, at, options, read] of drawn)
    test(what, async () => {
      await renderApp(at, options);
      await read();

      // The screen has sections to count the controls of, and the menu stands beside it.
      expect(main().getAllByRole("region")).not.toHaveLength(0);
      expect(beside()).toHaveLength(1);
      expect(settingsFindings(document.body)).toEqual([]);
    });

  test("the form with four fields, where no more can be added", async () => {
    const app = await renderApp(form);
    await formRead();
    for (const n of [2, 3, 4]) {
      await app.user.click(main().getByRole("button", { name: "Add a field" }));
      await main().findByRole("group", { name: `Field ${String(n)}` });
    }

    expect(settingsFindings(document.body)).toEqual([]);
  });
});
