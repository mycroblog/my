// SPDX-License-Identifier: AGPL-3.0-only

// Relationships in the settings (#3202), as the app places it: the entry at /settings/relationships, which the menu's
// row leads to; the whole of each list at an address of its own under it, which the entry's "See all" row leads to and
// whose way back leads to the entry, with the menu still marking the entry; an address that names no list; a row's
// account, which leads to its profile; and the rules of a settings screen, kept by every state of both screens. What
// the lists show and what undoing a row does is the feature's (app-features, settings/relationships.dom.test.tsx);
// the menu's rows and groups are settings.dom.test.tsx.

import { describe, expect, test } from "bun:test";
import { settingsFindings } from "@my/design-system/test/settings";
import { scenario, type FakeOptions } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { main, renderApp } from "./test/app";
import { fakeApp } from "./test/server";

const entry = "/settings/relationships";

/** The scenario's server with two muted servers and two followed hashtags beside its blocks and its mute. */
const some: FakeOptions = { domainMutes: ["spam.example", "ads.example"], followedTags: ["baking", "photography"] };

/** Nine muted servers, of which the entry shows five and the way to all, and seven followed hashtags, shown whole. */
const many: FakeOptions = {
  domainMutes: Array.from({ length: 9 }, (_, i) => `server-${String(i + 1)}.example`),
  followedTags: ["baking", "photography", "hiking", "sourdough", "film", "mountains", "rivers"],
};

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

