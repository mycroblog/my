// SPDX-License-Identifier: AGPL-3.0-only

// The board of an event (ADR-0161 §17), on the fake API: what a story cannot reach, as it follows a press. The Board
// tab's address and the tab that replaces it; the posts on the board, newest first, each marked as the board's, with no
// boost and no bookmark; a writer's composer, which posts to the board and offers no audience, schedule or drafts; an
// edit, a deletion and a reply on the board; the refusals that read the event again; the line that says why a reader
// does not write; the board's empty, failed and longer reads; the answers that give and take the tabs, and where the
// focus goes then; the organizer's choice of who writes; and the notification of a post, which leads to the board. What
// each state looks like is the stories' (events.stories.tsx, `screens-events--board-*`).

import { scenario, type FakeEventPlan, type FakeOptions } from "@my/e2e/fake";
import { expect, onTestFinished, test } from "bun:test";
import { act, screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, words, type TestApp } from "./test/app";
import { composerOf, paste, replace } from "./test/composer";
import { fakeApp } from "./test/server";

const day = 24 * 60 * 60_000;

/** The Board tab's address of the event `id`. */
const boardPath = (id: string) => `/events/${id}?tab=board`;

/** Renders the app at the Board tab of the fake's event of `key`, once its board's feed shows its posts. */
async function atBoard(key: string, options: FakeOptions = {}, before?: (on: ReturnType<typeof fakeApp>) => void) {
  const on = fakeApp(options);
  const id = on.fake.events.id(key);
  before?.(on);
  const app = await renderApp(boardPath(id), { on });
  const feed = await main().findByRole("feed", { name: "The event's board" });
  return { app, id, feed };
}

/** Renders the app at /lists, then goes to `to`: the address before it is in the history. */
async function from(to: string, on: ReturnType<typeof fakeApp>): Promise<TestApp> {
  const app = await renderApp("/lists", { on });
  await app.navigate(to);
  return app;
}

/** The event's tabs, named by its title; none for a reader who does not read the board. */
const tabsOf = (title: string) => main().queryAllByRole("tablist", { name: title });

/** The tab `name` of the event titled `title`. */
const tab = (title: string, name: "Details" | "Board") =>
  within(main().getByRole("tablist", { name: title })).getByRole("tab", { name });

/** The posts of the board, in their order, each as its first words. */
const posts = (feed: HTMLElement) =>
  within(feed)
    .queryAllByRole("article")
    .map((post) => words(post));

/** The post of the board whose text holds `text`. */
function postWith(feed: HTMLElement, text: string): HTMLElement {
  const found = within(feed)
    .queryAllByRole("article")
    .filter((post) => post.textContent.includes(text));
  const [post] = found;
  if (post === undefined || found.length !== 1) throw new Error(`test: ${String(found.length)} posts hold "${text}"`);
  return post;
}

/** The ID of `post`, from the link of its time to its own page. */
function idOf(post: HTMLElement): string {
  const href = within(post)
    .getAllByRole("link")
    .map((link) => link.getAttribute("href") ?? "")
    .find((h) => h.includes("/status/"));
  if (href === undefined) throw new Error("test: the post has no link to its page");
  return href.slice(href.lastIndexOf("/") + 1);
}

/** What has the focus, as "<role>: <name>". */
function focused(): string {
  const el = document.activeElement;
  if (el === null || el === document.body) return "nothing";
  const role =
    el.getAttribute("role") ?? (el instanceof HTMLInputElement ? el.type : undefined) ?? el.tagName.toLowerCase();
  const checked = el instanceof HTMLInputElement && el.checked ? " (checked)" : "";
  return `${role}: ${el.getAttribute("aria-label") ?? words(el.closest("label") ?? el)}${checked}`;
}

/** Waits until the main says `sentence`. */
const says = (sentence: string) =>
  waitFor(() => {
    const said = [words(screen.getByRole("main")), ...screen.queryAllByRole("status").map((s) => words(s))];
    if (!said.some((text) => text.includes(sentence))) throw new Error(`test: the page says ${JSON.stringify(said)}`);
  });

/** Chooses the reader's answer `choice` to the event, and returns its radio. */
async function answer(app: TestApp, choice: "Going" | "Maybe" | "Not going"): Promise<HTMLElement> {
  const radio = within(main().getByRole("radiogroup", { name: "Your answer" })).getByRole("radio", { name: choice });
  await app.user.click(radio);
  return radio;
}

