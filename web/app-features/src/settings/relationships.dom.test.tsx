// SPDX-License-Identifier: AGPL-3.0-only

// Relationships, the settings' entry (#3202, FR-303, FR-304): the accounts the reader blocks and mutes, the servers
// they mute and the hashtags they follow, each a section read from its list of the server, with its count, its rows
// and the button that takes a row out; a long list cut to its first rows with the way to the whole list, which is a
// screen of its own and pages through the server's list; and undoing, which acts at once and leaves the row where it
// was, saying what was done, until the reader leaves. Before the lists are read the screen says that it loads them,
// and when they cannot be read, that. Where the focus goes after an undo is relationships.jsdom.test.tsx; the rows
// themselves are the design system's (settings-rows.dom.test.tsx); the entry's place in the settings and its
// addresses are the app's (web/app/src/settings-relationships.dom.test.tsx).

import { describe, expect, test } from "bun:test";
import { blocksQuery } from "@my/app-data";
import { cleanup, isInaccessible, screen, waitFor, within } from "@testing-library/react";
import { fakeData } from "../test/feature";
import {
  first,
  headed,
  inOrder,
  lists,
  plain,
  read,
  relationships,
  rows,
  said,
  section,
  serverOf,
  some,
  writes,
  type ListKind,
} from "../test/relationships";

/** The entry on a server of `options`, once its lists are read. */
async function entry(options = some) {
  const on = fakeData(options);
  const drawn = await relationships(on);
  await screen.findAllByRole("region");
  await read(on);
  return { ...on, ...drawn };
}

/** The screen of the whole list `list` on a server of `options`, once the list is read. */
async function whole(list: string, options = some) {
  const on = fakeData(options);
  const drawn = await relationships(on, { list });
  await screen.findAllByRole("listitem");
  await read(on);
  return { ...on, ...drawn };
}

/** What each row of `part` says, as a reader reads it. */
const texts = (part: HTMLElement) => rows(part).map((row) => plain(row.textContent));

/** The button that undoes the row whose group is named `group`, in `part`. */
const undoOf = (part: HTMLElement, group: string, undo: string) =>
  within(within(part).getByRole("group", { name: group })).getByRole<HTMLButtonElement>("button", { name: undo });

