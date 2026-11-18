// SPDX-License-Identifier: AGPL-3.0-only

// Keyword filters in the settings (#3202, ADR-0157 §10), as the app places them: the list at /settings/filters, which
// the menu's row leads to; a filter's editor at /settings/filters/new and /settings/filters/<id>, under the same row
// of the menu, which the list leads to and whose way back leads to the list; a filter added, changed or deleted there,
// after which the app is at the list and the list shows it, and the home timeline folds what the new filter matches;
// the date and time a filter ends at, read on the reader's clock; the navigation the app holds while the editor has
// unsaved changes; and the rules of a settings screen, kept by every state of both screens. What the list and the
// editor show, send and say is the feature's (app-features, settings/filters.dom.test.tsx and
// filters-edit.dom.test.tsx); what a folded post is, the post's (app-components, post-filter.dom.test.tsx).

import { afterAll, beforeAll, describe, expect, onTestFinished, test } from "bun:test";
import { settingsFindings } from "@my/design-system/test/settings";
import type { FakeOptions } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, words, type TestApp } from "./test/app";
import { fakeApp } from "./test/server";
import { rail } from "./test/viewport";

const list = "/settings/filters";
const fresh = "/settings/filters/new";

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
    .map((link) => [
      link.textContent.replace(/\s*\d+\s*\d+ items? to check$/, ""),
      link.getAttribute("href"),
      link.getAttribute("aria-current"),
    ]);

/** The settings' menus outside the screen: the one the frame places beside an entry. */
const beside = () =>
  screen.queryAllByRole("navigation", { name: "Settings" }).filter((menu) => !screen.getByRole("main").contains(menu));

/** The rows of the menu beside a screen of Keyword filters at `at`: Keyword filters is the entry the reader is at. */
const besideFilters = (at: string) => [
  ["Profile", "/settings/profile", null],
  ["Display and posting", "/settings/display", null],
  ["Notifications", "/settings/notifications", null],
  ["Relationships", "/settings/relationships", null],
  ["Keyword filters", list, "page"],
  ["Security", "/settings/security", null],
  ["Apps", "/settings/apps", null],
  ["Invites", "/settings/invites", null],
  ["Account and data", "/settings/account", null],
  ["Sign out…", `${at}?sign_out=true`, null],
];

/**
 * The menu beside a screen opened by its address once it lists Invites: whether the reader may create invites is
 * read as the app opens there, and the entry comes with the answer.
 */
const withInvites = () => screen.findByRole("link", { name: "Invites" });

/** Waits until the list is drawn: its section is in place. */
const listRead = () => main().findByRole("region", { name: /^Filters/ });
/** Waits until the editor is drawn, and returns the field of the name. */
const editorRead = () => main().findByRole<HTMLInputElement>("textbox", { name: "Name" });
const nameField = () => main().getByRole<HTMLInputElement>("textbox", { name: "Name" });
/** The names of the filters the list shows, in its order: each row's first words. */
const listed = () =>
  within(main().getByRole("list", { name: "Filters" }))
    .getAllByRole("link")
    .map((row) => /^(.*?)\s*(Expired\s*)?(Folds|Hides) matching posts/.exec(words(row))?.[1]);

/** Puts `text` in `field` in place of what it holds. */
async function rewrite({ user }: TestApp, field: HTMLElement, text: string) {
  await user.clear(field);
  await user.click(field);
  await user.paste(text);
}

/** The marks that isolate a name's direction inside a sentence (U+2066 to U+2069), which no reader sees. */
const isolates = new RegExp(`[${String.fromCodePoint(0x2066)}-${String.fromCodePoint(0x2069)}]`, "g");

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

type SeedFilter = NonNullable<NonNullable<FakeOptions["settings"]>["filters"]>[number];

/** A filter that folds at home, with `count` words. */
const filterOf = (title: string, count: number): SeedFilter => ({
  title,
  action: "warn",
  contexts: ["home"],
  keywords: Array.from({ length: count }, (_, i) => ({ keyword: `${title} ${String(i + 1)}`, whole_word: false })),
});

