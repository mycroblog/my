// SPDX-License-Identifier: AGPL-3.0-only

// Invites in the app (#3207, ADR-0106 §18), what its routes and the settings' menu decide: the menu lists Invites
// only for a reader who may create invites (the default menu, with the entry, is settings.dom.test.tsx's), and draws
// Sign out… alone before it knows, for no longer than it waits; the entry stands beside the menu, which marks it on
// every screen under it; the list leads to an invite's screen and to the form that creates one, each at an address
// of its own with its way back; an invite created or revoked shows in the list once the reader is back; an account
// the server refuses as one that may not create invites reads so on the form's screen and on the list; and a
// one-person server has no such screens (ADR-0152). What each screen shows of an account and does with a press is
// the feature's test (app-features, settings/invites/*.dom.test.tsx).

import { describe, expect, test } from "bun:test";
import { settingsFactsWaitMs } from "@my/app-features";
import { settingsFindings } from "@my/design-system/test/settings";
import { fakeOrigin, type FakeApi, type FakeOptions } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { heldWaitsOf, main, renderApp, words, type TestApp } from "./test/app";
import { fakeApp } from "./test/server";
import { rail } from "./test/viewport";

const invites = "/settings/invites";
const creating = `${invites}/new`;
const display = "/settings/display";
const cannot = "You cannot create invites";

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
 * The rows of the settings' menu, wherever it stands (the screen's own at /settings, or beside an entry): where each
 * leads, and the one the reader is at.
 */
const menu = () =>
  screen.queryAllByRole("navigation", { name: "Settings" }).flatMap((part) =>
    within(part)
      .queryAllByRole("link")
      .map(
        (link) => `${link.getAttribute("href") ?? ""}${link.getAttribute("aria-current") === "page" ? " (here)" : ""}`,
      ),
  );

/** The menu's rows for a reader at `at`, with Invites or without it, marking the entry `here`. */
const rowsAt = (at: string, { withInvites, here }: { withInvites: boolean; here?: string | undefined }) =>
  [
    "/settings/profile",
    display,
    "/settings/notifications",
    "/settings/voice",
    "/settings/events",
    "/settings/relationships",
    "/settings/filters",
    "/settings/security",
    "/settings/apps",
    ...(withInvites ? [invites] : []),
    "/settings/account",
    `${at}?sign_out=true`,
  ].map((row) => (row === here ? `${row} (here)` : row));

/** The menu's one row for a reader at `at` while it does not know its entries: Sign out…, which is always its last. */
const signOutAlone = (at: string) => [`${at}?sign_out=true`];

/** Waits until the screen titled `title` is in place at `path`. */
const at = (app: TestApp, path: string, title: string) =>
  waitFor(() => {
    if (app.path() !== path || heading() !== title)
      throw new Error(`test: the app is at ${app.path()}, on the screen "${heading()}"`);
  });

/** Waits until the app's data has no read on its way but `held` of them. */
const readsLeft = ({ data }: Pick<TestApp, "data">, held = 0) =>
  waitFor(() => {
    if (data.queryClient.isFetching() !== held)
      throw new Error(`test: ${String(data.queryClient.isFetching())} reads are on their way`);
  });

/** The list of the reader's invites, once it is drawn, whatever count stands beside its title. */
const list = () => main().findByRole("region", { name: /^Your invites(\s*\d+)?$/ });

/** An invite as the server answers it, in what these tests read of it. */
type Invite = { id: string; code: string; state: string };

/** The reader's invites as the fake holds them, newest first: all of them, in one page of the server's most. */
async function held(fake: FakeApi): Promise<Invite[]> {
  const answer = await fake.serve(new Request(`${fakeOrigin}/api/v1/invites?limit=40`));
  return ((await answer.json()) as { items: Invite[] }).items;
}

/** The reader's invite at `index` of the fake's list. */
async function invite(fake: FakeApi, index: number): Promise<Invite> {
  const found = (await held(fake)).find((one) => one.id === fake.inviteId(index));
  if (found === undefined) throw new Error(`test: the fake holds no invite at ${String(index)}`);
  return found;
}

