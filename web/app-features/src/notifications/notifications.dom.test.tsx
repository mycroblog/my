// SPDX-License-Identifier: AGPL-3.0-only

// The notifications as a feature: what its loader reads, what each notification says and whether it is unread, the
// rows over the list, what a press on a mention's post asks of the server or of the screen, and when a notification
// becomes read.

import { expect, test } from "bun:test";
import { NavigationProvider } from "@my/design-system";
import { fakeHost, scenario, type FakeOptions, type FakeSeed } from "@my/e2e/fake";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import { PostActions, type ComposeRequest } from "../post-actions";
import { fakeData, renderFeature } from "../test/feature";
import { loadNotifications, Notifications } from "./notifications";

type Post = FakeSeed["posts"][number];

/** marco's post that mentions the reader, with `more`. */
const mention = (more: Partial<Post> = {}): Post => ({
  key: "m",
  author: "marco",
  text: "@ren Are you coming on Saturday?",
  visibility: "public",
  language: "en",
  ...more,
});

/**
 * A server as ren reads it, whose follows need their approval: sam asked to follow them, marco mentioned them, and
 * hana favourited their post, the newest; they have not read the last two. The server's policy holds back three
 * notifications of two people, and the list ends with a notice of the moderators and a notification of a kind a
 * newer server sends, which this client does not know.
 */
const answers = (posts: Post[] = [mention()]): FakeOptions => ({
  viewer: "ren",
  seed: {
    people: scenario.people,
    follows: [{ follower: "sam", followee: "ren" }],
    posts: [{ key: "y", author: "ren", text: "Bread tomorrow.", visibility: "public", language: "en" }, ...posts],
    interactions: [{ kind: "favourite", by: "hana", post: "y" }],
  },
  held: true,
  unreadNotifications: 2,
});

function server(options = answers()) {
  const made = fakeData(options);
  made.fake.script("GET /notifications", {
    change: (page) => ({
      ...page,
      items: [
        ...page.items,
        {
          id: "00000000-0000-7000-8000-000000000002",
          type: "moderation_notice" as const,
          created_at: "2026-11-13T01:00:00Z",
        },
        {
          id: "00000000-0000-7000-8000-000000000001",
          // No kind of the specification: one a server newer than this client sends.
          type: "some_new_kind" as never,
          created_at: "2026-11-12T01:00:00Z",
        },
      ],
    }),
  });
  return made;
}

/** The notifications of `made`, loaded and alone in the document, for a reader of `locale`: the reader's hands. */
async function shown(made = server(), locale = "en", onCompose: (to: ComposeRequest) => void = () => undefined) {
  await loadNotifications(made.data);
  cleanup();
  return renderFeature(
    made.data,
    <PostActions onCompose={onCompose}>
      <Notifications instance="my.example" />
    </PostActions>,
    locale,
  );
}

const feed = () => screen.getByRole("feed", { name: "Notifications" });
/** The notifications of the list, each by its words. */
const entries = () => within(feed()).getAllByRole("article");
const entry = (words: RegExp) => {
  const found = entries().filter((article) => words.test(article.textContent));
  if (found.length !== 1) throw new Error(`test: ${String(found.length)} notifications say ${String(words)}`);
  return found[0] as HTMLElement;
};

/** The buttons of a notification, by their names. */
const buttons = (article: HTMLElement) =>
  within(article)
    .queryAllByRole("button")
    .map((button) => button.getAttribute("aria-label") ?? button.textContent);

test("the loader reads the notifications, the read marker, the count of the requests, what waits for approval and the follow requests, then what the prompt of Web Push reads", async () => {
  const { data, requests } = server();

  await loadNotifications(data);

  expect(requests.map((r) => r.split("?")[0])).toEqual([
    "GET /notifications",
    "GET /notifications/marker",
    "GET /notifications/requests/summary",
    "GET /interaction_requests",
    "GET /follow_requests",
    "GET /push/subscription",
    "GET /preferences/notifications",
  ]);
});

test("each notification newer than the marker carries its own unread mark, the others none, newest first", async () => {
  await shown();
  const favourite = entry(/favourited your post/);
  const mentioned = entry(/mentioned you/);
  const requested = entry(/requested to follow you/);

  expect(entries().slice(0, 3)).toEqual([favourite, mentioned, requested]);
  expect(within(favourite).getAllByRole("img", { name: "Unread" })).toHaveLength(1);
  expect(within(mentioned).getAllByRole("img", { name: "Unread" })).toHaveLength(1);
  expect(within(requested).queryAllByRole("img", { name: "Unread" })).toHaveLength(0);
  expect(within(feed()).getAllByRole("img", { name: "Unread" })).toHaveLength(2);
});