describe("the list", () => {
  test("is the second row of the menu's group People and filters, and opens at its address with the menu beside it marking it", async () => {
    const app = await renderApp("/settings");
    const group = within(main().getByRole("navigation", { name: "Settings" })).getByRole("region", {
      name: "People and filters",
    });
    const [, second] = within(group).getAllByRole("link");
    if (second === undefined) throw new Error("test: the group has no second row");
    expect(second.getAttribute("href")).toBe(list);

    await app.user.click(second);

    await listRead();
    expect(app.path()).toBe(list);
    expect(heading()).toBe("Keyword filters");
    expect(app.title()).toContain("Keyword filters");
    await withInvites();
    expect(beside().flatMap(rows)).toEqual(besideFilters(list));
    expect(listed()).toEqual(["Spoilers", "Election season", "Marathon week"]);
  });

  test("leads to the editor of a new filter with Add a filter, and the editor's way back, named Keyword filters, leads to the list", async () => {
    const app = await renderApp(list);
    await listRead();

    await app.user.click(main().getByRole("link", { name: "Add a filter" }));

    expect((await editorRead()).value).toBe("");
    expect(app.path()).toBe(fresh);
    expect(heading()).toBe("New filter");
    expect(app.title()).toContain("New filter");
    await withInvites();
    // The editor is a screen of the same entry: the menu beside it still marks Keyword filters.
    expect(beside().flatMap(rows)).toEqual(besideFilters(fresh));

    await app.user.click(main().getByRole("button", { name: "Keyword filters" }));

    await listRead();
    expect(app.path()).toBe(list);
    expect(heading()).toBe("Keyword filters");
    expect(confirmations()).toEqual([]);
  });

  test("leads to a filter's editor by its row, at the filter's address", async () => {
    const app = await renderApp(list);
    await listRead();

    await app.user.click(main().getByRole("link", { name: /^Election season/ }));

    expect((await editorRead()).value).toBe("Election season");
    expect(app.path()).toBe(app.fake.filterPath(1));
    expect(heading()).toBe("Edit filter");
    expect(app.title()).toContain("Edit filter");
    await withInvites();
    expect(beside().flatMap(rows)).toEqual(besideFilters(app.fake.filterPath(1)));
  });

  for (const at of [list, fresh])
    test(`asks a reader without a session to sign in, at ${at}`, async () => {
      await renderApp(at, { visitor: true });

      expect(main().getByRole("status").textContent).toMatch(/^Sign in to see this screen\.\s*Sign in$/);
      expect(main().queryAllByRole("textbox")).toHaveLength(0);
      expect(main().queryAllByRole("region")).toHaveLength(0);
    });
});

describe("the editor", () => {
  test("opens at a filter's address with what the filter has, as the app's first screen too", async () => {
    const on = fakeApp();
    const app = await renderApp(on.fake.filterPath(0), { on });

    expect((await editorRead()).value).toBe("Spoilers");
    expect(heading()).toBe("Edit filter");
    expect(app.path()).toBe(on.fake.filterPath(0));
  });

  test("at an address that names no filter says that the filter does not exist, and leads to the list", async () => {
    const app = await renderApp(`${list}/nothing`);

    const missing = await main().findByRole("alert");
    expect(missing.textContent).toMatch(/^This filter does not exist\. It may have been deleted\.\s*See all filters$/);
    await withInvites();
    expect(beside().flatMap(rows)).toEqual(besideFilters(`${list}/nothing`));

    await app.user.click(within(missing).getByRole("link", { name: "See all filters" }));

    await listRead();
    expect(app.path()).toBe(list);
  });
});