/** The row of the list that says `says` after the code of `one`. */
const rowOf = (one: Invite, says: string) =>
  main().findByRole("link", { name: new RegExp(`^${one.code}\\s*${says.replaceAll(" ", "\\s+")}`) });

const notAllowed = { access: { invites: "not-allowed" } } as const satisfies FakeOptions;
const closed = { registrations: "closed" } as const satisfies FakeOptions;
const onePerson = { singleUser: true, viewer: "hana" } as const satisfies FakeOptions;

describe("the settings' menu", () => {
  for (const [who, options] of [
    ["a reader who may not create invites", notAllowed],
    ["a reader of a server that takes no new account", closed],
    ["the reader of a one-person server", onePerson],
  ] as const)
    test(`lists no Invites for ${who}, at /settings and beside an entry's screen`, async () => {
      const app = await renderApp("/settings", options);
      await main().findByRole("navigation", { name: "Settings" });
      await readsLeft(app);
      expect(menu()).toEqual(rowsAt("/settings", { withInvites: false }));

      await app.navigate(display);

      await main().findByRole("combobox", { name: "Display language" });
      await readsLeft(app);
      expect(menu()).toEqual(rowsAt(display, { withInvites: false, here: display }));
    });

  // Whether the reader may create invites is the first page of their invites' to say: on the way from another
  // screen the settings open once the server answered it, so that no row comes in under the reader's hand.
  test("draws no row while it is not known whether the reader may create invites, then all of them at once, on the way from another screen", async () => {
    const on = fakeApp();
    const app = await renderApp("/lists", { on });
    const asked = on.fake.script("GET /invites", { hold: "request" });

    await app.user.click(within(rail()).getByRole("link", { name: "Settings" }));

    await asked.arrived();
    await readsLeft(app, 1);
    expect(menu()).toEqual([]);

    const first = waitFor(() => {
      if (menu().length === 0) throw new Error("test: the menu has no row yet");
      return menu();
    });
    asked.release();

    expect(await first).toEqual(rowsAt("/settings", { withInvites: true }));
    expect(heading()).toBe("Settings");
  });

  /** The addresses a menu opens at with the app, the entry it marks there, and what is drawn around it meanwhile. */
  const places = [
    // The shell is drawn around the menu's screen, and the entry's screen beside the menu: only the menu waits.
    ["the menu's own screen", "/settings", undefined, () => screen.findAllByRole("navigation", { name: "Main" })],
    [
      "an entry's screen, beside which it stands",
      display,
      display,
      () => main().findByRole("combobox", { name: "Display language" }),
    ],
  ] as const;

  // Sign out… is always the menu's last row: drawn from the start, it is under the reader's hand where it stays.
  for (const [where, address, here, drawn] of places)
    test(`draws Sign out… alone while it is not known, then every entry at once, when the app opens at ${where}`, async () => {
      const on = fakeApp();
      const asked = on.fake.script("GET /invites", { hold: "request" });
      const opened = renderApp(address, { on });

      await asked.arrived();
      await drawn();
      await readsLeft(on, 1);
      expect(menu()).toEqual(signOutAlone(address));

      const next = waitFor(() => {
        if (menu().length === 1) throw new Error("test: the menu still lists Sign out… alone");
        return menu();
      });
      asked.release();

      expect(await next).toEqual(rowsAt(address, { withInvites: true, here }));
      await opened;
    });

  // A server that is slow to say whether the reader may create invites keeps them from no other entry for longer
  // than the menu waits; the test ends that wait itself.
  for (const [where, address, here, drawn] of places)
    test(`lists its other entries once it waited ${String(settingsFactsWaitMs)} ms for the answer, and adds Invites when the answer then says the reader may create them, when the app opens at ${where}`, async () => {
      const waits = heldWaitsOf(settingsFactsWaitMs);
      const on = fakeApp();
      const asked = on.fake.script("GET /invites", { hold: "request" });
      const opened = renderApp(address, { on });
      await asked.arrived();
      await drawn();
      await readsLeft(on, 1);
      expect(menu()).toEqual(signOutAlone(address));

      // The menu's wait, and the only one of that length.
      expect(waits.pass()).toBe(1);

      const without = await waitFor(() => {
        if (menu().length === 1) throw new Error("test: the menu still lists Sign out… alone");
        return menu();
      });
      expect(without).toEqual(rowsAt(address, { withInvites: false, here }));
      expect(on.data.queryClient.isFetching()).toBe(1);

      const withInvites = waitFor(() => {
        if (!menu().includes(invites)) throw new Error("test: the menu does not list Invites yet");
        return menu();
      });
      asked.release();

      expect(await withInvites).toEqual(rowsAt(address, { withInvites: true, here }));
      await opened;
    });

  test("each menu waits for itself: beside an entry opened from a menu whose wait is over, the menu lists Sign out… alone until its own is, on the one read", async () => {
    const waits = heldWaitsOf(settingsFactsWaitMs);
    const on = fakeApp();
    const asked = on.fake.script("GET /invites", { hold: "request" });
    const opened = renderApp("/settings", { on });
    await asked.arrived();
    await screen.findAllByRole("navigation", { name: "Main" });
    await readsLeft(on, 1);
    expect(waits.pass()).toBe(1);

    await userEvent.setup().click(await main().findByRole("link", { name: /^Display and posting/ }));

    await main().findByRole("combobox", { name: "Display language" });
    expect(menu()).toEqual(signOutAlone(display));
    expect(waits.pass()).toBe(1);
    const without = await waitFor(() => {
      if (menu().length === 1) throw new Error("test: the menu still lists Sign out… alone");
      return menu();
    });
    expect(without).toEqual(rowsAt(display, { withInvites: false, here: display }));
    expect(asked.seen).toHaveLength(1);

    const withInvites = waitFor(() => {
      if (!menu().includes(invites)) throw new Error("test: the menu does not list Invites yet");
    });
    asked.release();
    await withInvites;
    await opened;
  });

  test("lists its other entries, and no Invites, when the invites could not be read", async () => {
    const on = fakeApp();
    on.fake.script("GET /invites", { fail: 500 });
    const app = await renderApp("/settings", { on });

    await main().findByRole("navigation", { name: "Settings" });
    await readsLeft(app);
    expect(menu()).toEqual(rowsAt("/settings", { withInvites: false }));
  });
});