describe("the entry", () => {
  test("reads the four lists of the server, and shows them as sections in their order, each heading with its count", async () => {
    const { requests } = await entry();

    for (const list of ["GET /blocks", "GET /mutes", "GET /domain_mutes?limit=40", "GET /followed_tags"])
      expect(requests).toContain(list);
    expect(
      screen.getAllByRole("region").map((part) => plain(within(part).getAllByRole("heading")[0]?.textContent)),
    ).toEqual(["Blocked accounts 2", "Muted accounts 1", "Muted servers 2", "Followed hashtags 2"]);
    // Each section's list bears the section's name, without the count.
    for (const { name } of lists) expect(within(section(name)).getAllByRole("list", { name })).toHaveLength(1);
    // Nothing was undone, and nothing asks.
    expect(said()).toEqual([]);
    expect(screen.queryAllByRole("alert")).toHaveLength(0);
  });

  test("a blocked account's row links to its profile by its name, says its whole handle, and holds Unblock in a group named by the handle", async () => {
    await entry();

    const [quickdeals, sora] = rows(section("Blocked accounts"));
    if (quickdeals === undefined || sora === undefined) throw new Error("test: the list has no two rows");
    expect(rows(section("Blocked accounts"))).toHaveLength(2);
    expect(plain(quickdeals.textContent)).toMatch(inOrder("QuickDeals", "@quickdeals@demo.test", "Unblock"));
    expect(within(quickdeals).getByRole("link", { name: "QuickDeals" }).getAttribute("href")).toBe("/@quickdeals");
    expect(quickdeals.contains(undoOf(section("Blocked accounts"), "@quickdeals", "Unblock"))).toBe(true);
    // An account of another server, by its whole handle.
    expect(plain(sora.textContent)).toMatch(inOrder("Sora そら", "@sora@social.example", "Unblock"));
    expect(within(sora).getByRole("link", { name: "Sora そら" }).getAttribute("href")).toBe("/@sora@social.example");
    expect(sora.contains(undoOf(section("Blocked accounts"), "@sora@social.example", "Unblock"))).toBe(true);
    // The avatar is a decoration: the name beside it says whose it is.
    expect(within(quickdeals).queryAllByRole("img")).toHaveLength(0);
  });

  test("a muted account's row says that its notifications are hidden too, and holds Unmute in a group named by the handle", async () => {
    await entry();

    const [kai] = rows(section("Muted accounts"));
    if (kai === undefined) throw new Error("test: the list has no row");
    expect(plain(kai.textContent)).toMatch(inOrder("Kai", "@kai@demo.test", "Notifications hidden too", "Unmute"));
    expect(within(kai).getByRole("link", { name: "Kai" }).getAttribute("href")).toBe("/@kai");
    expect(kai.contains(undoOf(section("Muted accounts"), "@kai", "Unmute"))).toBe(true);
  });

  // The fake's mutes always hide notifications and never end: the server's answer is changed to one that does.
  test("a mute that ends says until when, in place of the notifications' line when it leaves them", async () => {
    const on = fakeData(some);
    // A moment that is always ahead, and always written with its year.
    const ends = new Date(Date.now() + 400 * 24 * 60 * 60 * 1000);
    on.fake.script("GET /mutes", {
      change: (body) => ({
        ...body,
        items: body.items.map((mute) => ({ ...mute, hide_notifications: false, expires_at: ends.toISOString() })),
      }),
    });
    await relationships(on);
    await screen.findAllByRole("region");
    await read(on);

    const until = new Intl.DateTimeFormat("en", { dateStyle: "medium", timeStyle: "short" }).format(ends);
    expect(texts(section("Muted accounts"))).toHaveLength(1);
    expect(texts(section("Muted accounts"))[0]).toMatch(
      inOrder("Kai", "@kai@demo.test", plain(`Until ${until}`), "Unmute"),
    );
    expect(within(section("Muted accounts")).queryAllByText("Notifications hidden too")).toHaveLength(0);
  });

  test("a muted server's row and a followed hashtag's row say its name, with its button in a group the name names; the hashtags stand latest first", async () => {
    await entry();

    expect(texts(section("Muted servers")).map((row) => row.replace(/\s/g, ""))).toEqual([
      "spam.exampleUnmute",
      "ads.exampleUnmute",
    ]);
    for (const server of ["spam.example", "ads.example"])
      expect(undoOf(section("Muted servers"), server, "Unmute")).not.toBeNull();
    // Followed in the order baking, photography.
    expect(texts(section("Followed hashtags")).map((row) => row.replace(/\s/g, ""))).toEqual([
      "#photographyUnfollow",
      "#bakingUnfollow",
    ]);
    for (const hashtag of ["#photography", "#baking"])
      expect(undoOf(section("Followed hashtags"), hashtag, "Unfollow")).not.toBeNull();
  });

  test("with every list empty, each section says so and how something comes to be in it, with no count and no list", async () => {
    await entry({ seed: { people: serverOf({}).seed?.people ?? [] } });

    expect(lists.map(({ name }) => headed(name))).toEqual(lists.map(({ name }) => name));
    expect(screen.queryAllByRole("list")).toHaveLength(0);
    expect(screen.queryAllByRole("button", { name: /^(Unblock|Unmute|Unfollow)$/ })).toHaveLength(0);
    for (const [name, sentence] of [
      [
        "Blocked accounts",
        "You have not blocked anyone. To block an account, choose “Block…” in the menu of its profile or of one of its posts.",
      ],
      [
        "Muted accounts",
        "You have not muted anyone. To mute an account, choose “Mute” in the menu of its profile or of one of its posts.",
      ],
      [
        "Muted servers",
        "You have not muted any server. To mute one, choose “Mute server” in the menu of a profile or a post from it.",
      ],
      [
        "Followed hashtags",
        "You do not follow any hashtag. To follow one, open it and choose “Follow”: its posts then join your home timeline.",
      ],
    ] as const)
      expect(within(section(name)).getAllByText(sentence)).toHaveLength(1);
  });

  test("a list of seven rows shows whole, with no way to a longer one", async () => {
    await entry(serverOf({ blocked: 7, muted: 7, servers: 7, hashtags: 7 }));

    for (const { name, undo, all } of lists) {
      expect([name, headed(name)]).toEqual([name, `${name} 7`]);
      expect([name, rows(section(name)).length]).toEqual([name, 7]);
      expect(within(section(name)).getAllByRole("button", { name: undo })).toHaveLength(7);
      expect(screen.queryAllByRole("link", { name: all })).toHaveLength(0);
    }
  });

  test("a list of eight rows shows its first five, its count, and the way to the whole list", async () => {
    await entry(serverOf({ blocked: 8, muted: 8, servers: 8, hashtags: 8 }));

    for (const { list, name, undo, all } of lists) {
      expect([name, headed(name)]).toEqual([name, `${name} 8`]);
      expect([name, rows(section(name)).length]).toEqual([name, 5]);
      expect(within(section(name)).getAllByRole("button", { name: undo })).toHaveLength(5);
      // The way to the whole list is the section's own, after its rows.
      const door = within(section(name)).getByRole("link", { name: all });
      expect(door.getAttribute("href")).toBe(`/settings/relationships/${list}`);
      const last = rows(section(name)).at(-1);
      expect(last !== undefined && (last.compareDocumentPosition(door) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0).toBe(
        true,
      );
    }
    // The five are the list's first: the servers as the server lists them.
    expect(texts(section("Muted servers")).map((row) => row.replace(/\s*Unmute$/, ""))).toEqual(
      [1, 2, 3, 4, 5].map((n) => `server-0${String(n)}.example`),
    );
  });

  // The server gives the blocks and the mutes a page of twenty at a time: the entry reads the first, and does not
  // know how many there are in all.
  for (const [list, kind] of [
    ["blocked", "blocked"],
    ["muted", "muted"],
  ] as const)
    test(`a list of ${kind} accounts that goes on past its first page shows five rows, no count, and the way to the whole list`, async () => {
      const { requests } = await entry(serverOf({ [kind]: 22 }));
      const { name, all } = lists.find((l) => l.list === list) ?? lists[0];

      expect(headed(name)).toBe(name);
      expect(rows(section(name))).toHaveLength(5);
      expect(within(section(name)).getByRole("link", { name: all }).getAttribute("href")).toBe(
        `/settings/relationships/${list}`,
      );
      // One page was read, and no more.
      expect(requests.filter((r) => r.startsWith(list === "blocked" ? "GET /blocks" : "GET /mutes"))).toHaveLength(1);
    });

  test("the muted servers are read whole, page after page, so their count is known past a page", async () => {
    const { requests } = await entry(serverOf({ servers: 41 }));

    expect(requests.filter((r) => r.startsWith("GET /domain_mutes"))).toHaveLength(2);
    expect(headed("Muted servers")).toBe("Muted servers 41");
    expect(rows(section("Muted servers"))).toHaveLength(5);
  });

  test("in Japanese the sections, what a mute covers and the buttons read in it", async () => {
    const on = fakeData(some);
    await relationships(on, { locale: "ja" });
    await screen.findAllByRole("region");
    await read(on);

    expect(
      screen.getAllByRole("region").map((part) => plain(within(part).getAllByRole("heading")[0]?.textContent)),
    ).toEqual([
      "ブロックしたアカウント 2",
      "ミュートしたアカウント 1",
      "ミュートしたサーバー 2",
      "フォロー中のハッシュタグ 2",
    ]);
    expect(screen.getAllByRole("button", { name: "ブロックを解除" })).toHaveLength(2);
    expect(screen.getAllByRole("button", { name: "ミュートを解除" })).toHaveLength(3);
    expect(screen.getAllByRole("button", { name: "フォローを解除" })).toHaveLength(2);
    expect(screen.getAllByText("通知も非表示")).toHaveLength(1);
  });

  test("the way back, named Settings, is the owner's to follow", async () => {
    const { user, told } = await entry();

    await user.click(screen.getByRole("button", { name: "Settings" }));

    expect(told).toEqual(["back"]);
  });
});

/** What the section of `kind` says of its first row once it is undone: the row, its place, and the count. */
function expectUndone(kind: ListKind, before: readonly string[], count: string) {
  const part = section(kind.name);
  const [row, ...others] = rows(part);
  if (row === undefined) throw new Error("test: the list has no row");
  const { group, title, said: sentence } = first[kind.list];
  // The row is where it was, by its name, and in place of its button it says what was done.
  expect(rows(part)).toHaveLength(before.length);
  expect(plain(row.textContent)).toMatch(inOrder(...title, kind.done, sentence));
  expect(within(row).queryAllByRole("button")).toHaveLength(0);
  expect(within(part).queryAllByRole("group", { name: group })).toHaveLength(0);
  // A word for the eye, and the whole sentence for assistive technology.
  expect(isInaccessible(within(row).getByText(kind.done))).toBe(true);
  const spoken = within(row).getByText(
    (_, element) => plain(element?.textContent) === sentence && element?.children.length === 0,
  );
  expect(isInaccessible(spoken)).toBe(false);
  // The rows after it are as they were, each with its button.
  expect(others.map((other) => plain(other.textContent))).toEqual(before.slice(1));
  expect(within(part).queryAllByRole("button", { name: kind.undo })).toHaveLength(before.length - 1);
  // The count is the server's.
  expect(headed(kind.name)).toBe(count);
}

describe("undoing", () => {
  for (const kind of lists) {
    const { group, path, username, failed } = first[kind.list];

    test(`${kind.undo} of ${kind.name.toLowerCase()} acts at once with one request, and the row stays where it was, saying what was done, with the count one less`, async () => {
      const on = await entry();
      const part = () => section(kind.name);
      const before = texts(part());
      const count = Number(headed(kind.name).replace(`${kind.name} `, ""));
      const sent = on.fake.script(kind.route);
      const from = on.requests.length;

      await on.user.click(undoOf(part(), group, kind.undo));

      // No confirmation stands between the press and the request.
      await sent.served();
      expect(screen.queryAllByRole("alertdialog")).toHaveLength(0);
      await waitFor(() => {
        if (within(part()).queryAllByText(kind.done).length === 0) throw new Error("test: the row does not say it yet");
      });
      await read(on);
      expect(writes(on, from)).toEqual([path(username === undefined ? "" : on.fake.accountId(username))]);
      expectUndone(kind, before, count === 1 ? kind.name : `${kind.name} ${String(count - 1)}`);
      // The other sections are as they were.
      expect(said()).toEqual([]);
      for (const other of lists.filter((l) => l !== kind))
        expect(within(section(other.name)).queryAllByText(other.done)).toHaveLength(0);
    });

    test(`${kind.undo} of ${kind.name.toLowerCase()} that fails says so in the section's status, and leaves the row, its button and the count; the next press undoes it`, async () => {
      const on = await entry();
      const part = () => section(kind.name);
      const before = texts(part());
      const count = headed(kind.name);
      const sent = on.fake.script(kind.route);
      const failing = on.fake.script(kind.route, { times: 1, fail: 500 });

      await on.user.click(undoOf(part(), group, kind.undo));

      await waitFor(() => {
        expect(said(part())).toEqual([failed]);
      });
      await failing.arrived();
      expect(texts(part())).toEqual(before);
      expect(undoOf(part(), group, kind.undo).disabled).toBe(false);
      expect(headed(kind.name)).toBe(count);
      expect(within(part()).queryAllByText(kind.done)).toHaveLength(0);
      // Only this section speaks.
      expect(said()).toEqual([failed]);

      await on.user.click(undoOf(part(), group, kind.undo));

      await waitFor(() => {
        if (within(part()).queryAllByText(kind.done).length === 0) throw new Error("test: the row does not say it yet");
      });
      await read(on);
      expect(sent.seen).toHaveLength(2);
      expect(sent.seen[1]?.url.pathname).toBe(sent.seen[0]?.url.pathname);
      const left = Number(count.replace(`${kind.name} `, "")) - 1;
      expectUndone(kind, before, left === 0 ? kind.name : `${kind.name} ${String(left)}`);
    });
  }

  // The sentence of the undo that failed leaves the section's status with the next press on that row: the section
  // never says both that the row was undone and that undoing it did not finish.
  test("once the next press undid the row, the section no longer says that undoing it did not finish", async () => {
    const kind = lists[3];
    const on = await entry();
    const part = () => section(kind.name);
    on.fake.script(kind.route, { times: 1, fail: 500 });
    await on.user.click(undoOf(part(), first.hashtags.group, kind.undo));
    await waitFor(() => {
      expect(said(part())).toEqual([first.hashtags.failed]);
    });
    await on.user.click(undoOf(part(), first.hashtags.group, kind.undo));
    await waitFor(() => {
      if (within(part()).queryAllByText(kind.done).length === 0) throw new Error("test: the row does not say it yet");
    });
    await read(on);

    expect(said(part()).join()).toBe("");
  });

  test("each undo of a section stands in its own row, and a second one does not take the first one's away", async () => {
    const on = await entry();
    const part = () => section("Muted servers");

    await on.user.click(undoOf(part(), "spam.example", "Unmute"));
    await waitFor(() => {
      expect(within(part()).queryAllByText("Unmuted")).toHaveLength(1);
    });
    await on.user.click(undoOf(part(), "ads.example", "Unmute"));
    await waitFor(() => {
      expect(within(part()).queryAllByText("Unmuted")).toHaveLength(2);
    });
    await read(on);

    expect(texts(part()).map((row) => row.replace(/\s/g, ""))).toEqual([
      "spam.exampleUnmutedYouunmutedtheserverspam.example.",
      "ads.exampleUnmutedYouunmutedtheserverads.example.",
    ]);
    // Nothing is muted any more: the heading has no count, and the rows stay until the reader leaves.
    expect(headed("Muted servers")).toBe("Muted servers");
    expect(within(part()).queryAllByRole("button")).toHaveLength(0);
  });

  // Seven rows show whole, and eight as five: after an undo among eight the server lists seven, and the section stays
  // as the reader found it.
  test("an undo in a list of eight shown as five keeps the five rows, the undone one in its place saying what was done, with the count one less and the way to the whole list", async () => {
    const on = await entry(serverOf({ servers: 8 }));
    const part = () => section("Muted servers");
    expect(headed("Muted servers")).toBe("Muted servers 8");

    await on.user.click(undoOf(part(), "server-02.example", "Unmute"));

    await waitFor(() => {
      if (within(part()).queryAllByText("Unmuted").length === 0) throw new Error("test: the row does not say it yet");
    });
    await read(on);
    expect(texts(part()).map((row) => row.replace(/\s/g, ""))).toEqual([
      "server-01.exampleUnmute",
      "server-02.exampleUnmutedYouunmutedtheserverserver-02.example.",
      "server-03.exampleUnmute",
      "server-04.exampleUnmute",
      "server-05.exampleUnmute",
    ]);
    expect(headed("Muted servers")).toBe("Muted servers 7");
    expect(
      within(part())
        .queryAllByRole("link", { name: "See all muted servers" })
        .map((door) => door.getAttribute("href")),
    ).toEqual(["/settings/relationships/servers"]);
  });

  test("the row kept after an undo is gone when the screen is left and opened again", async () => {
    const on = await entry();
    await on.user.click(undoOf(section("Blocked accounts"), "@quickdeals", "Unblock"));
    await waitFor(() => {
      expect(within(section("Blocked accounts")).queryAllByText("Unblocked")).toHaveLength(1);
    });
    await read(on);
    expect(rows(section("Blocked accounts"))).toHaveLength(2);

    // The reader leaves, and comes back to the same app: its data is the same.
    cleanup();
    await relationships(on);
    await screen.findAllByRole("region");
    await read(on);

    expect(headed("Blocked accounts")).toBe("Blocked accounts 1");
    expect(texts(section("Blocked accounts"))).toHaveLength(1);
    expect(texts(section("Blocked accounts"))[0]).toMatch(inOrder("Sora そら", "@sora@social.example", "Unblock"));
    expect(screen.queryAllByText("Unblocked")).toHaveLength(0);
    // What the server lists now.
    const held = await on.data.queryClient.infiniteQuery({ ...blocksQuery(on.data), staleTime: Infinity });
    expect(held.pages.flatMap((page) => page.items.map((account) => account.username))).toEqual(["sora"]);
  });

  test("a list read again after an undo that then cannot be read says so in its section, with Try again, and the other sections stay", async () => {
    const on = await entry();
    const failing = on.fake.script("GET /blocks", { fail: 500 });

    await on.user.click(undoOf(section("Blocked accounts"), "@quickdeals", "Unblock"));

    const again = await within(section("Blocked accounts")).findByRole("button", { name: "Try again" });
    await read(on);
    expect(within(section("Blocked accounts")).getAllByText("This list could not be loaded.")).toHaveLength(1);
    expect(rows(section("Blocked accounts"))).toHaveLength(0);
    expect(rows(section("Muted servers"))).toHaveLength(2);
    expect(screen.queryAllByRole("alert")).toHaveLength(0);
    failing.end();

    await on.user.click(again);

    // The list is back: the row undone, kept until the reader leaves, and the one still blocked.
    await waitFor(() => {
      if (within(section("Blocked accounts")).queryAllByRole("button", { name: "Unblock" }).length !== 1)
        throw new Error("test: the list is not back yet");
    });
    expect(texts(section("Blocked accounts"))[0]).toMatch(
      inOrder("QuickDeals", "@quickdeals@demo.test", "Unblocked", "You unblocked @quickdeals."),
    );
    expect(rows(section("Blocked accounts"))).toHaveLength(2);
    expect(headed("Blocked accounts")).toBe("Blocked accounts 1");
    expect(within(section("Blocked accounts")).queryAllByText("This list could not be loaded.")).toHaveLength(0);
  });
});

describe("the whole list", () => {
  // The list's address, its title, and what its section's heading says of the server `some`.
  for (const [list, title, count] of [
    ["blocked", "Blocked accounts", "2 accounts"],
    ["muted", "Muted accounts", "1 account"],
    ["servers", "Muted servers", "2 servers"],
    ["hashtags", "Followed hashtags", "2 hashtags"],
  ] as const)
    test(`of the ${title.toLowerCase()} is titled by the list's name, reads that list alone, and heads its rows with how many there are`, async () => {
      const kind = lists.find((l) => l.list === list) ?? lists[0];
      const { requests } = await whole(list);

      expect(screen.getAllByRole("heading").map((heading) => plain(heading.textContent))).toEqual([title, count]);
      const part = screen.getByRole("region", { name: count });
      expect(rows(part)).toHaveLength(Number(count.split(" ")[0]));
      expect(within(part).getAllByRole("button", { name: kind.undo })).toHaveLength(rows(part).length);
      const read = { blocked: "/blocks", muted: "/mutes", servers: "/domain_mutes", hashtags: "/followed_tags" };
      expect(
        requests.filter((r) => /^GET \/(blocks|mutes|domain_mutes|followed_tags)/.test(r)).map((r) => r.split("?")[0]),
      ).toEqual([`GET ${read[list]}`]);
      // No way to itself.
      expect(screen.queryAllByRole("link", { name: kind.all })).toHaveLength(0);
    });

  test("shows every row of a list the entry cuts short", async () => {
    await whole("hashtags", serverOf({ hashtags: 9 }));

    const part = screen.getByRole("region", { name: "9 hashtags" });
    expect(rows(part)).toHaveLength(9);
    // Latest first.
    expect(plain(rows(part)[0]?.textContent).replace(/\s/g, "")).toBe("#tag09Unfollow");
  });

  for (const [list, kind, route] of [
    ["blocked", "blocked", "GET /blocks"],
    ["muted", "muted", "GET /mutes"],
  ] as const)
    test(`of the ${kind} accounts pages through the server's list: a page, that there are more, and Show more for the next`, async () => {
      const on = await whole(list, serverOf({ [kind]: 22 }));
      const cursors = () =>
        on.requests
          .filter((r) => r.startsWith(route))
          .map((r) => new URL(r.slice("GET ".length), "https://demo.test").searchParams.get("cursor"));
      expect(screen.getAllByRole("heading").map((heading) => plain(heading.textContent))[1]).toBe(
        "More than 20 accounts",
      );
      expect(screen.getAllByRole("listitem")).toHaveLength(20);
      expect(cursors()).toEqual([null]);

      await on.user.click(screen.getByRole("button", { name: "Show more" }));

      await waitFor(() => {
        expect(screen.getAllByRole("listitem")).toHaveLength(22);
      });
      await read(on);
      expect(screen.getAllByRole("heading").map((heading) => plain(heading.textContent))[1]).toBe("22 accounts");
      // The second page was asked for by the first one's cursor, and there is no third.
      expect(cursors()).toHaveLength(2);
      expect(cursors()[1]).not.toBeNull();
      expect(screen.queryAllByRole("button", { name: "Show more" })).toHaveLength(0);
    });

  test("Show more that fails keeps the rows and says once, above its button, that the rest could not be loaded; the button, now Try again, keeps the focus, and its next press reads the rest", async () => {
    const on = await whole("blocked", serverOf({ blocked: 22 }));
    const rest = "The rest of the list could not be loaded.";
    const asked = () => on.requests.filter((r) => r.startsWith("GET /blocks"));
    const failing = on.fake.script("GET /blocks", { fail: 500 });
    const more = screen.getByRole("button", { name: "Show more" });

    await on.user.click(more);

    const again = await screen.findByRole("button", { name: "Try again" });
    await read(on);
    expect(screen.getAllByRole("listitem")).toHaveLength(20);
    expect(screen.getAllByRole("heading").map((heading) => plain(heading.textContent))[1]).toBe(
      "More than 20 accounts",
    );
    const [line, ...others] = screen.getAllByText(rest);
    expect(others).toEqual([]);
    expect(line !== undefined && (line.compareDocumentPosition(again) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0).toBe(
      true,
    );
    // The same button under another name: the focus the press left on it stays.
    expect(again === more).toBe(true);
    expect(document.activeElement === again).toBe(true);
    expect(screen.queryAllByRole("button", { name: "Show more" })).toHaveLength(0);
    expect(asked()).toHaveLength(2);
    failing.end();

    await on.user.click(again);

    await waitFor(() => {
      expect(screen.getAllByRole("listitem")).toHaveLength(22);
    });
    await read(on);
    // The page that failed was asked for again, and no other.
    expect(asked()).toHaveLength(3);
    expect(asked()[2]).toBe(asked()[1] ?? "");
    expect(screen.getAllByRole("heading").map((heading) => plain(heading.textContent))[1]).toBe("22 accounts");
    expect(screen.queryAllByText(rest)).toHaveLength(0);
    expect(screen.queryAllByRole("button", { name: /^(Show more|Try again)$/ })).toHaveLength(0);
  });

  test("of the muted servers reads the list whole, with nothing left to show more of", async () => {
    const { requests } = await whole("servers", serverOf({ servers: 41 }));

    expect(requests.filter((r) => r.startsWith("GET /domain_mutes"))).toHaveLength(2);
    expect(rows(screen.getByRole("region", { name: "41 servers" }))).toHaveLength(41);
    expect(screen.queryAllByRole("button", { name: "Show more" })).toHaveLength(0);
  });

  test("an undo there leaves the row where it was, and the heading counts what the server holds", async () => {
    const on = await whole("servers");
    const from = on.requests.length;

    await on.user.click(
      within(screen.getByRole("group", { name: "spam.example" })).getByRole("button", { name: "Unmute" }),
    );

    const part = await screen.findByRole("region", { name: "1 server" });
    await read(on);
    expect(writes(on, from)).toEqual(["DELETE /domain_mutes/spam.example"]);
    expect(texts(part).map((row) => row.replace(/\s/g, ""))).toEqual([
      "spam.exampleUnmutedYouunmutedtheserverspam.example.",
      "ads.exampleUnmute",
    ]);
  });

  test("says that it loads the list under the list's name, then that it could not be read, with Try again, which reads it", async () => {
    const on = fakeData(some);
    const reading = on.fake.script("GET /mutes", { times: 1, hold: "request", fail: 500 });
    const { user } = await relationships(on, { list: "muted" });
    await reading.arrived();

    // The heading is the list's name while nothing is known of it.
    expect(screen.getAllByRole("heading").map((heading) => plain(heading.textContent))).toEqual([
      "Muted accounts",
      "Muted accounts",
    ]);
    expect(screen.getAllByText("Loading this list…")).toHaveLength(1);
    expect(screen.queryAllByRole("listitem")).toHaveLength(0);

    reading.release();
    const again = await screen.findByRole("button", { name: "Try again" });
    expect(screen.getAllByText("This list could not be loaded.")).toHaveLength(1);
    expect(screen.getAllByRole("heading").map((heading) => plain(heading.textContent))).toEqual([
      "Muted accounts",
      "Muted accounts",
    ]);

    await user.click(again);

    await screen.findByRole("region", { name: "1 account" });
    expect(reading.seen).toHaveLength(2);
    expect(screen.queryAllByText("This list could not be loaded.")).toHaveLength(0);
  });

  test("the way back, named Blocks, mutes, and hashtags, is the owner's to follow", async () => {
    const { user, told } = await whole("servers");

    await user.click(screen.getByRole("button", { name: "Blocks, mutes, and hashtags" }));

    expect(told).toEqual(["back"]);
  });

  test("a list the settings do not have says that the page does not exist, under the entry's name, and leads to the entry", async () => {
    const on = fakeData(some);
    await relationships(on, { list: "nothing" });

    const missing = await screen.findByRole("alert");
    expect(plain(missing.textContent)).toMatch(
      inOrder("This settings page does not exist. Check the address.", "Blocks, mutes, and hashtags"),
    );
    expect(within(missing).getByRole("link", { name: "Blocks, mutes, and hashtags" }).getAttribute("href")).toBe(
      "/settings/relationships",
    );
    expect(screen.getAllByRole("heading").map((heading) => plain(heading.textContent))).toEqual([
      "Blocks, mutes, and hashtags",
    ]);
    // No list was asked for.
    expect(on.requests.filter((r) => /^GET \/(blocks|mutes|domain_mutes|followed_tags)/.test(r))).toEqual([]);
  });
});

describe("before the lists are read", () => {
  for (const route of ["GET /blocks", "GET /mutes", "GET /domain_mutes", "GET /followed_tags"] as const)
    test(`the entry says that it loads the settings, with no section, until ${route} answers`, async () => {
      const on = fakeData(some);
      const reading = on.fake.script(route, { hold: "request" });
      await relationships(on);
      await reading.arrived();

      expect(said()).toEqual(["Loading your settings…"]);
      expect(screen.queryAllByRole("region")).toHaveLength(0);
      expect(screen.queryAllByRole("button", { name: /^(Unblock|Unmute|Unfollow)$/ })).toHaveLength(0);

      reading.release();
      await screen.findAllByRole("region");
      await read(on);
      // The notice is gone, and no section has anything to say.
      expect(said()).toEqual([]);
      expect(screen.getAllByRole("region")).toHaveLength(4);
    });

  test("when the lists cannot be read the entry says that the settings could not be loaded, and Try again, inside the alert, reads them", async () => {
    const on = fakeData(some);
    const routes = ["GET /blocks", "GET /mutes", "GET /domain_mutes", "GET /followed_tags"] as const;
    const failing = routes.map((route) => on.fake.script(route, { fail: 500 }));
    const { user } = await relationships(on);

    const failed = await screen.findByRole("alert");
    expect(plain(failed.textContent)).toMatch(inOrder("Your settings could not be loaded.", "Try again"));
    expect(screen.queryAllByRole("region")).toHaveLength(0);
    for (const script of failing) script.end();

    await user.click(within(failed).getByRole("button", { name: "Try again" }));

    await screen.findAllByRole("region");
    await read(on);
    expect(screen.queryAllByRole("alert")).toHaveLength(0);
    expect(
      screen.getAllByRole("region").map((part) => plain(within(part).getAllByRole("heading")[0]?.textContent)),
    ).toEqual(["Blocked accounts 2", "Muted accounts 1", "Muted servers 2", "Followed hashtags 2"]);
  });

  // While one list keeps failing and the others answer, the entry shows its sections, and that list's section says
  // that it could not be loaded.
  test("when one list alone cannot be read, its section says so with Try again, and the other sections show", async () => {
    const on = fakeData(some);
    const failing = on.fake.script("GET /mutes", { fail: 500 });
    await relationships(on);
    await failing.arrived();
    // Time for the other lists to come and for the screen to draw them: twenty turns of the event loop.
    for (let turn = 0; turn < 20; turn += 1) await new Promise((resolve) => setTimeout(resolve, 0));

    expect(screen.queryAllByText("This list could not be loaded.")).toHaveLength(1);
  });
});
