// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { screen, waitFor, within } from "@testing-library/react";
import { renderApp, type AppOptions, type TestApp } from "./test/app";
import { fakeApp } from "./test/server";

test("the address /notifications opens the screen of the reader's notifications, under its title", async () => {
  const app = await renderApp("/notifications", { viewer: "sam" });

  expect(app.title()).toBe("Notifications");
  const main = screen.getByRole("main");
  expect(within(main).getByRole("heading", { level: 1, name: "Notifications" })).toBeDefined();
  expect(within(main).getByRole("feed", { name: "Notifications" })).toBeDefined();
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

test("a visitor at the address a push opens is asked to sign in, and the link comes back to that address", async () => {
  const path = "/notifications/01890a5d-ac96-774b-bcce-b302099a8057";
  await renderApp(path, { visitor: true });

  const link = await within(screen.getByRole("main")).findByRole("link", { name: "Sign in" });
  const href = new URL(link.getAttribute("href") ?? "", "http://app.test");
  expect(href.pathname).toBe("/login");
  expect(href.searchParams.get("return_to")).toBe(path);
});

test("a visitor at the notifications is asked to sign in, in their language", async () => {
  await renderApp("/notifications", { visitor: true, locale: "ja" });

  const main = screen.getByRole("main");
  expect(within(main).getByRole("heading", { level: 1, name: "通知" })).toBeDefined();
  expect(within(main).getByRole("link", { name: "ログイン" }).getAttribute("href")).toBe("/login");
});

// A notification the reader acts on becomes read, which the feature's tests hold (web/app-features/src/notifications/
// notifications.dom.test.tsx); here, what the app around the list adds: the navigation's count, the shortcuts' keys,
// and a hashtag's link, which the app opens. The scenario's notifications, newest first: Hana reacted to, boosted
// and favourited sam's post, Yuki favourited it, Yuki mentioned sam in three posts, then Marco, Layla, Yuki and Hana
// followed sam. Unless a test says otherwise, the reader has not read the three newest.

const everything = { unreadNotifications: 11 } as const;

/** The notifications of sam, with the marker's requests scripted to be counted. */
async function inbox(options: AppOptions = {}) {
  const app = await renderApp("/notifications", { viewer: "sam", ...options });
  const marker = app.fake.script("PUT /notifications/marker");
  const feed = () => within(screen.getByRole("main")).getByRole("feed", { name: "Notifications" });
  return { app, marker, feed };
}

/** The rows of the scenario's notifications that carry something to press: each is found in the feed it is in. */
const rows = {
  follow: (feed: HTMLElement) => within(feed).getByRole("article", { name: "Marco followed you" }),
  boost: (feed: HTMLElement) => within(feed).getByRole("article", { name: "Hana 花 boosted your post" }),
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

test("the navigation counts the unread notifications, and a click on the words of one lowers the count", async () => {
  const { app, feed } = await inbox(everything);
  expect(unreadCount()).toBe(11);

  await app.user.click(within(rows.boost(feed())).getByText(/^Day 3 of the sourdough/));
  await becomesRead(() => rows.boost(feed()), 10);
});

// A link of the row, followed by a pointer and by Enter: the app leaves the list, and the row is read when it is back.

const presses: [string, (app: TestApp) => Promise<void>][] = [
  ["a pointer", (app) => app.user.click(document.activeElement as HTMLElement)],
  ["Enter", (app) => app.user.keyboard("{Enter}")],
];

for (const [how, press] of presses) {
  test(`following a hashtag in the post of a notification by ${how} reads it`, async () => {
    const { app, feed } = await inbox(everything);
    focus(within(rows.boost(feed())).getByRole("link", { name: /baking$/ }));
    await press(app);
    await waitFor(() => {
      expect(app.path()).not.toBe("/notifications");
    });
    expect(unreadCount()).toBe(10);
    await app.back();
    await becomesRead(() => rows.boost(feed()), 10);
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

test("the key m on the focused notification opens its More menu, and reads it", async () => {
  const { app, feed } = await inbox(everything);
  focus(rows.mention(feed()));

  await app.user.keyboard("m");

  expect(await screen.findByRole("menu", { name: "More" })).toBeDefined();
  await becomesRead(() => rows.mention(feed()), 10);
});

// The keys of what is not the notification's own do not read it: a new post (n) and the help (?), also on a
// notification drawn as one line, which declares no shortcut of its own.

for (const [key, opens, dialog] of [
  ["n", "the composer", "New post"],
  ["?", "the shortcuts' help", /shortcuts/i],
] as const) {
  for (const row of ["mention", "follow"] as const) {
    test(`the key ${key} on the focused ${row === "mention" ? "mention" : "follow"} opens ${opens}, and does not read it`, async () => {
      const { app, feed, marker } = await inbox(everything);
      const focused = rows[row](feed());
      focus(focused);

      await app.user.keyboard(key);

      expect(await screen.findByRole("dialog", { name: dialog })).toBeDefined();
      staysUnread(focused, 11);
      expect(marker.seen).toHaveLength(0);
    });
  }
}

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

test("a dismissal the server fails brings the notification back with a notice, whose Try again dismisses it for good", async () => {
  const { app, feed } = await inbox({ dismissFails: true });
  const dismissed = app.fake.script("POST /notifications/{id}/dismiss");
  const name = "Marco followed you";
  const shown = () => within(feed()).queryAllByRole("article", { name });

  await app.user.click(within(rows.follow(feed())).getByRole("button", { name: "Dismiss notification" }));

  // The notice is the page's, under the screen.
  const notice = await waitFor(() => {
    const said = screen
      .getAllByRole("status")
      .find((status) => status.textContent.startsWith("The notification could not be dismissed."));
    if (said === undefined) throw new Error("test: no notice says that the dismissal failed");
    return said;
  });
  await waitFor(() => {
    if (shown().length !== 1) throw new Error("test: the notification is not back in the list");
  });
  expect(dismissed.seen).toHaveLength(1);

  await app.user.click(within(notice).getByRole("button", { name: "Try again" }));
  await waitFor(() => {
    if (shown().length !== 0) throw new Error("test: the notification is still in the list");
  });
  expect(dismissed.seen).toHaveLength(2);
  // Kept by the server: the list read again leaves it out.
  const { data: page } = await app.data.api.GET("/notifications");
  expect(page?.items.filter((item) => item.type === "follow" && item.account?.username === "marco")).toHaveLength(0);
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