describe("the entry", () => {
  test("opens from the settings' menu, titled by its name, with the menu beside it marking it, and its way back leads to the menu", async () => {
    const app = await renderApp("/settings");

    await app.user.click(main().getByRole("link", { name: /^Invites/ }));

    await list();
    expect(app.path()).toBe(invites);
    expect(heading()).toBe("Invites");
    expect(app.title()).toContain("Invites");
    expect(menu()).toEqual(rowsAt(invites, { withInvites: true, here: invites }));

    await app.user.click(main().getByRole("button", { name: "Settings" }));

    await main().findByRole("navigation", { name: "Settings" });
    expect(app.path()).toBe("/settings");
  });

  test("leads by an invite's row to the invite's screen, titled by its code, beside the menu that still marks Invites, whose way back leads to the list", async () => {
    const on = fakeApp();
    const app = await renderApp(invites, { on });
    const usable = await invite(on.fake, 0);

    await app.user.click(await rowOf(usable, "Usable until"));

    await at(app, `${invites}/${usable.id}`, usable.code);
    await main().findByRole("region", { name: "Invite link" });
    expect(app.title()).toContain(usable.code);
    expect(menu()).toEqual(rowsAt(`${invites}/${usable.id}`, { withInvites: true, here: invites }));

    await app.user.click(main().getByRole("button", { name: "Invites" }));

    await at(app, invites, "Invites");
    await list();
  });

  // The screen's title and the screen both look for the invite in the list: one of them reads the list on.
  test("opens an invite of a later page of the list by its address, titled by its code, with each page of the list asked for once", async () => {
    const on = fakeApp({ access: { invites: "many" } });
    const old = await invite(on.fake, 25);
    const asked = on.fake.script("GET /invites");

    const app = await renderApp(`${invites}/${old.id}`, { on });

    await at(app, `${invites}/${old.id}`, old.code);
    await main().findByRole("region", { name: "Status" });
    await readsLeft(app);
    expect(app.title()).toContain(old.code);
    expect(
      asked.seen.map((seen) => (seen.url.searchParams.has("cursor") ? "the page after it" : "the first page")),
    ).toEqual(["the first page", "the page after it"]);
  });

  test("leads by Create invite to the form's screen, beside the menu that still marks Invites, whose way back leads to the list", async () => {
    const app = await renderApp(invites);

    await app.user.click(await main().findByRole("link", { name: "Create invite" }));

    await at(app, creating, "Create invite");
    await main().findByRole("textbox", { name: "People who can sign up" });
    expect(app.title()).toContain("Create invite");
    expect(menu()).toEqual(rowsAt(creating, { withInvites: true, here: invites }));

    await app.user.click(main().getByRole("button", { name: "Invites" }));

    await at(app, invites, "Invites");
    await list();
  });

  test("an address that names none of the reader's invites says so under the list's name, and leads to the list", async () => {
    const on = fakeApp();
    const app = await renderApp(`${invites}/${on.fake.inviteId(99)}`, { on });

    const missing = await main().findByRole("alert");
    expect(words(missing)).toMatch(/^This invite is not among your invites\. Check the address\.\s*See your invites$/);
    expect(heading()).toBe("Your invites");
    expect(app.title()).toContain("Your invites");
    expect(menu()).toEqual(rowsAt(app.path(), { withInvites: true, here: invites }));

    await app.user.click(within(missing).getByRole("link", { name: "See your invites" }));

    await at(app, invites, "Invites");
    await list();
  });

  for (const address of [invites, creating, `${invites}/01900000-0000-7000-8000-000000000000`])
    test(`asks a reader without a session to sign in, at ${address}`, async () => {
      await renderApp(address, { visitor: true });

      expect(main().getByRole("status").textContent).toMatch(/^Sign in to see this screen\.\s*Sign in$/);
      expect(main().getByRole("link", { name: "Sign in" }).getAttribute("href")).toBe("/login");
      expect(main().queryAllByRole("region")).toHaveLength(0);
      expect(main().queryAllByRole("textbox")).toHaveLength(0);
    });
});