describe("after the editor's write, the app is at the list, which shows it", () => {
  test("a filter added: the list counts and shows it last, and its row leads to its editor", async () => {
    const app = await renderApp(fresh);
    await editorRead();
    await rewrite(app, nameField(), "Sourdough");
    await rewrite(app, main().getByRole("textbox", { name: "Word or phrase 1" }), "levain");

    await app.user.click(main().getByRole("button", { name: "Add" }));

    await settledAt(app, list);
    await main().findByRole("region", { name: /^Filters\s*4$/ });
    expect(heading()).toBe("Keyword filters");
    expect(listed()).toEqual(["Spoilers", "Election season", "Marathon week", "Sourdough"]);
    const row = main().getByRole("link", { name: /^Sourdough/ });
    expect(words(row)).toMatch(
      /^Sourdough Folds matching posts · 1 word In home and lists, notifications, public timelines, threads, and profiles · Never ends$/,
    );
    expect(row.getAttribute("href")).toBe(app.fake.filterPath(3));
    expect(confirmations()).toEqual([]);
    expect(app.left()).toEqual([]);
  });

  test("a filter saved with nothing changed: nothing is sent, and the app is at the list", async () => {
    const on = fakeApp();
    const app = await renderApp(on.fake.filterPath(0), { on });
    await editorRead();
    const sent = on.fake.script("PATCH /filters/{id}");

    await app.user.click(main().getByRole("button", { name: "Save" }));

    await settledAt(app, list);
    await listRead();
    expect(sent.seen).toHaveLength(0);
    expect(listed()).toEqual(["Spoilers", "Election season", "Marathon week"]);
  });

  test("a filter changed: its row says the change", async () => {
    const on = fakeApp();
    const app = await renderApp(on.fake.filterPath(0), { on });
    await editorRead();
    await rewrite(app, nameField(), "Spoilers and more");
    await app.user.selectOptions(main().getByRole("combobox", { name: "Posts that match" }), "Hide completely");

    await app.user.click(main().getByRole("button", { name: "Save" }));

    await settledAt(app, list);
    await main().findByRole("link", { name: /^Spoilers and more\s*Hides matching posts/ });
    expect(listed()).toEqual(["Spoilers and more", "Election season", "Marathon week"]);
    // The editor opened again holds the change, too.
    await app.navigate(on.fake.filterPath(0));
    expect((await editorRead()).value).toBe("Spoilers and more");
  });

  test("a filter deleted, once the reader confirmed: the list is without it", async () => {
    const on = fakeApp();
    const gone = on.fake.filterPath(0);
    const app = await renderApp(gone, { on });
    await editorRead();

    await app.user.click(main().getByRole("button", { name: "Delete…" }));
    const asking = await waitFor(() => {
      const [dialog] = screen.queryAllByRole("alertdialog");
      if (dialog === undefined) throw new Error(`test: no confirmation at ${app.path()}`);
      return dialog;
    });
    expect(confirmations().map((title) => title?.replace(isolates, ""))).toEqual(["Delete the filter “Spoilers”?"]);
    expect(app.path()).toBe(gone);
    await app.user.click(within(asking).getByRole("button", { name: "Delete" }));

    await settledAt(app, list);
    await main().findByRole("region", { name: /^Filters\s*2$/ });
    expect(listed()).toEqual(["Election season", "Marathon week"]);
    // Its address names no filter any more.
    await app.navigate(gone);
    await waitFor(() => {
      if (main().queryAllByRole("alert").length === 0)
        throw new Error(`test: at ${app.path()} the screen says "${words(screen.getByRole("main")).slice(0, 300)}"`);
    });
    expect(main().getByRole("alert").textContent).toContain("This filter does not exist.");
  });

  test("a filter deleted: the confirmation stays busy until the app is at the list, nothing reads the filter again, and nothing says it does not exist", async () => {
    const on = fakeApp();
    const gone = on.fake.filterPath(0);
    const app = await renderApp(gone, { on });
    await editorRead();
    await app.user.click(main().getByRole("button", { name: "Delete…" }));
    const asking = await waitFor(() => {
      const [dialog] = screen.queryAllByRole("alertdialog");
      if (dialog === undefined) throw new Error(`test: no confirmation at ${app.path()}`);
      return dialog;
    });
    const reads = on.fake.script("GET /filters/{id}");
    // Each state the page passes through: where the app is, what the confirmation is, and whether a line says that
    // the filter does not exist.
    const states: string[] = [];
    const look = () => {
      const [dialog] = screen.queryAllByRole("alertdialog");
      const confirmation =
        dialog === undefined
          ? "closed"
          : within(dialog).queryAllByRole("button", { name: "Delete" })[0]?.getAttribute("aria-busy") === "true"
            ? "busy"
            : "open";
      const missing = document.body.textContent.includes("This filter does not exist") ? " missing" : "";
      const state = `${app.path()} ${confirmation}${missing}`;
      if (states.at(-1) !== state) states.push(state);
    };
    const watching = new MutationObserver(look);
    watching.observe(document.body, { subtree: true, childList: true, attributes: true, characterData: true });
    onTestFinished(() => {
      watching.disconnect();
    });

    await app.user.click(within(asking).getByRole("button", { name: "Delete" }));
    await settledAt(app, list);
    await main().findByRole("region", { name: /^Filters\s*2$/ });
    await waitFor(() => {
      if (app.data.queryClient.isFetching() > 0) throw new Error("test: the app is still reading");
    });
    watching.disconnect();

    expect(reads.seen).toHaveLength(0);
    expect(states.filter((state) => state.endsWith(" missing"))).toEqual([]);
    // From the press on, the confirmation is busy until it closes, and it closes at the list.
    const pressed = states.findIndex((state) => state.endsWith(" busy"));
    expect(pressed).toBeGreaterThanOrEqual(0);
    expect(states.slice(pressed).filter((state) => state.endsWith(" open"))).toEqual([]);
    expect(states.filter((state) => state.endsWith(" closed")).every((state) => state.startsWith(`${list} `))).toBe(
      true,
    );
  });

  test("the home timeline folds what a filter just added matches, under its name", async () => {
    const app = await renderApp(fresh, { settings: { filters: [] } });
    await editorRead();
    await rewrite(app, nameField(), "Baking");
    await rewrite(app, main().getByRole("textbox", { name: "Word or phrase 1" }), "bread");
    await app.user.click(main().getByRole("button", { name: "Add" }));
    await settledAt(app, list);

    await app.navigate("/");

    const folded = await main().findAllByRole("article", { name: /^Matches filter “\s*Baking\s*”/ });
    expect(folded.length).toBeGreaterThan(0);
    for (const post of folded) expect(within(post).getAllByRole("button", { name: "Show" })).toHaveLength(1);
  });
});

