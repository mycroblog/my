// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { scenario } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { renderApp, type AppOptions, type TestApp } from "./test/app";
import { fakeApp } from "./test/server";

test("a person's notifications show the follows, replies, favourites and boosts of others", async () => {
  const app = await renderApp("/notifications", { viewer: "sam" });

  expect(app.title()).toBe("Notifications");
  const main = screen.getByRole("main");
  expect(within(main).getByRole("heading", { level: 1, name: "Notifications" })).toBeDefined();
  const feed = within(main).getByRole("feed", { name: "Notifications" });
  expect(within(feed).getByRole("article", { name: "Yuki 雪 followed you" })).toBeDefined();
  // A mention is the post that mentions the reader, named by its author.
  const mention = within(feed)
    .getAllByRole("article", { name: "Yuki 雪" })
    .filter((post) => post.textContent.includes("北側の尾根道です"));
  expect(mention).toHaveLength(1);
  expect(mention[0]?.textContent).toContain("Yuki 雪 mentioned you");
  expect(within(feed).getByRole("article", { name: "Yuki 雪 favourited your post" }).textContent).toContain(
    "Day 3 of the sourdough",
  );
  expect(within(feed).getByRole("article", { name: "Hana 花 boosted your post" }).textContent).toContain(
    "Day 3 of the sourdough",
  );
});

test("a notification's line leads to what it tells of, where the screen's heading takes the focus, and Back returns", async () => {
  const app = await renderApp("/notifications", { viewer: "sam" });
  const feed = within(screen.getByRole("main")).getByRole("feed", { name: "Notifications" });

  await app.user.click(within(feed).getByRole("link", { name: "Yuki 雪 followed you" }));

  const heading = await within(screen.getByRole("main")).findByRole("heading", { level: 1, name: /^Yuki 雪/ });
  expect(app.path()).toBe("/@yuki");
  expect(app.title()).toBe(heading.textContent);
  expect(app.focused()).toBe(heading);

  await app.back();
  expect(app.path()).toBe("/notifications");
  expect(app.title()).toBe("Notifications");
});

test("a visitor at the notifications is asked to sign in, in their language", async () => {
  await renderApp("/notifications", { visitor: true, locale: "ja" });

  const main = screen.getByRole("main");
  expect(within(main).getByRole("heading", { level: 1, name: "通知" })).toBeDefined();
  expect(within(main).getByRole("link", { name: "ログイン" }).getAttribute("href")).toBe("/login");
});

// A notification the reader acts on becomes read. The scenario's notifications, newest first: Hana reacted to, boosted
// and favourited sam's post, Yuki favourited it, Yuki mentioned sam in three posts, then Marco, Layla, Yuki and Hana
// followed sam. Unless a test says otherwise, the reader has not read the three newest.

const everything = { unreadNotifications: 11 } as const;

/** The notifications of sam, with the marker's and the dismissal's requests scripted to be counted. */
async function inbox(options: AppOptions = {}) {
  const app = await renderApp("/notifications", { viewer: "sam", ...options });
  const marker = app.fake.script("PUT /notifications/marker");
  const dismissed = app.fake.script("POST /notifications/{id}/dismiss");
  const feed = () => within(screen.getByRole("main")).getByRole("feed", { name: "Notifications" });
  const { data: page } = await app.data.api.GET("/notifications");
  /** The ID of the `n`th notification, 0 the newest. */
  const id = (n: number) => page?.items[n]?.id ?? `no notification ${String(n)}`;
  return { app, marker, dismissed, feed, id };
}

/** The rows of the scenario's notifications that carry something to press: each is found in the feed it is in. */
const rows = {
  boost: (feed: HTMLElement) => within(feed).getByRole("article", { name: "Hana 花 boosted your post" }),
  follow: (feed: HTMLElement) => within(feed).getByRole("article", { name: "Marco followed you" }),
  /** Yuki's mention of sam: a post with its reply, boost, favourite, react and more. */
  mention: (feed: HTMLElement) => {
    const row = within(feed)
      .getAllByRole("article")
      .find((article) => article.textContent.includes("北側の尾根道です"));
    if (row === undefined) throw new Error("test: no notification of the mention");
    return row;
  },
};

const unreadMark = (row: HTMLElement) => within(row).queryByRole("img", { name: "Unread" });

/** How many the navigation says are unread: its Notifications link, which the rail and the tab bar both render. */
const unreadCount = () => {
  const [link] = screen.getAllByRole("link", { name: /^Notifications/ });
  return Number(/(\d+) unread/.exec(link?.textContent ?? "")?.[1] ?? "0");
};

/** Waits until `row` shows no Unread mark and the navigation counts `unread`. */
const becomesRead = (row: () => HTMLElement, unread: number) =>
  waitFor(() => {
    if (unreadMark(row()) !== null) throw new Error("test: the row still says Unread");
    if (unreadCount() !== unread) throw new Error(`test: the navigation counts ${String(unreadCount())} unread`);
  });