describe("what the reader did on a screen under the entry shows in the list", () => {
  test("an invite created: Finish leads to the list, where the new invite is first and counted", async () => {
    const on = fakeApp();
    const app = await renderApp(creating, { on });
    await main().findByRole("textbox", { name: "People who can sign up" });

    await app.user.click(main().getByRole("button", { name: "Create invite" }));
    await app.user.click(await main().findByRole("button", { name: "Finish" }));

    await at(app, invites, "Invites");
    const [made, ...before] = await held(on.fake);
    if (made === undefined) throw new Error("test: the fake holds no invite");
    expect(before.map((one) => one.id)).toEqual([0, 1, 2, 3, 4, 5].map((index) => on.fake.inviteId(index)));
    await main().findByRole("region", { name: /^Your invites\s*7$/ });
    const rows = within(main().getByRole("list", { name: "Your invites" })).getAllByRole("link");
    expect(rows.map((row) => row.getAttribute("href"))).toEqual([made, ...before].map((one) => `${invites}/${one.id}`));
    expect(await rowOf(made, "Usable until")).not.toBeNull();
  });

  test("an invite revoked: the confirmation gives way to the list, where its row reads Revoked", async () => {
    const on = fakeApp();
    const usable = await invite(on.fake, 0);
    const app = await renderApp(`${invites}/${usable.id}`, { on });

    await app.user.click(await main().findByRole("button", { name: "Revoke" }));
    const asking = await screen.findByRole("alertdialog", { name: `Revoke the invite ${usable.code}?` });
    await app.user.click(within(asking).getByRole("button", { name: "Revoke" }));

    await at(app, invites, "Invites");
    expect(await rowOf(usable, "Revoked Used 0 of 1 time$")).not.toBeNull();
    expect(screen.queryAllByRole("alertdialog")).toHaveLength(0);
  });

  test("an account the server refuses as one that may not create invites reads why in the form's place, and on the list once back, where no way to create is left", async () => {
    // What a server answers an account that a moderation action keeps from creating invites.
    const refusal = await fakeApp({ access: { invites: "not-allowed", invitesRefusal: "restricted" } }).fake.serve(
      new Request(`${fakeOrigin}/api/v1/invites`, { method: "POST" }),
    );
    const on = fakeApp();
    const app = await renderApp(creating, { on });
    await main().findByRole("textbox", { name: "People who can sign up" });
    // From the refusal on, the server answers the list as for such an account.
    on.fake.script("POST /invites", {
      fail: (await refusal.json()) as { type: string; title: string; status: number },
    });
    on.fake.script("GET /invites", {
      change: (body) => ({ ...body, can_create: false, cannot_create_reason: "restricted" as const }),
    });
    // Why, and where the action stands and can be appealed (#3115).
    const why =
      "A moderation action keeps your account from creating invites for now. To see why or to appeal, open “Account status” under “Account and data” in the settings.";

    await app.user.click(main().getByRole("button", { name: "Create invite" }));

    expect(words(await main().findByRole("region", { name: cannot }))).toContain(why);
    expect(main().queryAllByRole("textbox")).toHaveLength(0);
    expect(app.path()).toBe(creating);

    await app.user.click(main().getByRole("button", { name: "Invites" }));

    await at(app, invites, "Invites");
    expect(words(await main().findByRole("region", { name: cannot }))).toContain(why);
    await list();
    expect(main().queryAllByRole("link", { name: "Create invite" })).toHaveLength(0);
  });
});