test("a sentence names the account, isolated, in the server's words, and a kind the client does not know says there is one", async () => {
  await shown();
  const sentence = within(entry(/mentioned you/)).getByText(/mentioned you$/);

  expect(sentence.textContent).toBe("Marco mentioned you");
  expect(within(sentence).getByText("Marco").tagName).toBe("BDI");
  expect(within(feed()).getByText(/The moderators of my\.example sent you a notice/)).not.toBeNull();
  expect(within(feed()).getByText(/You have a new notification on my\.example/)).not.toBeNull();

  await shown(server(), "ja");
  expect(screen.getByText(/さんがあなたをメンションしました$/)).not.toBeNull();
});

test("over the list, the tabs, mark-all-read, and the row of the requests", async () => {
  await shown();
  const tabs = screen.getByRole("tablist", { name: "Show" });
  const held = screen.getByRole("link", { name: /^3 held-back notifications · from 2 people/ });

  expect(
    within(tabs)
      .getAllByRole("tab")
      .map((tab) => tab.textContent),
  ).toEqual(["All", "Mentions"]);
  expect(screen.getByRole("button", { name: "Mark all as read" })).not.toBeNull();
  expect(held.getAttribute("href")).toBe("/notifications/requests");
  expect(held.compareDocumentPosition(feed()) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0);
});

test("a row over the list counts the follow requests and opens their screen; a request's notification has no buttons to answer it", async () => {
  await shown();
  const row = screen.getByRole("link", { name: "1 follow request" });

  expect(row.getAttribute("href")).toBe("/notifications/follow_requests");
  expect(row.compareDocumentPosition(feed()) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0);
  expect(buttons(entry(/requested to follow you/))).toEqual([
    "More actions for this notification",
    "Dismiss notification",
  ]);
});

test("over the list, a row counts what waits for the reader's approval; a request's notification has no buttons to answer it", async () => {
  // A post of yuki's waits for their answers to marco's reply and boost and mio's quote (the fake's `approvals`).
  const made = fakeData({ approvals: true });
  await shown(made);
  const row = screen.getByText("3 waiting for your approval");
  const asked = entry(/asked to reply to your post/);

  expect(row.compareDocumentPosition(asked) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0);
  expect(buttons(asked)).toEqual(["More actions for this notification", "Dismiss notification"]);

  await shown(made, "ja");
  expect(screen.getByText("承認待ち 3件")).not.toBeNull();
});

test("the row of what waits leads to its screen, and stays over the Mentions tab; no request to reply, boost or quote has buttons to answer it", async () => {
  const user = await shown(fakeData({ approvals: true }));
  const row = () => screen.getByRole("link", { name: "3 waiting for your approval" });
  const requests = () =>
    within(feed()).queryAllByRole("article", { name: /asked to (reply to|boost|quote) your post$/ });

  expect(row().getAttribute("href")).toBe("/notifications/waiting");
  expect(requests().map(buttons)).toEqual([
    ["More actions for this notification", "Dismiss notification"],
    ["More actions for this notification", "Dismiss notification"],
    ["More actions for this notification", "Dismiss notification"],
  ]);

  await user.click(screen.getByRole("tab", { name: "Mentions" }));
  await waitFor(() => {
    if (requests().length > 0) throw new Error("the list still shows every notification");
  });
  expect(row()).not.toBeNull();
});

test("without a request waiting, no row counts them", async () => {
  await shown();

  expect(screen.queryAllByText(/waiting for your approval/)).toHaveLength(0);
});

test("the post of a mention answers its actions: a favourite reaches the server, Reply opens the composer", async () => {
  const made = server();
  const id = made.fake.idOf("m");
  const favourite = made.fake.script("POST /statuses/{id}/favourite");
  const composed: ComposeRequest[] = [];
  const user = await shown(made, "en", (to) => composed.push(to));
  const post = within(entry(/mentioned you/));

  await user.click(post.getByRole("button", { name: /^Favourite/ }));
  expect((await favourite.arrived()).url.pathname).toEndWith(`/statuses/${id}/favourite`);
  await waitFor(() => {
    if (post.queryAllByRole("button", { name: /^Favourite/, pressed: true }).length === 0)
      throw new Error("the post does not say it is favourited");
  });

  await user.click(post.getByRole("button", { name: /^Reply/ }));
  expect(composed).toEqual([{ kind: "reply", id }]);
});

