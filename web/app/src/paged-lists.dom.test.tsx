// SPDX-License-Identifier: AGPL-3.0-only

// The reader's lists that grow under Show more (#3279), each on its route and the fake API with the data of its story of
// a second page (pages.samples.ts): the open voice rooms, a room's invitations and the rooms the reader hosted; the
// accounts whose notifications are held back; the supporters; an event's attendees, its invitations and who answered a
// date of its poll; the reader's own custom emoji and their requests; the moderators' past actions on the account; the
// blocked accounts; a list's members; and who reacted to a post (forty a page). Each shows a page of twenty, a press of
// Show more adds the rest under it with the focus on the first it added, and Show more goes once the list is read to
// its end; who answered a date is read only once the date is opened, and its name counts them all. Then
// the windows of periods: the public finance's Later and Earlier, and the administrators' income, whose window and
// list of movements page apart. The invites, follows, follow requests and the report panel's posts are
// show-more-routes.dom.test.tsx's, the staff's lists staff-pages.dom.test.tsx's.

import { createAppData, watchServer } from "@my/app-data";
import { fakeOrigin, scenario, type FakeEventPlan, type FakePerson } from "@my/e2e/fake";
import { createClient } from "@mycroblog/mykit";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import { expect, test } from "bun:test";
import {
  manyAttending,
  manyBlocked,
  manyHeld,
  manyMembers,
  manyReactors,
  manySupporters,
  manyVoice,
} from "./pages.samples";
import { main, renderApp, words, type AppOptions, type TestApp } from "./test/app";
import { callsOnFakes } from "./test/call";
import { fakeApp, type FakeApp } from "./test/server";

/** How many items a page of the server's holds when the request names no limit. */
const page = 20;

/**
 * Waits until the focus is on `items[at]`, in it, or on the row that holds it and not the one before: the first item a
 * press of Show more added.
 */
const focusIsOn = (app: TestApp, items: () => readonly HTMLElement[], at: number) =>
  waitFor(() => {
    const focused = app.focused();
    const item = items()[at];
    const before = items()[at - 1];
    const onIt = focused !== null && item !== undefined && (item.contains(focused) || focused.contains(item));
    if (!onIt || (before !== undefined && focused.contains(before)))
      throw new Error(`test: the focus is on ${focused?.outerHTML.slice(0, 80) ?? "the page"}`);
  });