/** Opens the composer of the board by Write on the board. */
async function writeOnTheBoard(app: TestApp) {
  await app.user.click(main().getByRole("button", { name: "Write on the board" }));
  return composerOf("New post");
}

const readOnly = {
  cancelled: "The event was cancelled, so the board is read-only.",
  ended: "7 days have passed since the event ended, so the board is read-only.",
  organizer: "Only the organizer can write on this board.",
  organizers: "Only the organizer and the co-organizers can write on this board.",
} as const;

// ---- The Board tab and its address -------------------------------------------------------------------------------

test("the Board tab's address opens the board under the event's tabs, its posts newest first, each marked as the board's", async () => {
  const { feed } = await atBoard("bread-class");
  expect(tabsOf("Sourdough class")).toHaveLength(1);
  expect(tab("Sourdough class", "Board").getAttribute("aria-selected")).toBe("true");
  expect(tab("Sourdough class", "Details").getAttribute("aria-selected")).toBe("false");
  const [newest, oldest] = posts(feed);
  expect(newest).toContain("I can bring extra flour and a scale.");
  expect(oldest).toContain("The oven fits four loaves at once");
  for (const post of within(feed).getAllByRole("article")) {
    expect(within(post).getAllByRole("img", { name: "On the event's board" })).toHaveLength(1);
    // The glyph stands in the visibility's place: no post of the board says who sees it.
    expect(within(post).queryAllByRole("img", { name: /^(Public|Unlisted|Followers only|Direct)$/ })).toHaveLength(0);
  }
});

test("choosing a tab replaces the address with the tab's, so that Back leaves the event", async () => {
  const on = fakeApp();
  const id = on.fake.events.id("bread-class");
  const app = await from(`/events/${id}`, on);
  await main().findByRole("tablist", { name: "Sourdough class" });
  await app.user.click(tab("Sourdough class", "Board"));
  await main().findByRole("feed", { name: "The event's board" });
  await waitFor(() => {
    if (app.path() !== boardPath(id)) throw new Error(`test: the app is at ${app.path()}`);
  });
  await app.user.click(tab("Sourdough class", "Details"));
  await main().findByRole("radiogroup", { name: "Your answer" });
  await waitFor(() => {
    if (app.path().split("?")[0] !== `/events/${id}` || app.path().includes("tab=board"))
      throw new Error(`test: the app is at ${app.path()}`);
  });
  await app.back();
  expect(app.path()).toBe("/lists");
});

test("a reader who may not read the board sees no tabs and no board, at the event's address", async () => {
  const on = fakeApp();
  const id = on.fake.events.id("garden-day");
  const board = on.fake.script("GET /events/{id}/board");
  const app = await renderApp(boardPath(id), { on });
  await main().findByRole("radiogroup", { name: "Your answer" });
  await waitFor(() => {
    if (app.path() !== `/events/${id}`) throw new Error(`test: the app is at ${app.path()}`);
  });
  expect(tabsOf("Community garden day")).toHaveLength(0);
  expect(main().queryAllByRole("tab")).toHaveLength(0);
  expect(main().queryAllByRole("feed", { name: "The event's board" })).toHaveLength(0);
  expect(board.seen).toHaveLength(0);
});

// ---- The posts of the board --------------------------------------------------------------------------------------

test("a post on the board offers no boost, and its More menu no bookmark", async () => {
  const { app, feed } = await atBoard("bread-class");
  const post = postWith(feed, "extra flour");
  expect(within(post).queryAllByRole("button", { name: /^Boost/ })).toHaveLength(0);
  expect(within(post).getAllByRole("button", { name: /^Reply/ })).toHaveLength(1);
  await app.user.click(within(post).getByRole("button", { name: "More" }));
  const menu = await screen.findByRole("menu", { name: "More" });
  const items = within(menu)
    .getAllByRole("menuitem")
    .map((item) => words(item));
  expect(items).toContain("Copy link");
  expect(items.filter((item) => /bookmark/i.test(item))).toEqual([]);
});

test("a post on the board is favourited as any post", async () => {
  const { app, feed } = await atBoard("bread-class");
  const post = postWith(feed, "extra flour");
  const favourite = app.fake.script("POST /statuses/{id}/favourite");
  await app.user.click(within(post).getByRole("button", { name: "Favourite" }));
  const sent = await favourite.served();
  expect(sent.url.pathname).toEndWith(`/statuses/${idOf(post)}/favourite`);
});