// The device is in one zone and the reader chose another: the date and time the reader writes is on their clock, as
// every time the app shows them is.
describe("the date and time a filter ends at", () => {
  const device = process.env.TZ;
  beforeAll(() => {
    process.env.TZ = "Asia/Tokyo";
  });
  afterAll(() => {
    if (device === undefined) delete process.env.TZ;
    else process.env.TZ = device;
  });

  test("is read in the reader's time zone, not the device's", async () => {
    const on = fakeApp({
      preferences: { locale: "en", time_zone: "America/New_York", time_zone_follows_browser: false },
    });
    const app = await renderApp(on.fake.filterPath(0), { on });
    await editorRead();
    const sent = on.fake.script("PATCH /filters/{id}");
    await app.user.selectOptions(main().getByRole("combobox", { name: "Ends" }), "At a date and time");
    const field = await main().findByLabelText<HTMLInputElement>("Ends at");

    // A winter's day far ahead: New York is five hours behind UTC, Tokyo nine ahead.
    await app.user.clear(field);
    await app.user.type(field, "2099-02-03T04:05");
    await app.user.click(main().getByRole("button", { name: "Save" }));

    await settledAt(app, list);
    expect(sent.seen.map((request) => request.body.expires_at)).toEqual(["2099-02-03T09:05:00.000Z"]);
    // And the list writes the day on the same clock.
    expect(words(main().getByRole("link", { name: /^Spoilers/ }))).toMatch(/· Ends Feb 3, 2099$/);
  });
});