/** The first Show more that follows `list` in the screen's main. */
function moreAfter(list: HTMLElement): HTMLElement {
  const more = main()
    .getAllByRole("button", { name: "Show more" })
    .find((b) => (list.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0);
  if (more === undefined) throw new Error("test: no Show more after the list");
  return more;
}

/**
 * A page of twenty of `items`, then Show more (`more`): the rest under it, `total` when it is known, the focus on the
 * first it added; and no Show more left.
 */
async function growsWhole(
  app: TestApp,
  items: () => readonly HTMLElement[],
  more: () => HTMLElement,
  total?: number,
  left = () => main().queryAllByRole("button", { name: "Show more" }),
) {
  await waitFor(() => {
    if (items().length !== page) throw new Error(`test: ${String(items().length)} items before Show more`);
  });
  await app.user.click(more());
  await waitFor(() => {
    const shown = items().length;
    if (total === undefined ? shown <= page : shown !== total)
      throw new Error(`test: ${String(shown)} items after Show more`);
  });
  await focusIsOn(app, items, page);
  await waitFor(() => {
    if (left().length !== 0) throw new Error("test: Show more is still there");
  });
}

/** The items of the list named `name` in the screen's main. */
const itemsOf = (name: string | RegExp) => () => within(main().getByRole("list", { name })).getAllByRole("listitem");

test("the open voice rooms: Show more adds the rooms past the first twenty, the focus on the first it added", async () => {
  const app = await renderApp("/voice", manyVoice);
  await main().findByRole("list", { name: "Rooms open now" });
  await growsWhole(app, itemsOf("Rooms open now"), () => main().getByRole("button", { name: "Show more" }));
});

test("the invitations of the room the reader hosts: Show more adds the newer ones under the oldest twenty", async () => {
  const calls = callsOnFakes();
  const on = fakeApp(manyVoice);
  const room = on.fake.voice.roomId(2);
  const app = await renderApp(`/voice/${room}`, { on });
  await app.user.click(await main().findByRole("button", { name: "Join" }));
  await calls.clock.advance(0);
  await screen.findByRole("toolbar", { name: "Room controls" });
  await app.navigate(`/voice/${room}/invitations`);
  await main().findByRole("list", { name: "Invitations" });
  await growsWhole(
    app,
    itemsOf("Invitations"),
    () => main().getByRole("button", { name: "Show more" }),
    manyVoice.voiceMany?.invited?.length,
  );
});

test("the rooms the reader hosted: Show more adds the older rooms under the newest twenty", async () => {
  const app = await renderApp("/settings/voice", manyVoice);
  await main().findByRole("list", { name: "Rooms you hosted" });
  await growsWhole(app, itemsOf("Rooms you hosted"), () => main().getByRole("button", { name: "Show more" }));
});

test("the accounts whose notifications are held back: Show more adds the older accounts under the newest twenty", async () => {
  const app = await renderApp("/notifications/requests", manyHeld);
  await main().findByRole("button", { name: "Show more" });
  // Each account's row holds its Accept.
  await growsWhole(
    app,
    () => main().getAllByRole("button", { name: "Accept" }),
    () => main().getByRole("button", { name: "Show more" }),
    // kiri, kobo and the more of the sample.
    2 + (manyHeld.heldFrom?.length ?? 0),
  );
});

test("the supporters listed: Show more adds the rest of a plan's supporters under the first twenty", async () => {
  const app = await renderApp("/about/supporters", manySupporters);
  await main().findByRole("list", { name: "Supporter" });
  await growsWhole(app, itemsOf("Supporter"), () => main().getByRole("button", { name: "Show more" }));
});

test("an event's attendees: Show more adds the rest of who is going under the first twenty", async () => {
  const on = fakeApp(manyAttending);
  const app = await renderApp(`/events/${on.fake.events.id("bread-class")}`, { on });
  const going = await main().findByRole("list", { name: /^Going/ });
  const count = Number(/\((\d+)\)/.exec(going.getAttribute("aria-label") ?? words(going))?.[1] ?? Number.NaN);
  // The first page holds twenty answers, of who goes and who may: the rest of who goes comes under Show more.
  const goingShown = itemsOf(/^Going/);
  const maybeShown = itemsOf(/^Maybe/);
  const before = goingShown().length;
  expect(before + maybeShown().length).toBe(page);
  await app.user.click(main().getByRole("button", { name: "Show more" }));
  await waitFor(() => {
    if (goingShown().length !== count)
      throw new Error(`test: ${String(goingShown().length)} of ${String(count)} going`);
  });
  await focusIsOn(app, goingShown, before);
  expect(main().queryAllByRole("button", { name: "Show more" })).toHaveLength(0);
});

test("an event's invitations: Show more adds the rest under the first twenty", async () => {
  const on = fakeApp(manyAttending);
  const app = await renderApp(`/events/${on.fake.events.id("bread-class")}/invitations`, { on });
  await main().findByRole("list", { name: "Invited" });
  await growsWhole(app, itemsOf("Invited"), () => main().getByRole("button", { name: "Show more" }));
});

/** Twenty-five people of this server, each a copy of sam with a name of their own. */
const voters: FakePerson[] = Array.from({ length: 25 }, (_, i) => {
  const n = String(i).padStart(2, "0");
  return {
    ...(scenario.people.find((p) => p.username === "sam") as FakePerson),
    username: `voter${n}`,
    email: `voter${n}@demo.test`,
    displayName: `Voter ${n}`,
  };
});

/** A poll of two dates the reader organizes, whose first date the twenty-five voters can go to. */
const choirPoll: FakeEventPlan = {
  key: "choir-poll",
  organizer: "yuki",
  title: "Choir practice: pick a day",
  description: "Answer the days that work for you.",
  timeZone: "Asia/Tokyo",
  inDays: 10,
  hour: 18,
  hours: 2,
  candidates: [
    { inDays: 10, hour: 18, hours: 2 },
    { inDays: 12, hour: 18, hours: 2 },
  ],
  votes: voters.map((v) => ({ by: v.username, candidate: 0, vote: "yes" })),
};

test("who answered a date of a poll: read once the date is opened, Show more adds the rest under the first twenty", async () => {
  const on = fakeApp({
    seed: { ...scenario, people: [...scenario.people, ...voters], events: [...scenario.events, choirPoll] },
  });
  const ballots = on.fake.script("GET /events/{id}/candidates/{candidate_id}/votes");
  const app = await renderApp(`/events/${on.fake.events.id("choir-poll")}`, { on });
  const [first] = await main().findAllByRole("button", { name: "Show" });
  if (first === undefined) throw new Error("test: no date to open");
  // Who answered a date is read once the reader opens it.
  expect(ballots.seen).toHaveLength(0);
  await app.user.click(first);
  const canGo = await main().findByRole("list", { name: /^Can go/ });
  await growsWhole(
    app,
    itemsOf(/^Can go/),
    () => moreAfter(canGo),
    voters.length,
    () => {
      const list = main().getByRole("list", { name: /^Can go/ });
      return main()
        .queryAllByRole("button", { name: "Show more" })
        .filter((b) => (list.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0);
    },
  );
  expect(ballots.seen).toHaveLength(2);
});

test("who answered a date of a poll counts them all in its name, as an event's attendees do, before Show more", async () => {
  const on = fakeApp({
    seed: { ...scenario, people: [...scenario.people, ...voters], events: [...scenario.events, choirPoll] },
  });
  const app = await renderApp(`/events/${on.fake.events.id("choir-poll")}`, { on });
  const [first] = await main().findAllByRole("button", { name: "Show" });
  if (first === undefined) throw new Error("test: no date to open");
  await app.user.click(first);
  const canGo = await main().findByRole("list", { name: /^Can go/ });
  expect(within(canGo).getAllByRole("listitem")).toHaveLength(page);
  // Twenty-five can go, of whom the first page shows twenty.
  expect(canGo.getAttribute("aria-label") ?? words(canGo)).toBe(`Can go (${String(voters.length)})`);
});

test("the reader's own emoji and their requests each grow under their own Show more", async () => {
  const app = await renderApp("/settings/custom-emojis", {
    viewer: "mio",
    staffServer: { roles: { viewer: ["artist"] }, emoji: { many: 24 } },
  });
  const requests = await main().findByRole("list", { name: "Your requests" });
  const emoji = main().getByRole("list", { name: "Your emoji" });
  const requestsShown = itemsOf("Your requests");
  const emojiShown = itemsOf("Your emoji");
  expect(emojiShown()).toHaveLength(page);

  // The requests' Show more adds requests alone.
  await growsWhole(
    app,
    requestsShown,
    () => moreAfter(requests),
    undefined,
    () => main().queryAllByRole("button", { name: "Show more" }).slice(0, -1),
  );
  expect(emojiShown()).toHaveLength(page);
  // The emoji's own, under them.
  await growsWhole(app, emojiShown, () => moreAfter(emoji));
});

/** Twenty-four warnings that ended, more than a page of the past actions. */
const pastWarnings: AppOptions = {
  settings: {
    moderation: Array.from({ length: 24 }, (_, i) => ({
      kind: "warn" as const,
      reason_category: "violation" as const,
      public_reason: `Warning ${String(24 - i)}: a reply that named another person's employer.`,
      state: "expired" as const,
      issued_at: `-${String(10 + i * 9)}d`,
      expires_at: `-${String(9 + i * 9)}d`,
      appealable: false,
      appeal_deadline: "-120d",
    })),
  },
};

test("the account's past actions: Show more adds the older ones under the newest twenty", async () => {
  const app = await renderApp("/settings/account/status", pastWarnings);
  await main().findByRole("list", { name: "Past actions" });
  await growsWhole(app, itemsOf("Past actions"), () => main().getByRole("button", { name: "Show more" }), 24);
});

test("the blocked accounts: Show more adds the rest under the first twenty", async () => {
  const app = await renderApp("/settings/relationships/blocked", manyBlocked);
  await main().findByRole("button", { name: "Show more" });
  await growsWhole(
    app,
    () => main().getAllByRole("button", { name: "Unblock" }),
    () => main().getByRole("button", { name: "Show more" }),
  );
});

test("a list's members: Show more adds the rest under the first twenty, as many as the list counts", async () => {
  const on = fakeApp(manyMembers);
  const app = await renderApp(`${on.fake.listPath(0)}/members`, { on });
  await main().findByRole("list", { name: "Members" });
  const counted = Number(/(\d+) members/.exec(words(screen.getByRole("main")))?.[1] ?? Number.NaN);
  await growsWhole(app, itemsOf("Members"), () => main().getByRole("button", { name: "Show more" }), counted);
});

test("who reacted to a post: Show more adds the next twenty under the first, the focus on the first it added", async () => {
  const on = fakeApp(manyReactors);
  const app = await renderApp(`${on.fake.postPath("yuki-ridge")}/reactions/${encodeURIComponent("🎉")}`, { on });
  await main().findByRole("button", { name: "Show more" });
  // A reactor is a link to their profile that holds their name, beside the link of their picture.
  const reactors = () =>
    main()
      .getAllByRole("link")
      .filter((link) => words(link) !== "");
  // The screen asks for the most a page of the server's holds, forty, of the 45 its heading counts.
  const counted = Number(/(\d+)$/.exec(words(main().getByRole("heading", { level: 2 })))?.[1] ?? Number.NaN);
  const first = reactors().length;
  expect([first, counted]).toEqual([40, 45]);
  await app.user.click(main().getByRole("button", { name: "Show more" }));
  await waitFor(() => {
    if (reactors().length !== counted) throw new Error(`test: ${String(reactors().length)} reactors are shown`);
  });
  await focusIsOn(app, reactors, first);
  expect(main().queryAllByRole("button", { name: "Show more" })).toHaveLength(0);
});

/** The pager of the window of periods: the first navigation named Pages. */
const periods = () => {
  const [first] = main().getAllByRole("navigation", { name: "Pages" });
  if (first === undefined) throw new Error("test: no window of periods");
  return within(first);
};

/** What the window of periods says it shows. */
const windowSaid = () => periods().getByRole("status").textContent;

test("the public finance: Later is disabled on the window ending this month, Earlier and Later step a window and say it", async () => {
  const app = await renderApp("/about/costs", { finance: { costs: "breakdown", income: "totals" } });
  await main().findByRole("navigation", { name: "Pages" });
  const later = () => periods().getByRole<HTMLButtonElement>("button", { name: "Later months" });
  const earlier = () => periods().getByRole<HTMLButtonElement>("button", { name: "Earlier months" });
  expect(later().disabled).toBe(true);
  const current = windowSaid();
  expect(current).toMatch(/^Showing \w+ \d{4} to \w+ \d{4}\.$/);

  await app.user.click(earlier());
  await waitFor(() => {
    if (windowSaid() === current) throw new Error("test: the window did not move");
  });
  expect(later().disabled).toBe(false);

  await app.user.click(later());
  await waitFor(() => {
    if (windowSaid() !== current) throw new Error(`test: the window says ${windowSaid()}`);
  });
  expect(later().disabled).toBe(true);
  // Later disabled under the press, the focus goes to the other way, not the page.
  await waitFor(() => {
    if (app.focused() !== earlier())
      throw new Error(`test: the focus is on ${app.focused()?.outerHTML.slice(0, 80) ?? "the page"}`);
  });
});

/**
 * The app loaded anew at the address it is at, on the same server: new data, nothing of the first app's cache kept,
 * as a reload of the page.
 */
async function reloaded(app: TestApp): Promise<TestApp> {
  const at = app.path();
  cleanup();
  const watch = watchServer({ server: fakeOrigin, fetch: app.fake.serve, followDevice: false });
  const data = createAppData(
    createClient({ server: fakeOrigin, retry: { disabled: true }, fetch: watch.fetch }),
    undefined,
    watch,
  );
  const on: FakeApp = {
    fake: app.fake,
    watch,
    data: { ...data, connection: { server: fakeOrigin, fetch: watch.fetch } },
    visitor: false,
  };
  return renderApp(at, { on });
}

/** The page the movements' pager says, the last navigation named Pages. */
const movementsPage = () => {
  const last = main().getAllByRole("navigation", { name: "Pages" }).at(-1);
  if (last === undefined) throw new Error("test: no pager of the movements");
  return within(last);
};

/** The rows of the movements, the last table of the screen. */
const movements = () => {
  const table = main().getAllByRole("table").at(-1);
  if (table === undefined) throw new Error("test: no table of the movements");
  return words(table);
};

test("the administrators' income: its window of months and its movements page apart, a reload keeping the movements' page", async () => {
  const app = await renderApp("/admin/finance/income", { viewer: "hana", staffPage: 2 });
  await waitFor(() => {
    if (movementsPage().getByRole("status").textContent !== "Page 1") throw new Error("test: no first page");
  });
  const window = windowSaid();
  const firstMovements = movements();

  // Earlier moves the window and leaves the movements where they are.
  await app.user.click(periods().getByRole("button", { name: "Earlier months" }));
  await waitFor(() => {
    if (windowSaid() === window) throw new Error("test: the window did not move");
  });
  const earlierWindow = windowSaid();
  expect(movementsPage().getByRole("status").textContent).toBe("Page 1");
  expect(movements()).toBe(firstMovements);

  // Next moves the movements and leaves the window where it is.
  await app.user.click(movementsPage().getByRole("button", { name: "Next" }));
  await waitFor(() => {
    if (movementsPage().getByRole("status").textContent !== "Page 2") throw new Error("test: no second page");
  });
  expect(app.path()).toContain("cursor=");
  const secondMovements = movements();
  expect(secondMovements).not.toBe(firstMovements);
  expect(windowSaid()).toBe(earlierWindow);

  // A reload of the address shows the movements' second page again.
  await reloaded(app);
  await waitFor(() => {
    if (movementsPage().getByRole("status").textContent !== "Page 2") throw new Error("test: the reload lost the page");
  });
  await waitFor(() => {
    if (movements() !== secondMovements) throw new Error("test: the reload shows other movements");
  });
});