test("a vote in the poll of a mentioning post reaches the server", async () => {
  const made = server(answers([mention({ poll: { options: ["Yes", "No"], days: 1 } })]));
  const sent = made.fake.script("POST /statuses/{id}/poll/votes");
  const user = await shown(made);
  const poll = within(within(entry(/mentioned you/)).getByRole("group", { name: "Poll" }));

  await user.click(poll.getByRole("radio", { name: "No" }));
  await user.click(poll.getByRole("button", { name: "Vote" }));

  const { url, body } = await sent.arrived();
  expect(url.pathname).toEndWith(`/statuses/${made.fake.idOf("m")}/poll/votes`);
  expect(body.choices).toEqual([1]);
  await waitFor(() => {
    if (poll.queryAllByText(/your vote/).length !== 1) throw new Error("the poll shows no vote of the reader's");
  });
});

test("the Mentions tab asks the server for the mentions alone, and shows them in place of every notification", async () => {
  const { fake, data } = fakeData({ viewer: "sam" });
  await loadNotifications(data);
  const user = renderFeature(data, <Notifications instance={fakeHost} />);
  const feed = () => screen.getByRole("feed", { name: "Notifications" });
  expect(within(feed()).getByRole("article", { name: "Yuki 雪 followed you" })).toBeDefined();

  const read = fake.script("GET /notifications");
  await user.click(screen.getByRole("tab", { name: "Mentions" }));

  const { url } = await read.arrived();
  expect(url.searchParams.getAll("types")).toContain("mention");
  await waitFor(() => {
    expect(within(feed()).queryByRole("article", { name: "Yuki 雪 followed you" })).toBeNull();
  });
  expect(within(feed()).getAllByRole("article", { name: "Yuki 雪" }).length).toBeGreaterThan(0);
});

test("the Mentions tab keeps the mention, the quote and the new post of a subscribed account, each its sentence over its post", async () => {
  // One notification of every kind from people.
  const { data } = fakeData({ everyKind: "people" });
  await loadNotifications(data);
  const user = renderFeature(data, <Notifications instance={fakeHost} />);
  const feed = () => screen.getByRole("feed", { name: "Notifications" });
  expect(within(feed()).getAllByRole("article").length).toBeGreaterThan(3);

  await user.click(screen.getByRole("tab", { name: "Mentions" }));

  await waitFor(() => {
    expect(within(feed()).getAllByRole("article")).toHaveLength(3);
  });
  // Newest first, each named by its post's author, its sentence before the author's line.
  const posts = within(feed()).getAllByRole("article");
  const expected = [
    ["Sam Rivera", /^⁨?Sam Rivera⁩? mentioned you⁨?Sam Rivera/],
    ["Mio 澪", /^⁨?Mio 澪⁩? quoted your post⁨?Mio 澪/],
    ["Hana 花", /^⁨?Hana 花⁩? posted⁨?Hana 花/],
  ] as const;
  for (const [i, [author, start]] of expected.entries()) {
    const named = within(feed()).getAllByRole("article", { name: author });
    expect(named).toHaveLength(1);
    expect(named[0] === posts[i]).toBe(true);
    expect(posts[i]?.textContent).toMatch(start);
  }
});

// A notification the reader acts on becomes read. The scenario's notifications, newest first: Hana reacted to, boosted
// and favourited sam's post, Yuki favourited it, Yuki mentioned sam in three posts, then Marco, Layla, Yuki and Hana
// followed sam. Unless a test says otherwise, the reader has not read the three newest.

const everything = { unreadNotifications: 11 } as const;

/** The notifications of sam, drawn: the marker's and the dismissal's requests scripted to be counted, and where a link asked to go. */
async function inbox(options: FakeOptions = {}) {
  const made = fakeData({ viewer: "sam", ...options });
  const marker = made.fake.script("PUT /notifications/marker");
  const dismissed = made.fake.script("POST /notifications/{id}/dismiss");
  await loadNotifications(made.data);
  const went: string[] = [];
  const user = renderFeature(
    made.data,
    <NavigationProvider navigate={(href) => went.push(href)}>
      <PostActions onCompose={() => undefined}>
        <Notifications instance={fakeHost} />
      </PostActions>
    </NavigationProvider>,
  );
  const { data: page } = await made.data.api.GET("/notifications");
  /** The ID of the `n`th notification, 0 the newest. */
  const id = (n: number) => page?.items[n]?.id ?? `no notification ${String(n)}`;
  return { ...made, user, marker, dismissed, went, id };
}