describe("leaving the editor with unsaved changes", () => {
  /** The editor of Spoilers with a name written and not saved. */
  async function unsaved() {
    const on = fakeApp();
    const app = await renderApp(on.fake.filterPath(0), { on });
    await editorRead();
    await rewrite(app, nameField(), "Spoilers and more");
    return app;
  }

  // Each way out of the editor: what the reader presses, and where it leads once they chose to leave.
  const ways = [
    {
      way: "the editor's way back",
      leave: (app: TestApp) => app.user.click(main().getByRole("button", { name: "Keyword filters" })),
      to: list,
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
    test(`${way} asks first, and Keep editing stays on the editor with what was written`, async () => {
      const app = await unsaved();
      const at = app.path();

      await leave(app);

      const asking = await asked();
      expect(app.path()).toBe(at);
      expect(
        within(asking)
          .getAllByRole("button")
          .map((button) => button.getAttribute("aria-label") ?? button.textContent),
      ).toEqual(["Close", "Keep editing", "Leave"]);

      await app.user.click(within(asking).getByRole("button", { name: "Keep editing" }));

      await settledAt(app, at);
      expect(heading()).toBe("Edit filter");
      expect(nameField().value).toBe("Spoilers and more");
    });

    test(`${way} asks first, and Leave goes there without saving`, async () => {
      const app = await unsaved();
      const sent = app.fake.script("PATCH /filters/{id}");

      await leave(app);
      await app.user.click(within(await asked()).getByRole("button", { name: "Leave" }));

      await settledAt(app, to);
      expect(main().queryAllByRole("textbox", { name: "Name" })).toHaveLength(0);
      expect(sent.seen).toHaveLength(0);
    });
  }

  // The browser's own Back is held too, which a browser decides: on a simulation the history goes back without the
  // app being asked (web/e2e/specs/profile.spec.ts holds it for the profile's form, the same mechanism).

  test("what was written and left is gone: the editor opened again holds the filter's name", async () => {
    const app = await unsaved();
    const at = app.path();
    await app.user.click(main().getByRole("button", { name: "Keyword filters" }));
    await app.user.click(within(await asked()).getByRole("button", { name: "Leave" }));
    await settledAt(app, list);
    await listRead();
    expect(listed()).toEqual(["Spoilers", "Election season", "Marathon week"]);

    await app.navigate(at);

    expect((await editorRead()).value).toBe("Spoilers");
  });

  test("a new filter with something written is held too", async () => {
    const app = await renderApp(fresh);
    await editorRead();
    await rewrite(app, nameField(), "Sourdough");

    await app.user.click(main().getByRole("button", { name: "Keyword filters" }));

    await asked();
    expect(app.path()).toBe(fresh);
  });

  test("Sign out over the editor leaves for the start without asking whether to leave it", async () => {
    const app = await unsaved();
    const [menu] = beside();
    if (menu === undefined) throw new Error("test: no settings menu beside the screen");
    await app.user.click(within(menu).getByRole("link", { name: "Sign out…" }));

    await app.user.click(
      within(await screen.findByRole("alertdialog", { name: "Sign out of this browser?" })).getByRole("button", {
        name: "Sign out",
      }),
    );

    await waitFor(() => {
      if (app.left().length === 0) throw new Error("test: the app did not leave yet");
    });
    expect(app.left()).toEqual(["/"]);
    expect(confirmations().includes("Leave without saving?")).toBe(false);
  });

  test("an editor with nothing changed is left without a question", async () => {
    const on = fakeApp();
    const app = await renderApp(on.fake.filterPath(0), { on });
    await editorRead();

    await app.user.click(main().getByRole("button", { name: "Keyword filters" }));

    await listRead();
    expect(app.path()).toBe(list);
    expect(confirmations()).toEqual([]);
  });
});

describe("a write of the editor answered after the reader left it", () => {
  /** The toasts that say a setting was not saved. */
  const toasts = () =>
    screen
      .queryAllByRole("status")
      .filter((toast) => toast.textContent.includes("could not be saved. Open the settings and change it again."));

  /** The editor of Spoilers renamed, its Save pressed and held as `hold` says, and the reader gone to Lists. */
  async function leftWhileSaving(script: { hold: "request" | "response"; fail?: 500 }) {
    const on = fakeApp();
    const at = on.fake.filterPath(0);
    const app = await renderApp(at, { on });
    await editorRead();
    await rewrite(app, nameField(), "Spoilers and more");
    const sent = on.fake.script("PATCH /filters/{id}", script);
    await app.user.click(main().getByRole("button", { name: "Save" }));
    await sent.arrived();
    await app.navigate("/lists");
    await settledAt(app, "/lists");
    return { app, at, sent };
  }

  /** Waits until nothing is read and a turn passed: whatever the answer leads to has happened. */
  const atRest = async (app: TestApp) => {
    await waitFor(() => {
      if (app.data.queryClient.isFetching() > 0) throw new Error("test: the app is still reading");
    });
    await new Promise((turn) => setTimeout(turn, 0));
  };

  test("a Save the server took takes the reader nowhere", async () => {
    const { app, sent } = await leftWhileSaving({ hold: "response" });

    sent.release();
    await atRest(app);

    expect(app.path()).toBe("/lists");
    expect(toasts()).toHaveLength(0);
  });

  test("a Save that failed: a toast names the filter and leads back to its editor", async () => {
    const { app, at, sent } = await leftWhileSaving({ hold: "request", fail: 500 });
    expect(toasts()).toHaveLength(0);

    sent.release();

    const [toast, ...others] = await waitFor(() => {
      if (toasts().length === 0) throw new Error("test: no toast says that the filter was not saved");
      return toasts();
    });
    if (toast === undefined) throw new Error("test: no toast");
    expect(others).toHaveLength(0);
    expect(toast.textContent.replace(isolates, "")).toMatch(/^“Spoilers( and more)?” could not be saved\./);
    expect(app.path()).toBe("/lists");

    await app.user.click(within(toast).getByRole("button", { name: "Open settings" }));

    await editorRead();
    expect(app.path()).toBe(at);
  });

  test("a delete the server took after the reader left takes them nowhere", async () => {
    const on = fakeApp();
    const app = await renderApp(on.fake.filterPath(0), { on });
    await editorRead();
    await app.user.click(main().getByRole("button", { name: "Delete…" }));
    const asking = await waitFor(() => {
      const [dialog] = screen.queryAllByRole("alertdialog");
      if (dialog === undefined) throw new Error(`test: no confirmation at ${app.path()}`);
      return dialog;
    });
    const sent = on.fake.script("DELETE /filters/{id}", { hold: "response" });
    await app.user.click(within(asking).getByRole("button", { name: "Delete" }));
    await sent.served();
    await app.navigate("/lists");
    await settledAt(app, "/lists");

    sent.release();
    await atRest(app);

    expect(app.path()).toBe("/lists");
  });
});

// What the rules are and how a tree breaks each is the check's own test (design-system, test/settings.dom.test.tsx).
describe("the screens of Keyword filters, as the app draws them, keep the rules of a settings screen (ADR-0110 §6)", () => {
  /** The address of the reader's filter at an index, on the server the test renders. */
  type At = string | number;
  const nearLimit = [
    ...Array.from({ length: 9 }, (_, i) => filterOf(`Topic ${String(i + 1)}`, 100)),
    filterOf("Last", 93),
  ];

  // The screen, where it is (an address, or the index of a filter), the server, and what shows that it is drawn.
  const drawn: readonly [string, At, FakeOptions, () => Promise<unknown>][] = [
    ["the list", list, {}, listRead],
    ["the list with no filter", list, { settings: { filters: [] } }, listRead],
    [
      "the list at the most filters a reader can have",
      list,
      { settings: { filters: Array.from({ length: 100 }, (_, i) => filterOf(`F${String(i)}`, 1)) } },
      listRead,
    ],
    ["the editor of a new filter", fresh, {}, editorRead],
    ["the editor of a filter that folds", 0, {}, editorRead],
    ["the editor of a filter that ends", 1, {}, editorRead],
    ["the editor of a filter that ended", 2, {}, editorRead],
    ["the editor of a new filter near the most words of all", fresh, { settings: { filters: nearLimit } }, editorRead],
    ["the editor at an address that names no filter", `${list}/nothing`, {}, () => main().findByRole("alert")],
  ];

  for (const [what, at, options, read] of drawn)
    test(what, async () => {
      const on = fakeApp(options);
      await renderApp(typeof at === "number" ? on.fake.filterPath(at) : at, { on });
      await read();

      // The menu stands beside the screen.
      expect(beside()).toHaveLength(1);
      expect(settingsFindings(document.body)).toEqual([]);
    });

  test("the editor of a filter of 100 words, the most it can have", async () => {
    const on = fakeApp({ settings: { filters: [filterOf("Full", 100)] } });
    await renderApp(on.fake.filterPath(0), { on });
    await editorRead();

    expect(main().getAllByRole("textbox", { name: /^Word or phrase \d+$/ })).toHaveLength(100);
    expect(settingsFindings(document.body)).toEqual([]);
  });

  test("the editor after a Save the server refused, with the refusal under the name", async () => {
    const on = fakeApp({ settings: { filterSave: "title" } });
    const app = await renderApp(on.fake.filterPath(0), { on });
    await editorRead();
    // A filter saved unchanged sends nothing to refuse.
    await rewrite(app, nameField(), "Spoilers and more");

    await app.user.click(main().getByRole("button", { name: "Save" }));

    await main().findByRole("alert");
    expect(nameField().getAttribute("aria-invalid")).toBe("true");
    expect(settingsFindings(document.body)).toEqual([]);
  });

  test("the editor while a Save is on its way, with its button busy", async () => {
    const on = fakeApp({ settings: { filterSave: "waits" } });
    const app = await renderApp(on.fake.filterPath(0), { on });
    await editorRead();
    // A filter saved unchanged sends nothing to wait for.
    await rewrite(app, nameField(), "Spoilers and more");

    await app.user.click(main().getByRole("button", { name: "Save" }));

    await waitFor(() => {
      if (main().getByRole("button", { name: "Save" }).getAttribute("aria-busy") !== "true")
        throw new Error("test: Save is not busy yet");
    });
    expect(settingsFindings(document.body)).toEqual([]);
  });

  // The app shows the screen once its filters answered, so that it loads them is the feature's to show
  // (filters.dom.test.tsx): no route test reaches it.
  test("the list when the filters could not be read", async () => {
    const on = fakeApp();
    const app = await renderApp("/lists", { on });
    on.fake.script("GET /filters", { fail: 500 });

    await app.navigate(list);

    expect((await main().findByRole("alert")).textContent).toContain("Your settings could not be loaded.");
    expect(heading()).toBe("Keyword filters");
    expect(beside()).toHaveLength(1);
    expect(settingsFindings(document.body)).toEqual([]);
  });
});