test("an empty board says so, and its writer is offered Write on the board", async () => {
  const on = fakeApp();
  const board = on.fake.script("GET /events/{id}/board");
  await renderApp(boardPath(on.fake.events.id("cooking-night")), { on });
  const panel = await main().findByRole("tabpanel", { name: "Board" });
  await board.served();
  await waitFor(() => {
    expect(words(panel)).toContain("Nothing on the board yet.");
  });
  expect(within(panel).queryAllByRole("article")).toHaveLength(0);
  expect(within(panel).getAllByRole("button", { name: "Write on the board" })).toHaveLength(1);
});

test("a board that could not be read says so, and its Try again reads it", async () => {
  const on = fakeApp();
  const id = on.fake.events.id("bread-class");
  const failing = on.fake.script("GET /events/{id}/board", { fail: 500 });
  const app = await renderApp(boardPath(id), { on });
  const panel = await main().findByRole("tabpanel", { name: "Board" });
  const alert = await within(panel).findByRole("alert");
  expect(words(alert)).toContain("The posts could not be loaded.");
  failing.end();
  await app.user.click(within(alert).getByRole("button", { name: "Try again" }));
  const feed = await main().findByRole("feed", { name: "The event's board" });
  await waitFor(() => {
    expect(within(feed).getAllByRole("article")).toHaveLength(2);
  });
});

/** An event the reader organizes with 21 posts on its board, one a minute, the first the oldest. */
const crowded: FakeEventPlan = {
  key: "crowded-board",
  organizer: "yuki",
  title: "Crowded board",
  description: "",
  timeZone: "UTC",
  inDays: 5,
  hour: 12,
  hours: 2,
  board: Array.from({ length: 21 }, (_, i) => ({ by: "hana", text: `Note number ${String(i)}.`, minutesAgo: 30 - i })),
};

/**
 * The browser's IntersectionObserver, held by the test until it ends: `reach` says that everything watched came into
 * view, as the end of a feed does when the reader scrolls to it. happy-dom's observer never calls back.
 */