/** Sees that `row` is unread still, and the navigation counts `unread`. */
const staysUnread = (row: HTMLElement, unread: number) => {
  expect(unreadMark(row)).not.toBeNull();
  expect(unreadCount()).toBe(unread);
};

/** The focus on `element`, as a reader's Tab or a script's gives it. */
const focus = (element: HTMLElement) => {
  element.focus();
  expect(document.activeElement).toBe(element);
};

test("the scenario's notifications are unread, each marked, and the navigation counts them", async () => {
  const { feed } = await inbox(everything);
  expect(within(feed()).getAllByRole("img", { name: "Unread" })).toHaveLength(11);
  expect(unreadCount()).toBe(11);
});

test("a click on the words of a notification reads it", async () => {
  const { app, feed } = await inbox(everything);
  await app.user.click(within(rows.boost(feed())).getByText(/^Day 3 of the sourdough/));
  await becomesRead(() => rows.boost(feed()), 10);
});

// A button of the row, pressed by a pointer, by Enter and by Space.

type Press = (app: TestApp) => Promise<void>;
const presses: [string, Press][] = [
  ["a pointer", (app) => app.user.click(document.activeElement as HTMLElement)],
  ["Enter", (app) => app.user.keyboard("{Enter}")],
  ["Space", (app) => app.user.keyboard(" ")],
];

const buttons: [string, keyof typeof rows, string | RegExp][] = [
  ["Follow back", "follow", "Follow back"],
  ["Favourite", "mention", "Favourite"],
  ["Reply", "mention", /^Reply/],
];

for (const [how, press] of presses) {
  for (const [title, row, name] of buttons) {
    test(`the ${title} button of a notification, pressed by ${how}, reads it`, async () => {
      const { app, feed } = await inbox(everything);
      focus(within(rows[row](feed())).getByRole("button", { name }));
      await press(app);
      await becomesRead(() => rows[row](feed()), 10);
    });
  }

  test(`the Dismiss button of an unread notification, pressed by ${how}, dismisses it and the count drops by one`, async () => {
    const { app, feed, dismissed, id } = await inbox(everything);
    focus(within(rows.boost(feed())).getByRole("button", { name: "Dismiss notification" }));
    await press(app);
    await waitFor(() => {
      expect(unreadCount()).toBe(10);
    });
    expect(dismissed.seen.map((s) => s.url.pathname)).toEqual([`/api/v1/notifications/${id(1)}/dismiss`]);
    expect(within(feed()).queryByRole("article", { name: "Hana 花 boosted your post" })).toBeNull();
  });
}

test("the Show button of a notification of a post with a content warning reads it", async () => {
  const { app, feed } = await inbox({
    seed: {
      people: scenario.people,
      posts: [
        {
          key: "cw",
          author: "yuki",
          text: "@sam The ending, in full.",
          spoiler: "Spoilers",
          visibility: "public",
          language: "en",
        },
      ],
    },
    unreadNotifications: 1,
  });
  const row = () => within(feed()).getByRole("article");
  await app.user.click(within(row()).getByRole("button", { name: /^Show/ }));
  await becomesRead(row, 0);
});

// A link of the row, followed by a pointer and by Enter: the app leaves the list, and the row is read when it is back.

const links: [string, keyof typeof rows, string | RegExp][] = [
  ["its line", "boost", "Hana 花 boosted your post"],
  ["the author's name", "mention", "Yuki 雪"],
  ["a hashtag in its post", "boost", /baking$/],
];

for (const [how, press] of presses.slice(0, 2)) {
  for (const [title, row, name] of links) {
    test(`following ${title} of a notification by ${how} reads it`, async () => {
      const { app, feed } = await inbox(everything);
      focus(within(rows[row](feed())).getByRole("link", { name }));
      await press(app);
      await waitFor(() => {
        expect(app.path()).not.toBe("/notifications");
      });
      expect(unreadCount()).toBe(10);
      await app.back();
      await becomesRead(() => rows[row](feed()), 10);
    });
  }
}

// An item chosen in a menu the row opened.

const menus: [string, string | RegExp, string][] = [
  ["More", "More", "Bookmark"],
  ["Boost", "Boost", "Boost"],
];

for (const [title, opener, item] of menus) {
  test(`the ${item} item of the ${title} menu of a notification, chosen by a pointer, reads it`, async () => {
    const { app, feed } = await inbox(everything);
    await app.user.click(within(rows.mention(feed())).getByRole("button", { name: opener }));
    await app.user.click(await screen.findByRole("menuitem", { name: item }));
    await becomesRead(() => rows.mention(feed()), 10);
  });

  test(`the ${item} item of the ${title} menu of a notification, chosen by Enter, reads it`, async () => {
    const { app, feed } = await inbox(everything);
    await app.user.click(within(rows.mention(feed())).getByRole("button", { name: opener }));
    focus(await screen.findByRole("menuitem", { name: item }));
    await app.user.keyboard("{Enter}");
    await becomesRead(() => rows.mention(feed()), 10);
  });
}