/** The rows of the scenario's notifications that carry something to press. */
const rows = {
  boost: () => within(feed()).getByRole("article", { name: "Hana 花 boosted your post" }),
  follow: () => within(feed()).getByRole("article", { name: "Marco followed you" }),
  /** Yuki's mention of sam: a post with its reply, boost, favourite, react and more. */
  mention: () => {
    const row = entries().find((article) => article.textContent.includes("北側の尾根道です"));
    if (row === undefined) throw new Error("test: no notification of the mention");
    return row;
  },
};

const unreadMark = (row: HTMLElement) => within(row).queryByRole("img", { name: "Unread" });
/** How many notifications of the list say Unread. */
const unreadCount = () => within(feed()).queryAllByRole("img", { name: "Unread" }).length;

/** Waits until `row` shows no Unread mark and `unread` notifications of the list say Unread. */
const becomesRead = (row: () => HTMLElement, unread: number) =>
  waitFor(() => {
    if (unreadMark(row()) !== null) throw new Error("test: the row still says Unread");
    if (unreadCount() !== unread) throw new Error(`test: ${String(unreadCount())} notifications say Unread`);
  });

/** Sees that `row` is unread still, and `unread` notifications say Unread. */
const staysUnread = (row: HTMLElement, unread: number) => {
  expect(unreadMark(row)).not.toBeNull();
  expect(unreadCount()).toBe(unread);
};

/** The focus on `element`, as a reader's Tab or a script's gives it. */
const focus = (element: HTMLElement) => {
  element.focus();
  expect(document.activeElement).toBe(element);
};

test("each kind says who did what to which post, and the scenario's notifications are unread, each marked", async () => {
  await inbox(everything);

  expect(unreadCount()).toBe(11);
  expect(within(feed()).getByRole("article", { name: "Yuki 雪 followed you" })).not.toBeNull();
  // A mention is the post that mentions the reader, named by its author.
  const mentioned = entries().filter(
    (post) => post.textContent.includes("北側の尾根道です") && post.textContent.includes("Yuki 雪 mentioned you"),
  );
  expect(mentioned).toHaveLength(1);
  expect(within(feed()).getByRole("article", { name: "Yuki 雪 favourited your post" }).textContent).toContain(
    "Day 3 of the sourdough",
  );
  expect(within(feed()).getByRole("article", { name: "Hana 花 boosted your post" }).textContent).toContain(
    "Day 3 of the sourdough",
  );
});

test("a click on the words of a notification reads it", async () => {
  const { user } = await inbox(everything);
  await user.click(within(rows.boost()).getByText(/^Day 3 of the sourdough/));
  await becomesRead(rows.boost, 10);
});

// A button of the row, pressed by a pointer, by Enter and by Space.

type Press = (user: Awaited<ReturnType<typeof inbox>>["user"]) => Promise<void>;
const presses: [string, Press][] = [
  ["a pointer", (user) => user.click(document.activeElement as HTMLElement)],
  ["Enter", (user) => user.keyboard("{Enter}")],
  ["Space", (user) => user.keyboard(" ")],
];

const pressable: [string, keyof typeof rows, string | RegExp][] = [
  ["Follow back", "follow", "Follow back"],
  ["Favourite", "mention", "Favourite"],
  ["Reply", "mention", /^Reply/],
];

for (const [how, press] of presses) {
  for (const [title, row, name] of pressable) {
    test(`the ${title} button of a notification, pressed by ${how}, reads it`, async () => {
      const { user } = await inbox(everything);
      // Follow back waits for the reader's relationship with the account.
      focus(await within(rows[row]()).findByRole("button", { name }));
      await press(user);
      await becomesRead(rows[row], 10);
    });
  }

  test(`the Dismiss button of an unread notification, pressed by ${how}, dismisses it and the count drops by one`, async () => {
    const { user, dismissed, id } = await inbox(everything);
    focus(within(rows.boost()).getByRole("button", { name: "Dismiss notification" }));
    await press(user);
    await waitFor(() => {
      expect(unreadCount()).toBe(10);
    });
    expect(dismissed.seen.map((s) => s.url.pathname)).toEqual([`/api/v1/notifications/${id(1)}/dismiss`]);
    expect(within(feed()).queryByRole("article", { name: "Hana 花 boosted your post" })).toBeNull();
  });
}