function watching() {
  const watchers: { callback: IntersectionObserverCallback; watched: Element[]; self: IntersectionObserver }[] = [];
  const real = globalThis.IntersectionObserver;
  class Held implements IntersectionObserver {
    readonly root = null;
    readonly rootMargin = "";
    readonly scrollMargin = "";
    readonly thresholds = [];
    readonly #watcher: (typeof watchers)[number];
    constructor(callback: IntersectionObserverCallback) {
      this.#watcher = { callback, watched: [], self: this };
      watchers.push(this.#watcher);
    }
    observe(target: Element) {
      this.#watcher.watched.push(target);
    }
    unobserve(target: Element) {
      this.#watcher.watched = this.#watcher.watched.filter((element) => element !== target);
    }
    disconnect() {
      this.#watcher.watched = [];
    }
    takeRecords() {
      return [];
    }
  }
  globalThis.IntersectionObserver = Held;
  onTestFinished(() => {
    globalThis.IntersectionObserver = real;
  });
  return {
    reach: () => {
      act(() => {
        for (const { callback, watched, self } of watchers)
          if (watched.length > 0)
            callback(
              watched.map(
                (target) => ({ isIntersecting: true, intersectionRatio: 1, target }) as IntersectionObserverEntry,
              ),
              self,
            );
      });
    },
  };
}

test("a board longer than a page shows the newest twenty, and its end in view brings the rest after them", async () => {
  const { reach } = watching();
  const { app, feed } = await atBoard("crowded-board", {
    seed: { ...scenario, events: [...scenario.events, crowded] },
  });
  await waitFor(() => {
    expect(within(feed).getAllByRole("article")).toHaveLength(20);
  });
  expect(posts(feed)[0]).toContain("Note number 20.");
  const next = app.fake.script("GET /events/{id}/board");
  reach();
  await next.served();
  await waitFor(() => {
    expect(within(feed).getAllByRole("article")).toHaveLength(21);
  });
  expect(posts(feed)[20]).toContain("Note number 0.");
  expect(next.seen[0]?.url.searchParams.get("cursor") ?? "").not.toBe("");
});

test("more of the board that could not be read says so under its posts, and its Try again reads it", async () => {
  const { reach } = watching();
  const { app, feed } = await atBoard("crowded-board", {
    seed: { ...scenario, events: [...scenario.events, crowded] },
  });
  await waitFor(() => {
    expect(within(feed).getAllByRole("article")).toHaveLength(20);
  });
  const failing = app.fake.script("GET /events/{id}/board", { fail: 500 });
  reach();
  const alert = await within(main().getByRole("tabpanel", { name: "Board" })).findByRole("alert");
  expect(words(alert)).toContain("More posts could not be loaded.");
  failing.end();
  await app.user.click(within(alert).getByRole("button", { name: "Try again" }));
  await waitFor(() => {
    expect(within(feed).getAllByRole("article")).toHaveLength(21);
  });
});

// ---- Writing on the board ----------------------------------------------------------------------------------------

test("Write on the board opens the composer of a new post on the board, without an audience, a schedule or drafts", async () => {
  const { app } = await atBoard("bread-class");
  const { dialog } = await writeOnTheBoard(app);
  expect(words(dialog)).toContain("On the board of Sourdough class");
  const buttons = within(dialog).getAllByRole("button");
  const names = buttons.map((b) => b.getAttribute("aria-label") ?? words(b));
  for (const offered of ["Add emoji", "Add images", "Add a poll", "Add a content warning", "Post"])
    expect([offered, names.includes(offered)]).toEqual([offered, true]);
  expect(names.filter((n) => /^(Who can see it|Schedule|Drafts)/.test(n))).toEqual([]);
});

test("a post written on the board is sent to the board once, with an Idempotency-Key, and shows at its top", async () => {
  const { app, id, feed } = await atBoard("bread-class");
  const board = app.fake.script("POST /events/{id}/board");
  const statuses = app.fake.script("POST /statuses");
  const composer = await writeOnTheBoard(app);
  await paste(app, composer.text, "The class starts at two sharp.");
  await app.user.click(within(composer.dialog).getByRole("button", { name: "Post" }));
  const sent = await board.served();
  expect(sent.url.pathname).toEndWith(`/events/${id}/board`);
  expect(sent.body.text).toBe("The class starts at two sharp.");
  expect(sent.headers.get("Idempotency-Key") ?? "").not.toBe("");
  await waitFor(() => {
    if (screen.queryAllByRole("dialog", { name: "New post" }).length > 0) throw new Error("test: the composer is open");
  });
  await waitFor(() => {
    expect(posts(feed)[0]).toContain("The class starts at two sharp.");
  });
  expect(within(postWith(feed, "two sharp")).getAllByRole("img", { name: "On the event's board" })).toHaveLength(1);
  expect(board.seen).toHaveLength(1);
  expect(statuses.seen).toHaveLength(0);
});

test("a reply to a post of the board goes to the board, in reply to it", async () => {
  const { app, id, feed } = await atBoard("bread-class");
  const post = postWith(feed, "extra flour");
  const board = app.fake.script("POST /events/{id}/board");
  const statuses = app.fake.script("POST /statuses");
  await app.user.click(within(post).getByRole("button", { name: /^Reply/ }));
  const composer = await composerOf("Reply");
  // The reply starts with the handle of the post's writer.
  await replace(app, composer.text, "@hana Thank you, a scale helps.");
  await app.user.click(within(composer.dialog).getByRole("button", { name: /^(Post|Reply)$/ }));
  const sent = await board.served();
  expect(sent.url.pathname).toEndWith(`/events/${id}/board`);
  expect(sent.body.in_reply_to_id).toBe(idOf(post));
  expect(statuses.seen).toHaveLength(0);
});

test("the writer of a post on the board edits it from its More menu, and the board shows the new text", async () => {
  const { app, feed } = await atBoard("bread-class", { viewer: "sam" });
  const post = postWith(feed, "The oven fits four loaves");
  const id = idOf(post);
  const edited = app.fake.script("PUT /statuses/{id}");
  await app.user.click(within(post).getByRole("button", { name: "More" }));
  const menu = await screen.findByRole("menu", { name: "More" });
  await app.user.click(within(menu).getByRole("menuitem", { name: "Edit" }));
  const composer = await composerOf("Edit post");
  await replace(app, composer.text, "The oven fits six loaves at once.");
  await app.user.click(within(composer.dialog).getByRole("button", { name: /^(Save|Update|Post)$/ }));
  const sent = await edited.served();
  expect(sent.url.pathname).toEndWith(`/statuses/${id}`);
  await waitFor(() => {
    expect(words(postWith(feed, "six loaves"))).toContain("The oven fits six loaves at once.");
  });
});

test("the writer of a post on the board deletes it from its More menu, asked first, and it leaves the board", async () => {
  const { app, feed } = await atBoard("bread-class", { viewer: "sam" });
  const post = postWith(feed, "The oven fits four loaves");
  await app.user.click(within(post).getByRole("button", { name: "More" }));
  const menu = await screen.findByRole("menu", { name: "More" });
  await app.user.click(within(menu).getByRole("menuitem", { name: "Delete…" }));
  const confirm = await screen.findByRole("alertdialog", { name: "Delete this post?" });
  await app.user.click(within(confirm).getByRole("button", { name: "Delete" }));
  await waitFor(() => {
    if (posts(feed).some((p) => p.includes("The oven fits four loaves")))
      throw new Error("test: the post is still there");
  });
  expect(posts(feed)).toHaveLength(1);
});

// ---- The refusals, and the line in the bar's place ---------------------------------------------------------------

const refusals = [
  ["event-board-closed", 409, "The board is read-only"],
  ["event-board-not-writer", 403, "Not a writer"],
  ["event-board-not-reader", 403, "Not a reader"],
] as const;

for (const [type, status, title] of refusals)
  test(`a post the server refuses as ${type} reads the event again`, async () => {
    const { app } = await atBoard("bread-class");
    app.fake.script("POST /events/{id}/board", {
      times: 1,
      fail: { type: `/api/v1/problems/${type}`, title, status },
    });
    const read = app.fake.script("GET /events/{id}");
    const composer = await writeOnTheBoard(app);
    await paste(app, composer.text, "Refused, as it turns out.");
    await app.user.click(within(composer.dialog).getByRole("button", { name: "Post" }));
    await read.arrived();
  });

test("a post refused as the board closed meanwhile reads the event again, and the board then says it is read-only", async () => {
  const { app } = await atBoard("bread-class");
  app.fake.script("POST /events/{id}/board", {
    times: 1,
    fail: { type: "/api/v1/problems/event-board-closed", title: "The board is read-only", status: 409 },
  });
  app.fake.script("GET /events/{id}", {
    change: (event) => ({
      ...event,
      state: "canceled" as const,
      cancellation: { reason: "organizer" as const, canceled_at: new Date().toISOString() },
      viewer: { ...event.viewer, board: { readable: true, writable: false } },
    }),
  });
  const composer = await writeOnTheBoard(app);
  await paste(app, composer.text, "Refused, as it turns out.");
  await app.user.click(within(composer.dialog).getByRole("button", { name: "Post" }));
  await says(readOnly.cancelled);
  expect(main().queryAllByRole("button", { name: "Write on the board" })).toHaveLength(0);
});

test("the board of a cancelled event says it is read-only in the bar's place", async () => {
  await atBoard("flea-market");
  await says(readOnly.cancelled);
  expect(main().queryAllByRole("button", { name: "Write on the board" })).toHaveLength(0);
});

test("the board says it is read-only from 7 days after the event's end", async () => {
  await atBoard("bread-class", {}, (on) => {
    const ended = Date.now() - 8 * day;
    on.fake.script("GET /events/{id}", {
      change: (event) => ({
        ...event,
        starts_at: new Date(ended - 3 * 3_600_000).toISOString(),
        ends_at: new Date(ended).toISOString(),
        viewer: { ...event.viewer, board: { readable: true, writable: false } },
      }),
    });
  });
  await says(readOnly.ended);
  expect(main().queryAllByRole("button", { name: "Write on the board" })).toHaveLength(0);
});

test("a reader whom only the organizers let write is told so in the bar's place", async () => {
  await atBoard("book-swap");
  await says(readOnly.organizers);
  expect(main().queryAllByRole("button", { name: "Write on the board" })).toHaveLength(0);
});

test("a reader whom only the organizer lets write is told so in the bar's place", async () => {
  await atBoard("book-swap", {}, (on) => {
    on.fake.script("GET /events/{id}", { change: (event) => ({ ...event, board_writers: "organizer" as const }) });
  });
  await says(readOnly.organizer);
});

test("a post refused as the organizer let fewer write meanwhile keeps the composer with the server's words, the line in the bar's place", async () => {
  // Hana goes to the class, which every reader wrote on when the page was read.
  const on = fakeApp({ viewer: "hana" });
  const opened = on.fake.script("GET /events/{id}", {
    change: (event) => ({
      ...event,
      board_writers: "readers" as const,
      viewer: { ...event.viewer, board: { readable: true, writable: true } },
    }),
  });
  const app = await renderApp(boardPath(on.fake.events.id("bread-class")), { on });
  await main().findByRole("feed", { name: "The event's board" });
  const composer = await writeOnTheBoard(app);
  // The organizer left the board to the organizers since: the server refuses the post, and the event read again
  // says who writes.
  opened.end();
  const refused = on.fake.script("POST /events/{id}/board");
  await paste(app, composer.text, "Refused, as it turns out.");
  await app.user.click(within(composer.dialog).getByRole("button", { name: "Post" }));
  await refused.arrived();
  await says(readOnly.organizers);
  expect(main().queryAllByRole("button", { name: "Write on the board" })).toHaveLength(0);
  // The composer keeps what was written, with the server's words.
  const dialog = screen.getByRole("dialog", { name: "New post" });
  expect(words(within(dialog).getByRole("alert"))).toContain("Only the people this event's organizer chose can write");
});

// ---- The answers that give and take the board ---------------------------------------------------------------------

test("a saved answer that gives the board brings the tabs at once, the focus staying on the answer chosen", async () => {
  const on = fakeApp();
  const id = on.fake.events.id("garden-day");
  const app = await renderApp(`/events/${id}`, { on });
  await main().findByRole("radiogroup", { name: "Your answer" });
  expect(tabsOf("Community garden day")).toHaveLength(0);
  await answer(app, "Going");
  await waitFor(() => {
    expect(tabsOf("Community garden day")).toHaveLength(1);
  });
  expect(tab("Community garden day", "Details").getAttribute("aria-selected")).toBe("true");
  await waitFor(() => {
    if (focused() !== "radio: Going (checked)") throw new Error(`test: the focus is on ${focused()}`);
  });
});

test("a saved answer that takes the board takes the tabs at once, the focus staying on the answer chosen", async () => {
  const on = fakeApp();
  const id = on.fake.events.id("book-swap");
  const app = await renderApp(`/events/${id}`, { on });
  await main().findByRole("tablist", { name: "Book swap" });
  await answer(app, "Not going");
  await waitFor(() => {
    expect(tabsOf("Book swap")).toHaveLength(0);
  });
  expect(main().queryAllByRole("feed", { name: "The event's board" })).toHaveLength(0);
  await waitFor(() => {
    if (focused() !== "radio: Not going (checked)") throw new Error(`test: the focus is on ${focused()}`);
  });
});

test("a reader who lost the board while on the Board tab is put at the event's address in place of the tab's", async () => {
  const on = fakeApp();
  const id = on.fake.events.id("cooking-night");
  const app = await from(boardPath(id), on);
  await main().findByRole("tabpanel", { name: "Board" });
  // The reader answered not going from another device since: the server refuses the post, and the event read
  // again has no board for them.
  await on.fake.serve(
    new Request(`https://demo.test/api/v1/events/${id}/rsvp`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ answer: "declined" }),
    }),
  );
  const composer = await writeOnTheBoard(app);
  await paste(app, composer.text, "Who brings the flour?");
  await app.user.click(within(composer.dialog).getByRole("button", { name: "Post" }));
  await waitFor(() => {
    if (app.path() !== `/events/${id}`) throw new Error(`test: the app is at ${app.path()}`);
  });
  expect(tabsOf("Cooking night")).toHaveLength(0);
  expect(main().queryAllByRole("tabpanel")).toHaveLength(0);
  // The address was replaced: Back leaves the event.
  await app.back();
  expect(app.path()).toBe("/lists");
});