// A shortcut's key on the focused post that asks something of it.

for (const [key, asks] of [
  ["r", "reply"],
  ["b", "boost"],
  ["f", "favourite"],
] as const) {
  test(`the key ${key} on the focused notification asks to ${asks}, and reads it`, async () => {
    const { app, feed } = await inbox(everything);
    focus(rows.mention(feed()));
    await app.user.keyboard(key);
    await becomesRead(() => rows.mention(feed()), 10);
  });
}

// A control that opens a menu or the emoji picker reads it: opening alone counts.

const opens: [string, (app: TestApp, row: HTMLElement) => Promise<void>][] = [
  ["a press on More", (app, row) => app.user.click(within(row).getByRole("button", { name: "More" }))],
  [
    "Enter on More",
    async (app, row) => {
      focus(within(row).getByRole("button", { name: "More" }));
      await app.user.keyboard("{Enter}");
    },
  ],
  ["a press on Boost", (app, row) => app.user.click(within(row).getByRole("button", { name: "Boost" }))],
  ["a press on React", (app, row) => app.user.click(within(row).getByRole("button", { name: "React" }))],
  [
    "Enter on React",
    async (app, row) => {
      focus(within(row).getByRole("button", { name: "React" }));
      await app.user.keyboard("{Enter}");
    },
  ],
];

for (const [opening, open] of opens) {
  test(`opening a menu or the picker by ${opening} reads the notification`, async () => {
    const { app, feed } = await inbox(everything);
    await open(app, rows.mention(feed()));
    await becomesRead(() => rows.mention(feed()), 10);
  });
}

// What does not read a notification: nothing was asked of it.

test("focusing a notification, and Tab from it, do not read it", async () => {
  const { app, feed, marker } = await inbox(everything);
  const row = rows.mention(feed());
  focus(row);
  staysUnread(row, 11);
  await app.user.tab();
  expect(document.activeElement).not.toBe(row);
  staysUnread(row, 11);
  expect(marker.seen).toHaveLength(0);
});

test("Space on the notification itself does not read it", async () => {
  const { app, feed, marker } = await inbox(everything);
  const row = rows.mention(feed());
  focus(row);
  await app.user.keyboard(" ");
  staysUnread(row, 11);
  expect(marker.seen).toHaveLength(0);
});

test("moving to the next and the previous notification does not read either", async () => {
  const { app, feed, marker } = await inbox(everything);
  const row = rows.mention(feed());
  focus(row);
  await app.user.keyboard("j");
  expect(document.activeElement).not.toBe(row);
  const next = document.activeElement as HTMLElement;
  await app.user.keyboard("k");
  expect(document.activeElement).toBe(row);
  staysUnread(row, 11);
  expect(unreadMark(next)).not.toBeNull();
  expect(marker.seen).toHaveLength(0);
});

// The marker the server keeps.

test("reading the oldest unread notification moves the marker to it", async () => {
  const { app, feed, marker, id } = await inbox();
  const favourite = within(feed()).getByRole("article", { name: "Hana 花 favourited your post" });
  await app.user.click(within(favourite).getByText(/^Day 3 of the sourdough/));
  await waitFor(() => {
    expect(marker.seen.map((s) => s.body.last_read_id)).toEqual([id(2)]);
  });
});

test("reading newer notifications while an older one is unread sends nothing, and reading the oldest moves the marker to the newest", async () => {
  const { app, feed, marker, id } = await inbox();
  for (const name of ["Hana 花 reacted to your post", "Hana 花 boosted your post"]) {
    await app.user.click(within(within(feed()).getByRole("article", { name })).getByText(/^Day 3 of the sourdough/));
  }
  await waitFor(() => {
    expect(unreadCount()).toBe(1);
  });
  expect(marker.seen).toHaveLength(0);
  const oldest = within(feed()).getByRole("article", { name: "Hana 花 favourited your post" });
  await app.user.click(within(oldest).getByText(/^Day 3 of the sourdough/));
  await waitFor(() => {
    expect(marker.seen.map((s) => s.body.last_read_id)).toEqual([id(0)]);
  });
  expect(unreadCount()).toBe(0);
});

// The notification a push or an email opens.

// Defect: the app lands on the post, but the notification stays unread and no marker is sent.
test("the address of a notification a push opens reads it, and the reader goes on to what it tells of", async () => {
  const on = fakeApp({ viewer: "sam" });
  const marker = on.fake.script("PUT /notifications/marker");
  const { data: page } = await on.data.api.GET("/notifications");
  // The oldest of the three unread: the favourite of Hana.
  const oldest = page?.items[2]?.id ?? "none";
  const app = await renderApp(`/notifications/${oldest}`, { on });
  await waitFor(() => {
    if (!/^\/@sam\/status\/[^/]+$/.test(app.path())) throw new Error(`test: the app is at ${app.path()}`);
  });
  await waitFor(() => {
    expect(unreadCount()).toBe(2);
  });
  expect(marker.seen.map((s) => s.body.last_read_id)).toEqual([oldest]);
});