test("the Show button of a notification of a post with a content warning reads it", async () => {
  const { user } = await inbox({
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
  await user.click(within(row()).getByRole("button", { name: /^Show/ }));
  await becomesRead(row, 0);
});

// A link of the row, followed by a pointer and by Enter: the link asks the app to go, and the row is read.

const links: [string, keyof typeof rows, string | RegExp][] = [
  ["its line", "boost", "Hana 花 boosted your post"],
  ["the author's name", "mention", "Yuki 雪"],
];

for (const [how, press] of presses.slice(0, 2)) {
  for (const [title, row, name] of links) {
    test(`following ${title} of a notification by ${how} reads it`, async () => {
      const { user, went } = await inbox(everything);
      focus(within(rows[row]()).getByRole("link", { name }));
      await press(user);
      await waitFor(() => {
        expect(went).toHaveLength(1);
      });
      expect(went[0]).not.toBe("/notifications");
      await becomesRead(rows[row], 10);
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
    const { user } = await inbox(everything);
    await user.click(within(rows.mention()).getByRole("button", { name: opener }));
    await user.click(await screen.findByRole("menuitem", { name: item }));
    await becomesRead(rows.mention, 10);
  });

  test(`the ${item} item of the ${title} menu of a notification, chosen by Enter, reads it`, async () => {
    const { user } = await inbox(everything);
    await user.click(within(rows.mention()).getByRole("button", { name: opener }));
    focus(await screen.findByRole("menuitem", { name: item }));
    await user.keyboard("{Enter}");
    await becomesRead(rows.mention, 10);
  });
}

// A control that opens a menu or the emoji picker reads it: opening alone counts.

const opens: [string, (user: Awaited<ReturnType<typeof inbox>>["user"], row: HTMLElement) => Promise<void>][] = [
  ["a press on More", (user, row) => user.click(within(row).getByRole("button", { name: "More" }))],
  [
    "Enter on More",
    async (user, row) => {
      focus(within(row).getByRole("button", { name: "More" }));
      await user.keyboard("{Enter}");
    },
  ],
  ["a press on Boost", (user, row) => user.click(within(row).getByRole("button", { name: "Boost" }))],
  ["a press on React", (user, row) => user.click(within(row).getByRole("button", { name: "React" }))],
  [
    "Enter on React",
    async (user, row) => {
      focus(within(row).getByRole("button", { name: "React" }));
      await user.keyboard("{Enter}");
    },
  ],
];

for (const [opening, open] of opens) {
  test(`opening a menu or the picker by ${opening} reads the notification`, async () => {
    const { user } = await inbox(everything);
    await open(user, rows.mention());
    await becomesRead(rows.mention, 10);
  });
}

// What does not read a notification: nothing was asked of it.

test("focusing a notification, and Tab from it, do not read it", async () => {
  const { user, marker } = await inbox(everything);
  const row = rows.mention();
  focus(row);
  staysUnread(row, 11);
  await user.tab();
  expect(document.activeElement).not.toBe(row);
  staysUnread(row, 11);
  expect(marker.seen).toHaveLength(0);
});

test("Space on the notification itself does not read it", async () => {
  const { user, marker } = await inbox(everything);
  const row = rows.mention();
  focus(row);
  await user.keyboard(" ");
  staysUnread(row, 11);
  expect(marker.seen).toHaveLength(0);
});

// The marker the server keeps.

test("reading the oldest unread notification moves the marker to it", async () => {
  const { user, marker, id } = await inbox();
  const favourite = within(feed()).getByRole("article", { name: "Hana 花 favourited your post" });
  await user.click(within(favourite).getByText(/^Day 3 of the sourdough/));
  await waitFor(() => {
    expect(marker.seen.map((s) => s.body.last_read_id)).toEqual([id(2)]);
  });
});

test("reading newer notifications while an older one is unread sends nothing, and reading the oldest moves the marker to the newest", async () => {
  const { user, marker, id } = await inbox();
  for (const name of ["Hana 花 reacted to your post", "Hana 花 boosted your post"]) {
    await user.click(within(within(feed()).getByRole("article", { name })).getByText(/^Day 3 of the sourdough/));
  }
  await waitFor(() => {
    expect(unreadCount()).toBe(1);
  });
  expect(marker.seen).toHaveLength(0);
  const oldest = within(feed()).getByRole("article", { name: "Hana 花 favourited your post" });
  await user.click(within(oldest).getByText(/^Day 3 of the sourdough/));
  await waitFor(() => {
    expect(marker.seen.map((s) => s.body.last_read_id)).toEqual([id(0)]);
  });
  expect(unreadCount()).toBe(0);
});