// ---- Who writes on the board, in the event's form -----------------------------------------------------------------

const writers = /^Who can write on the board$/;

test("the form that creates an event asks who writes on the board after who sees it, and sends the choice", async () => {
  const on = fakeApp();
  const create = on.fake.script("POST /events");
  const app = await renderApp("/events/new", { on });
  await app.user.type(await main().findByRole("textbox", { name: "Title" }), "Tea by the river");
  const sees = main().getByRole("radiogroup", { name: "Who can see the event" });
  const group = main().getByRole("radiogroup", { name: writers });
  expect(sees.compareDocumentPosition(group) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  expect(
    within(group)
      .getAllByRole("radio")
      .map((r) => words(r.closest("label") ?? r)),
  ).toEqual(["Only you", "You and the co-organizers", "Everyone who reads the board"]);
  await says("You, the co-organizers, and the people who answered going or maybe can read the board.");
  await app.user.click(within(group).getByText("Everyone who reads the board"));
  await app.user.click(main().getByRole("button", { name: "Create the event" }));
  const sent = await create.arrived();
  expect(sent.body.board_writers).toBe("readers");
  await main().findByRole("heading", { name: "Tea by the river", level: 2 });
  expect(on.fake.events.shown("Tea by the river")?.board_writers).toBe("readers");
});

test("the organizer changes who writes on the board, and the change sends it", async () => {
  const on = fakeApp();
  const id = on.fake.events.id("photo-walk");
  const app = await renderApp(`/events/${id}/edit`, { on });
  await main().findByRole("heading", { name: "Change the event", level: 2 });
  const patch = on.fake.script("PATCH /events/{id}");
  const group = main().getByRole("radiogroup", { name: writers });
  await app.user.click(within(group).getByText("Only you"));
  await app.user.click(main().getByRole("button", { name: "Save" }));
  const sent = await patch.arrived();
  expect(sent.body.board_writers).toBe("organizer");
  await waitFor(() => {
    expect(on.fake.events.shown("photo-walk")?.board_writers).toBe("organizer");
  });
});

test("the organizer's change that leaves who writes as it was does not send it", async () => {
  const on = fakeApp();
  const id = on.fake.events.id("photo-walk");
  const app = await renderApp(`/events/${id}/edit`, { on });
  await main().findByRole("heading", { name: "Change the event", level: 2 });
  const patch = on.fake.script("PATCH /events/{id}");
  await app.user.type(main().getByRole("textbox", { name: "Title" }), "!");
  await app.user.click(main().getByRole("button", { name: "Save" }));
  const sent = await patch.arrived();
  expect(sent.body.board_writers).toBeUndefined();
});

test("a co-organizer is told who writes on the board, the organizer's to change, and their change does not send it", async () => {
  const on = fakeApp();
  const id = on.fake.events.id("bread-class");
  const app = await renderApp(`/events/${id}/edit`, { on });
  await main().findByRole("heading", { name: "Change the event", level: 2 });
  expect(main().queryAllByRole("radiogroup", { name: writers })).toHaveLength(0);
  await says(
    "The organizer and the co-organizers can write on the board. Only the organizer, Sam Rivera, changes who can write on it.",
  );
  const patch = on.fake.script("PATCH /events/{id}");
  await app.user.type(main().getByRole("textbox", { name: "Title" }), "!");
  await app.user.click(main().getByRole("button", { name: "Save" }));
  const sent = await patch.arrived();
  expect(sent.body.board_writers).toBeUndefined();
});

// ---- The notification of a post on the board ----------------------------------------------------------------------

test("a post on the board is notified as a line with its writer's avatar, an excerpt and a link to the Board tab", async () => {
  const on = fakeApp({ everyKind: "people" });
  const id = on.fake.events.id("bread-class");
  await renderApp("/notifications", { on });
  const row = await main().findByRole("article", { name: "Hana 花 wrote on the board of Sourdough class" });
  expect(words(row)).toContain("I can bring extra flour and a scale.");
  const links = within(row)
    .getAllByRole("link")
    .map((link) => link.getAttribute("href"));
  expect(links).toContain(boardPath(id));
  expect(within(row).getAllByRole("presentation", { hidden: true }).length).toBeGreaterThan(0);
});

test("the address of the notification a push opens leads on to the Board tab", async () => {
  const on = fakeApp({ everyKind: "people" });
  const id = on.fake.events.id("bread-class");
  const app = await renderApp(on.fake.notificationPath("event_board_posted"), { on });
  await waitFor(() => {
    if (app.path() !== boardPath(id)) throw new Error(`test: the app is at ${app.path()}`);
  });
  await main().findByRole("feed", { name: "The event's board" });
  expect(tab("Sourdough class", "Board").getAttribute("aria-selected")).toBe("true");
});

// ---- An unknown tab, and the board changing under an open page ---------------------------------------------------

// Any other tab of the address is Details.
test("any other tab in the address opens the Details, with the answers and no board", async () => {
  const on = fakeApp();
  const id = on.fake.events.id("bread-class");
  await renderApp(`/events/${id}?tab=elsewhere`, { on });
  await main().findByRole("tablist", { name: "Sourdough class" });
  await waitFor(() => {
    if (tab("Sourdough class", "Details").getAttribute("aria-selected") !== "true")
      throw new Error("test: the Details tab is not chosen");
  });
  expect(main().getAllByRole("radiogroup", { name: "Your answer" })).toHaveLength(1);
  expect(main().queryAllByRole("feed", { name: "The event's board" })).toHaveLength(0);
});

// A page left open past the 7 days after the end closes the board as the answers close at the start.
test("the bar that has the focus gives way to the read-only line as the 7 days pass, the focus going to the Board tab", async () => {
  const { app } = await atBoard("bread-class", {}, (on) => {
    // The 7 days after the end pass a second and a half after the page opened.
    const ended = Date.now() - 7 * day + 1500;
    on.fake.script("GET /events/{id}", {
      change: (event) => ({
        ...event,
        starts_at: new Date(ended - 3 * 3_600_000).toISOString(),
        ends_at: new Date(ended).toISOString(),
      }),
    });
  });
  const bar = main().getByRole("button", { name: "Write on the board" });
  act(() => {
    bar.focus();
  });
  expect(app.focused()).toBe(bar);
  await waitFor(
    () => {
      if (main().queryAllByRole("button", { name: "Write on the board" }).length > 0)
        throw new Error("test: the bar is still there");
    },
    { timeout: 4000 },
  );
  await says(readOnly.ended);
  await waitFor(() => {
    if (focused() !== "tab: Board") throw new Error(`test: the focus is on ${focused()}`);
  });
});

// A post refused as `event-board-not-writer`: the event read again puts the line in the bar's place, and the composer,
// closed, gives the focus to the Board tab in place of the bar that is gone.
test("the composer closed after its bar gave way to the read-only line leaves the focus on the Board tab", async () => {
  const on = fakeApp({ viewer: "hana" });
  const opened = on.fake.script("GET /events/{id}", {
    change: (event) => ({
      ...event,
      board_writers: "readers" as const,
      viewer: { ...event.viewer, board: { readable: true, writable: true } },
    }),
  });
  const app = await renderApp(boardPath(on.fake.events.id("bread-class")), { on });
  await main().findByRole("feed", { name: "The event's board" });
  const composer = await writeOnTheBoard(app);
  opened.end();
  await paste(app, composer.text, "Refused, as it turns out.");
  await app.user.click(within(composer.dialog).getByRole("button", { name: "Post" }));
  await says(readOnly.organizers);
  await app.user.keyboard("{Escape}");
  await waitFor(() => {
    if (screen.queryAllByRole("dialog").length > 0) throw new Error("test: the composer is open");
  });
  await waitFor(() => {
    if (focused() !== "tab: Board") throw new Error(`test: the focus is on ${focused()}`);
  });
});

// A post refused as `event-board-not-reader` (the reader answered not going elsewhere): the address is the event's,
// and once the composer closes the focus is on the screen's title.
test("a reader who lost the board while on the Board tab has the focus on the screen's title once the composer closes", async () => {
  const on = fakeApp();
  const id = on.fake.events.id("cooking-night");
  const app = await from(boardPath(id), on);
  await main().findByRole("tabpanel", { name: "Board" });
  await on.fake.serve(
    new Request(`https://demo.test/api/v1/events/${id}/rsvp`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ answer: "declined" }),
    }),
  );
  const composer = await writeOnTheBoard(app);
  await paste(app, composer.text, "Who brings the flour?");
  await app.user.click(within(composer.dialog).getByRole("button", { name: "Post" }));
  await waitFor(() => {
    if (app.path() !== `/events/${id}`) throw new Error(`test: the app is at ${app.path()}`);
  });
  const during = focused();
  await app.user.keyboard("{Escape}");
  await waitFor(() => {
    if (screen.queryAllByRole("dialog").length > 0) throw new Error("test: the composer is open");
  });
  await waitFor(() => {
    if (focused() !== "h1: Cooking night")
      throw new Error(`test: the focus is on ${focused()} (with the composer open, ${during})`);
  });
});