// ADR-0152: a one-person server has no invites, so the settings have no such page.
describe("on a one-person server", () => {
  for (const address of [invites, creating, `${invites}/01900000-0000-7000-8000-000000000000`])
    test(`${address} says under the heading Settings that the settings page does not exist, beside a menu without Invites`, async () => {
      const app = await renderApp(address, onePerson);

      const missing = await main().findByRole("alert");
      expect(words(missing)).toMatch(/^This settings page does not exist\. Check the address\.\s*See all settings$/);
      expect(heading()).toBe("Settings");
      expect(app.title()).toContain("Settings");
      await readsLeft(app);
      expect(menu()).toEqual(rowsAt(address, { withInvites: false }));
      expect(main().queryAllByRole("region")).toHaveLength(0);
    });
});

// What the rules are and how a tree breaks each is the check's own test (design-system, test/settings.dom.test.tsx).
describe("as the app draws them, the screens keep the rules of a settings screen (ADR-0110 §6)", () => {
  for (const [what, address, options, shows] of [
    ["the list", () => invites, {}, "Your invites"],
    ["the list without an invite", () => invites, { access: { invites: "none" } }, "Your invites"],
    ["the list at the server's limit", () => invites, { access: { invites: "at-cap" } }, "Your invites"],
    ["the list of a reader who may not create invites", () => invites, notAllowed, cannot],
    ["the form", () => creating, {}, "New invite"],
    ["the form's screen of a reader who may not create invites", () => creating, notAllowed, cannot],
    ["a usable invite", (fake: FakeApi) => `${invites}/${fake.inviteId(0)}`, {}, "Revoke invite"],
    ["a revoked invite", (fake: FakeApi) => `${invites}/${fake.inviteId(2)}`, {}, "Status"],
  ] as const)
    test(what, async () => {
      const on = fakeApp(options);
      await renderApp(address(on.fake), { on });
      await main().findByRole("region", { name: new RegExp(`^${shows}(\\s*\\d+)?$`) });

      expect(beside()).toHaveLength(1);
      expect(settingsFindings(document.body)).toEqual([]);
    });

  test("the invite that was created, in the form's place", async () => {
    const app = await renderApp(creating);
    await app.user.click(await main().findByRole("button", { name: "Create invite" }));
    await main().findByRole("region", { name: "The invite is created" });

    expect(beside()).toHaveLength(1);
    expect(settingsFindings(document.body)).toEqual([]);
  });
});