/** The rows of the menu beside a screen of Relationships at `at`: Relationships is the entry the reader is at. */
const besideRelationships = (at: string) => [
  ["Profile", "/settings/profile", null],
  ["Display and posting", "/settings/display", null],
  ["Notifications", "/settings/notifications", null],
  ["Voice chat", "/settings/voice", null],
  ["Events", "/settings/events", null],
  ["Relationships", entry, "page"],
  ["Keyword filters", "/settings/filters", null],
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

/** Waits until the entry's lists are drawn: its sections are in place. */
const entryRead = () => main().findByRole("region", { name: /^Blocked accounts/ });
/** Waits until a whole list is drawn: its section, which its count names, has rows. */
const listRead = () => main().findAllByRole("listitem");

describe("the entry", () => {
  test("is the first row of the menu's group People and filters, and opens at its address with the menu beside it marking it", async () => {
    const app = await renderApp("/settings", some);
    const group = within(main().getByRole("navigation", { name: "Settings" })).getByRole("region", {
      name: "People and filters",
    });
    const [first] = within(group).getAllByRole("link");
    if (first === undefined) throw new Error("test: the group has no row");
    expect(first.getAttribute("href")).toBe(entry);

    await app.user.click(first);

    await entryRead();
    expect(app.path()).toBe(entry);
    expect(heading()).toBe("Relationships");
    expect(app.title()).toContain("Relationships");
    await withInvites();
    expect(beside().flatMap(rows)).toEqual(besideRelationships(entry));
    // Its four lists, as the feature draws them.
    expect(main().getAllByRole("region")).toHaveLength(4);
  });

  for (const at of [entry, `${entry}/blocked`])
    test(`asks a reader without a session to sign in, at ${at}`, async () => {
      await renderApp(at, { visitor: true });

      expect(main().getByRole("status").textContent).toMatch(/^Sign in to see this screen\.\s*Sign in$/);
      expect(main().queryAllByRole("region")).toHaveLength(0);
      expect(main().queryAllByRole("listitem")).toHaveLength(0);
    });

  test("a row's account leads to its profile", async () => {
    const app = await renderApp(entry, some);
    await entryRead();

    await app.user.click(main().getByRole("link", { name: "QuickDeals" }));

    await waitFor(() => {
      if (app.path() !== "/@quickdeals") throw new Error(`test: the app is at ${app.path()}`);
    });
  });
});

describe("the whole of a list", () => {
  test("is where the entry's See all row leads, a screen titled by the list under the same row of the menu, whose way back, named Relationships, leads to the entry", async () => {
    const app = await renderApp(entry, many);
    await entryRead();

    await app.user.click(main().getByRole("link", { name: "See all muted servers" }));

    await main().findByRole("region", { name: "9 servers" });
    expect(app.path()).toBe(`${entry}/servers`);
    expect(heading()).toBe("Muted servers");
    expect(app.title()).toContain("Muted servers");
    expect(main().getAllByRole("listitem")).toHaveLength(9);
    await withInvites();
    // A screen of the same entry: the menu beside it still marks Relationships.
    expect(beside().flatMap(rows)).toEqual(besideRelationships(`${entry}/servers`));

    await app.user.click(main().getByRole("button", { name: "Relationships" }));

    await entryRead();
    expect(app.path()).toBe(entry);
    expect(heading()).toBe("Relationships");
  });

  for (const [list, title, count] of [
    ["blocked", "Blocked accounts", "2 accounts"],
    ["muted", "Muted accounts", "1 account"],
    ["servers", "Muted servers", "2 servers"],
    ["hashtags", "Followed hashtags", "2 hashtags"],
  ] as const)
    test(`of the ${title.toLowerCase()} opens at /settings/relationships/${list}, as the app's first screen too`, async () => {
      const app = await renderApp(`${entry}/${list}`, some);

      await main().findByRole("region", { name: count });
      expect(heading()).toBe(title);
      expect(app.title()).toContain(title);
      expect(app.path()).toBe(`${entry}/${list}`);
    });

  test("at an address that names no list says under the entry's title, Relationships, that the page does not exist, with the menu still marking the entry, and leads to the entry", async () => {
    const app = await renderApp(`${entry}/nothing`, some);

    const missing = await main().findByRole("alert");
    expect(missing.textContent).toMatch(/^This settings page does not exist\. Check the address\.\s*Relationships$/);
    // No list's name is known: the screen and the page are titled as the entry is.
    expect(heading()).toBe("Relationships");
    expect(app.title()).toContain("Relationships");
    expect(main().queryAllByRole("listitem")).toHaveLength(0);
    await withInvites();
    expect(beside().flatMap(rows)).toEqual(besideRelationships(`${entry}/nothing`));

    await app.user.click(within(missing).getByRole("link", { name: "Relationships" }));

    await entryRead();
    expect(app.path()).toBe(entry);
  });
});

// What the rules are and how a tree breaks each is the check's own test (design-system, test/settings.dom.test.tsx).
describe("the screens of Relationships, as the app draws them, keep the rules of a settings screen (ADR-0110 §6)", () => {
  // The screen, where it is, the server it is drawn on, and what shows that it is drawn.
  const drawn: readonly [string, string, FakeOptions, () => Promise<unknown>][] = [
    ["the entry", entry, some, entryRead],
    ["the entry with every list empty", entry, { seed: { people: scenario.people } }, entryRead],
    ["the entry with a list cut short and one of seven rows", entry, many, entryRead],
    ["the whole list of nine muted servers", `${entry}/servers`, many, listRead],
    ["the whole list of the blocked accounts", `${entry}/blocked`, some, listRead],
    ["a list that does not exist", `${entry}/nothing`, some, () => main().findByRole("alert")],
  ];

  for (const [what, at, options, read] of drawn)
    test(what, async () => {
      await renderApp(at, options);
      await read();

      // The menu stands beside the screen.
      expect(beside()).toHaveLength(1);
      expect(settingsFindings(document.body)).toEqual([]);
    });

  test("the entry after an undo, with the row that says what was done", async () => {
    const app = await renderApp(entry, some);
    await entryRead();

    await app.user.click(
      within(main().getByRole("group", { name: "spam.example" })).getByRole("button", { name: "Unmute" }),
    );

    await main().findByText("Unmuted");
    expect(settingsFindings(document.body)).toEqual([]);
  });

  // The app shows the screen once its lists answered, so that it loads them is the feature's to show
  // (relationships.dom.test.tsx): no route test reaches it.
  test("the entry when its lists could not be read", async () => {
    const on = fakeApp(some);
    const app = await renderApp("/lists", { on });
    for (const route of ["GET /blocks", "GET /mutes", "GET /domain_mutes", "GET /followed_tags"] as const)
      on.fake.script(route, { fail: 500 });

    await app.navigate(entry);

    expect((await main().findByRole("alert")).textContent).toContain("Your settings could not be loaded.");
    expect(heading()).toBe("Relationships");
    expect(beside()).toHaveLength(1);
    expect(settingsFindings(document.body)).toEqual([]);
  });
});
