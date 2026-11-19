// SPDX-License-Identifier: AGPL-3.0-only

// The voice chat rooms for the listener (ADR-0100 §5, §8, §9, §10; FR-924, FR-928, FR-930), on the fake API: the list
// in the server's order and its next page; a room before joining, who is in it and that joining lists the reader, and
// that joining leaves the room the reader is in; in it, the controls a listener and a speaker have, where the focus
// goes on Join and Leave, a raised hand and leaving, its lines (reconnecting, an action that did not finish, muted by a
// host) spoken from one live region; the call's bar, hidden on the room's own page and shown on every other; why the
// call ended, on the room's page and in a toast elsewhere; a room that ended, one the reader does not see, the page of
// a visitor and the voice chat off; and the ways in, the shell, the hashtag's page, the profile and the notifications.
// The call
// runs on the route test's fakes (test/call.ts); what its states look like is the stories' (voice.stories.tsx), how the
// call ends the feature's (web/app-features/src/call/calls.test.ts).

import { expect, test } from "bun:test";
import { heartbeatInterval, joinRoom } from "@my/app-features";
import { fakeOrigin, type FakeApi, type FakeOptions, type Script } from "@my/e2e/fake";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, words } from "./test/app";
import { callsOnFakes, type FakeCalls } from "./test/call";
import { fakeApp } from "./test/server";

/** The IDs of the rooms the fake's list gives, in its order, asked as a client does (`tag` for a hashtag's). */
async function listedBy(fake: FakeApi, tag?: string): Promise<string[]> {
  const query = tag === undefined ? "" : `?tag=${tag}`;
  const answer = await fake.serve(new Request(`${fakeOrigin}/api/v1/voice/rooms${query}`));
  const page = (await answer.json()) as { items: { id: string }[] };
  return page.items.map((room) => room.id);
}

/** The rooms a list on the screen leads to, in its order, by the IDs of their addresses. */
const roomsIn = (list: HTMLElement) =>
  within(list)
    .getAllByRole("link")
    .map((link) => decodeURIComponent((link.getAttribute("href") ?? "").replace(/^\/voice\//, "")));

/** Renders the app at the room of `at`, given the fake once it stands, on a server of `options`. */
async function atRoom(at: (fake: FakeApi) => string, options: FakeOptions = {}) {
  const on = fakeApp(options);
  const id = at(on.fake);
  const app = await renderApp(`/voice/${id}`, { on });
  return { app, id };
}

/** The people a list of the room names, each as their tile says them. */
const tiles = (list: string) =>
  within(main().getByRole("list", { name: list }))
    .getAllByRole("img")
    .map((tile) => tile.getAttribute("aria-label"));

/** The room's controls, the toolbar of a reader in it. */
const controls = () => within(main().getByRole("toolbar", { name: "Room controls" }));

/** The names of the buttons of the room's controls. */
const offered = () =>
  controls()
    .getAllByRole("button")
    .map((button) => words(button));

/** The bars of the call the shell shows, each a group named by its room: beside the rail, and in a phone's dock. */
const bars = (room: string) => screen.queryAllByRole("group", { name: room });

/** The first bar of the call in `room`; the shell keeps one per width, and no CSS chooses one on a simulation. */
const bar = (room: string) => {
  const [first] = bars(room);
  if (first === undefined) throw new Error(`test: the shell shows no bar of the call in ${room}`);
  return within(first);
};

/** Joins the fake's room at `index` as a screen's Join does, then opens its page. */
async function inRoom(calls: FakeCalls, index: number, options: FakeOptions = {}) {
  const app = await renderApp("/lists", options);
  const id = app.fake.voice.roomId(index);
  const call = await joinRoom(app.data, id);
  await calls.clock.advance(0);
  const room = call.getState().room.name;
  await app.navigate(`/voice/${id}`);
  await main().findByRole("toolbar", { name: "Room controls" });
  return { app, id, room };
}

/**
 * Waits until the screen says `sentence`: as a line of the screen's main, or in a toast of the shell. The room's lines
 * are spoken from one live region of the page, so a visible line is no status of its own.
 */
const says = (sentence: string) =>
  waitFor(() => {
    const said = [words(screen.getByRole("main")), ...screen.queryAllByRole("status").map((status) => words(status))];
    if (!said.some((text) => text.includes(sentence))) throw new Error(`test: the screen says ${JSON.stringify(said)}`);
  });

/** Waits until the screen no longer says `sentence` in its main. */
const saysNoLonger = (sentence: string) =>
  waitFor(() => {
    if (words(screen.getByRole("main")).includes(sentence)) throw new Error(`test: the screen still says ${sentence}`);
  });

/** Waits until the focus is on `element` or inside it, as a reader hears where it is. */
const focusIsIn = (element: () => HTMLElement, what: string) =>
  waitFor(() => {
    const at = document.activeElement;
    if (at === null || !element().contains(at))
      throw new Error(
        `test: the focus is on ${at?.getAttribute("aria-label") ?? at?.textContent ?? "nothing"}, not ${what}`,
      );
  });

test("the list shows the open rooms in the server's order, each leading to its room", async () => {
  const on = fakeApp();
  const order = await listedBy(on.fake);
  await renderApp("/voice", { on });
  expect(order.length).toBeGreaterThan(1);
  expect(roomsIn(main().getByRole("list", { name: "Rooms open now" }))).toEqual(order);
});

test("Show more asks for the list's next page by its cursor", async () => {
  const on = fakeApp();
  on.fake.script("GET /voice/rooms", { times: 1, change: (page) => ({ ...page, next_cursor: "next-rooms" }) });
  const app = await renderApp("/voice", { on });
  const next = app.fake.script("GET /voice/rooms");
  await app.user.click(main().getByRole("button", { name: "Show more" }));
  const asked = await next.arrived();
  expect(asked.url.searchParams.get("cursor")).toBe("next-rooms");
});

test("before joining, the room shows its speakers and listeners, a participant the reader blocks marked, and that joining lists the reader", async () => {
  await atRoom((fake) => fake.voice.roomId(0));
  expect(main().getByRole("heading", { level: 1 }).textContent).toBe("Morning coffee chat");
  expect(tiles("Speakers")).toEqual(["Sam Rivera, Host", "Mio 澪, Co-host", "Hana 花"]);
  // The one the reader blocks stays in the list, marked.
  expect(tiles("Listeners")).toEqual(["Marco", "ليلى Layla, Hand raised", "Ren 蓮", "Kai", "QuickDeals, Blocked"]);
  expect(words(screen.getByRole("main"))).toContain(
    "When you join, your name shows in the list of participants for everyone who can see this room.",
  );
  expect(main().getByRole("button", { name: "Join" })).toBeDefined();
  expect(words(screen.getByRole("main"))).not.toContain("when you join this one");
});

test("joining from a room's page while in another says that joining leaves it, and joins this one", async () => {
  const calls = callsOnFakes();
  const { app } = await inRoom(calls, 0);
  const other = app.fake.voice.roomId(1);
  await app.navigate(`/voice/${other}`);
  expect(words(screen.getByRole("main"))).toContain("You leave Morning coffee chat when you join this one.");
  await app.user.click(main().getByRole("button", { name: "Join" }));
  await calls.clock.advance(0);
  await waitFor(() => {
    if (app.fake.voice.joined()?.room !== other) throw new Error("test: the reader is not in the other room");
  });
  // The focus goes on to the room's controls.
  await focusIsIn(() => main().getByRole("toolbar", { name: "Room controls" }), "the room's controls");
});

test("a listener in a room that asks for a hand has Raise hand, React and Leave, no microphone, and is in the listeners as You", async () => {
  const calls = callsOnFakes();
  const { app, room } = await inRoom(calls, 0);
  expect(offered()).toEqual(["Raise hand", "React", "Leave"]);
  expect(tiles("Listeners")).toContain("Yuki 雪, You");
  // On the room's own page the call's bar is not shown; on another screen it is, with no microphone for a listener.
  expect(bars(room)).toHaveLength(0);
  await app.navigate("/lists");
  expect(bar(room).queryAllByRole("button", { name: /^(Mute|Unmute)$/ })).toHaveLength(0);
  expect(bar(room).getAllByRole("button", { name: "Leave" })).toHaveLength(1);
});

test("Raise hand raises the reader's hand on the server, and Lower hand takes it down", async () => {
  const calls = callsOnFakes();
  const { app } = await inRoom(calls, 0);
  const raise = app.fake.script("POST /voice/rooms/{id}/hand");
  await app.user.click(controls().getByRole("button", { name: "Raise hand" }));
  await raise.served();
  const lower = await controls().findByRole("button", { name: "Lower hand" });
  expect(tiles("Listeners")).toContain("Yuki 雪, You, Hand raised");
  const lowered = app.fake.script("DELETE /voice/rooms/{id}/hand");
  await app.user.click(lower);
  await lowered.served();
  await controls().findByRole("button", { name: "Raise hand" });
});

test("in a room where everyone may speak, Speak makes the reader a speaker with four controls in one row, and a microphone in the call's bar elsewhere", async () => {
  const calls = callsOnFakes();
  const { app, room } = await inRoom(calls, 1);
  expect(offered()).toEqual(["Speak", "React", "Leave"]);
  const speak = app.fake.script("POST /voice/rooms/{id}/speaker");
  await app.user.click(controls().getByRole("button", { name: "Speak" }));
  await speak.served();
  await waitFor(() => {
    if (!tiles("Speakers").some((tile) => tile?.startsWith("Yuki 雪, You") === true))
      throw new Error(`test: the speakers are ${JSON.stringify(tiles("Speakers"))}`);
  });
  const [mic, ...rest] = offered();
  expect(mic).toMatch(/^(Mute|Unmute)$/);
  expect(rest).toEqual(["Stop speaking", "React", "Leave"]);
  expect(bars(room)).toHaveLength(0);
  await app.navigate("/lists");
  expect(bar(room).getAllByRole("button", { name: /^(Mute|Unmute)$/ })).toHaveLength(1);
  expect(bar(room).getAllByRole("button", { name: "Leave" })).toHaveLength(1);
});

test("Leave ends the reader's place in the room, and the page offers Join again, with the focus on it", async () => {
  const calls = callsOnFakes();
  const { app, room } = await inRoom(calls, 0);
  await app.user.click(controls().getByRole("button", { name: "Leave" }));
  await calls.clock.advance(0);
  await main().findByRole("button", { name: "Join" });
  await focusIsIn(() => main().getByRole("button", { name: "Join" }), "Join");
  expect(app.fake.voice.joined()).toBeNull();
  await app.navigate("/lists");
  expect(bars(room)).toHaveLength(0);
});

test("while the room's stream comes back the room says it reconnects and the call's bar elsewhere is busy, and neither once it is back", async () => {
  const calls = callsOnFakes();
  const { app, room } = await inRoom(calls, 0);
  const reconnecting = "The connection was lost. Reconnecting…";
  app.fake.voice.drop();
  await calls.clock.advance(0);
  await says(reconnecting);
  await calls.clock.advance(1_000);
  await saysNoLonger(reconnecting);

  await app.navigate("/lists");
  expect(bars(room).some((shown) => shown.getAttribute("aria-busy") === "true")).toBe(false);
  app.fake.voice.drop();
  await calls.clock.advance(0);
  await waitFor(() => {
    if (!bars(room).every((shown) => shown.getAttribute("aria-busy") === "true"))
      throw new Error("test: the call's bar is not busy");
  });
  await calls.clock.advance(1_000);
  await waitFor(() => {
    if (bars(room).some((shown) => shown.getAttribute("aria-busy") === "true"))
      throw new Error("test: the call's bar is still busy");
  });
});

test("an action that did not finish says which, and leaves its control as it was", async () => {
  const calls = callsOnFakes();
  const { app } = await inRoom(calls, 1);
  const refused = app.fake.script("POST /voice/rooms/{id}/speaker", { fail: 500 });
  await app.user.click(controls().getByRole("button", { name: "Speak" }));
  await refused.arrived();
  await says("Speaking did not finish. Try again.");
  expect(offered()).toEqual(["Speak", "React", "Leave"]);
});

test("the room's lines are spoken from one polite live region, there and quiet before anything is said", async () => {
  const calls = callsOnFakes();
  const { app } = await inRoom(calls, 1);
  const [region, ...more] = main().getAllByRole("status");
  if (region === undefined) throw new Error("test: the room has no live region");
  expect({ more: more.length, said: words(region) }).toEqual({ more: 0, said: "" });
  app.fake.voice.drop();
  await calls.clock.advance(0);
  await waitFor(() => {
    if (!words(region).includes("The connection was lost. Reconnecting…"))
      throw new Error(`test: the live region says ${words(region)}`);
  });
  await calls.clock.advance(1_000);
  app.fake.script("POST /voice/rooms/{id}/speaker", { fail: 500 });
  await app.user.click(controls().getByRole("button", { name: "Speak" }));
  await waitFor(() => {
    if (!words(region).includes("Speaking did not finish. Try again."))
      throw new Error(`test: the live region says ${words(region)}`);
  });
  // The same region, still the only one: it was kept mounted.
  expect(main().getAllByRole("status")).toEqual([region]);
});

test("a speaker a host muted is told who did, and the microphone offers Unmute", async () => {
  const calls = callsOnFakes();
  const { app } = await inRoom(calls, 1);
  const speak = app.fake.script("POST /voice/rooms/{id}/speaker");
  await app.user.click(controls().getByRole("button", { name: "Speak" }));
  await speak.served();
  await controls().findByRole("button", { name: /^(Mute|Unmute)$/ });
  app.fake.voice.send({ event: "muted", data: { by: app.fake.accountId("hana") } });
  await calls.clock.advance(0);
  await says("Hana 花 turned your microphone off. You can turn it on again.");
  expect(controls().getByRole("button", { name: /^(Mute|Unmute)$/ }).textContent).toBe("Unmute");
});

test("a call whose session the server no longer holds ends as lost, and the room's page says so", async () => {
  const calls = callsOnFakes();
  const { app, room } = await inRoom(calls, 0);
  app.fake.script("POST /voice/rooms/{id}/heartbeat", { fail: 404 });
  await calls.clock.advance(heartbeatInterval);
  await says("The connection to Morning coffee chat was lost.");
  expect(bars(room)).toHaveLength(0);
});

/** The ends the server tells the reader of, and what the room's page says of each. */
const ends: readonly (readonly [string, Parameters<FakeApi["voice"]["send"]>[0], string])[] = [
  [
    "a host removed them",
    { event: "removed", data: { reason: "removed" } },
    "A host removed you from Morning coffee chat. You cannot join it again.",
  ],
  [
    "another device took the call",
    { event: "replaced", data: { reason: "replaced" } },
    "You joined Morning coffee chat on another device, so you left it on this one.",
  ],
  ["the host ended the room", { event: "ended", data: { reason: "host" } }, "The host ended Morning coffee chat."],
];

test.each(ends)("the room's page says why the call ended when %s", async (_, end, sentence) => {
  const calls = callsOnFakes();
  const { app, room } = await inRoom(calls, 0);
  app.fake.voice.send(end);
  await calls.clock.advance(0);
  await says(sentence);
  await app.navigate("/lists");
  expect(bars(room)).toHaveLength(0);
});

test("a room that ends while the reader is on its page shows as gone at once, and the rail drops it", async () => {
  const calls = callsOnFakes();
  const { app, id } = await inRoom(calls, 0);
  const leadsToIt = () => screen.queryAllByRole("link").filter((link) => link.getAttribute("href") === `/voice/${id}`);
  expect(leadsToIt().length).toBeGreaterThan(0);
  app.fake.voice.send({ event: "ended", data: { reason: "host" } });
  await calls.clock.advance(0);
  await says("The host ended Morning coffee chat.");
  expect(
    main()
      .getAllByRole("heading")
      .map((heading) => words(heading)),
  ).toContain("Morning coffee chat");
  expect(main().queryAllByRole("button", { name: "Join" })).toHaveLength(0);
  expect(main().queryAllByRole("list")).toHaveLength(0);
  expect(main().queryAllByRole("toolbar")).toHaveLength(0);
  await waitFor(() => {
    if (leadsToIt().length > 0) throw new Error("test: a link still leads to the room that ended");
  });
});

test("on another screen a toast says that the host ended the room, and Open the room leads to it", async () => {
  const calls = callsOnFakes();
  const { app, id } = await inRoom(calls, 0);
  await app.navigate("/lists");
  app.fake.voice.send({ event: "ended", data: { reason: "host" } });
  await calls.clock.advance(0);
  await says("The host ended Morning coffee chat.");
  await app.user.click(screen.getByRole("button", { name: "Open the room" }));
  await app.arrived(`/voice/${id}`);
});

test("a room that ended says so, and one the reader does not see says it is not open to them", async () => {
  await atRoom((fake) => fake.voice.endedRoomId());
  await says("This room has ended.");
  expect(main().queryAllByRole("button", { name: "Join" })).toHaveLength(0);
});

test("a room the reader does not see says that it is not open or not theirs to see, and offers no Join", async () => {
  await atRoom((fake) => fake.voice.hiddenRoomId());
  await says("This room is not open, or you cannot see it.");
  expect(main().queryAllByRole("button", { name: "Join" })).toHaveLength(0);
});

test("a visitor learns that only this server's people join, with Sign in to join, and nothing of the room", async () => {
  await atRoom((fake) => fake.voice.roomId(0), { visitor: true });
  const said = words(screen.getByRole("main"));
  expect(said).toContain("Only people with an account on this server can join its rooms.");
  expect(main().getByRole("button", { name: "Sign in to join" })).toBeDefined();
  expect(said).not.toContain("Morning coffee chat");
  expect(main().queryAllByRole("list")).toHaveLength(0);
});

test("a visitor of a server that shows its rooms' summaries sees the name, the host and the description, and no participant", async () => {
  await atRoom((fake) => fake.voice.roomId(0), { visitor: true, voiceSummaries: true });
  expect(main().getByRole("heading", { level: 1 }).textContent).toBe("Morning coffee chat");
  const said = words(screen.getByRole("main"));
  expect(said).toContain("Hosted by Sam Rivera");
  expect(said).toContain("Bread, bikes and whatever the week brought.");
  expect(main().getByRole("button", { name: "Sign in to join" })).toBeDefined();
  expect(main().queryAllByRole("list")).toHaveLength(0);
});

test("a visitor of a room that ended sees that it ended, with its name where the server shows summaries", async () => {
  await atRoom((fake) => fake.voice.endedRoomId(), { visitor: true, voiceSummaries: true });
  expect(main().getByRole("heading", { level: 1 }).textContent).toBe("Late night radio");
  await says("This room has ended.");
});

test("a visitor's page that could not be loaded offers Try again, which loads it", async () => {
  const on = fakeApp({ visitor: true });
  const id = on.fake.voice.roomId(0);
  const failing = on.fake.script("GET /voice/rooms/{id}/summary", { fail: 500 });
  const app = await renderApp(`/voice/${id}`, { on });
  const again = await main().findByRole("button", { name: "Try again" });
  failing.end();
  await app.user.click(again);
  await main().findByRole("button", { name: "Sign in to join" });
});

test("with the voice chat off, a room's page says it is not available and offers no Join", async () => {
  await atRoom((fake) => fake.voice.roomId(0), { voiceOff: true });
  await says("Voice chat is not available on this server.");
  expect(main().queryAllByRole("button", { name: "Join" })).toHaveLength(0);
});

test("with the voice chat off, the list says so and reads no rooms", async () => {
  const on = fakeApp({ voiceOff: true });
  const reads = on.fake.script("GET /voice/rooms");
  const app = await renderApp("/voice", { on });
  await waitFor(() => {
    if (!/[Vv]oice chat is (off|not available)/.test(words(screen.getByRole("main"))))
      throw new Error(`test: the list says ${words(screen.getByRole("main"))}`);
  });
  await waitFor(() => {
    if (app.data.queryClient.isFetching() > 0) throw new Error("test: the app still reads");
  });
  expect(main().queryAllByRole("button", { name: "Try again" })).toHaveLength(0);
  expect(main().queryAllByRole("alert")).toHaveLength(0);
  expect(reads.seen).toHaveLength(0);
});

test("the shell leads to the rooms, and not with the voice chat off", async () => {
  const leads = () => screen.queryAllByRole("link").filter((link) => link.getAttribute("href") === "/voice").length;
  await renderApp("/lists");
  await waitFor(() => {
    if (leads() === 0) throw new Error("test: nothing leads to the rooms");
  });
  cleanup();
  const off = await renderApp("/lists", { voiceOff: true });
  await waitFor(() => {
    if (off.data.queryClient.isFetching() > 0) throw new Error("test: the shell still reads");
  });
  expect(leads()).toBe(0);
});

test("a hashtag's page lists that tag's open rooms, in the server's order", async () => {
  const on = fakeApp();
  const order = await listedBy(on.fake, "coffee");
  await renderApp("/tags/coffee", { on });
  expect(order.length).toBeGreaterThan(0);
  expect(roomsIn(await main().findByRole("list", { name: "Voice chat rooms" }))).toEqual(order);
});

/** A notification as the fake's page of them holds it. */
type Notification = Parameters<NonNullable<Script<"GET /notifications">["change"]>>[0]["items"][number];

test("the notifications of a room's start and of an invitation to it lead to the room", async () => {
  const on = fakeApp();
  const id = on.fake.voice.roomId(0);
  const answer = await on.fake.serve(new Request(`${fakeOrigin}/api/v1/voice/rooms/${id}`));
  const room = (await answer.json()) as NonNullable<Notification["voice_room"]>;
  const host = room.host;
  const at = new Date(Date.now() - 60_000).toISOString();
  on.fake.script("GET /notifications", {
    change: (page) => ({
      ...page,
      items: [
        {
          id: "voice-started",
          type: "voice_room_started" as const,
          created_at: at,
          account: host,
          voice_room_id: id,
          voice_room: room,
        },
        {
          id: "voice-invited",
          type: "voice_room_invited" as const,
          created_at: at,
          account: host,
          voice_room_id: id,
          voice_room: room,
        },
        ...page.items,
      ],
    }),
  });
  await renderApp("/notifications", { on });
  await waitFor(() => {
    const leading = main()
      .queryAllByRole("link")
      .filter((link) => link.getAttribute("href") === `/voice/${id}`);
    if (leading.length < 2) throw new Error(`test: ${String(leading.length)} links lead to the room`);
  });
});

test.each([
  ["hosts", "navigated to", "sam", "Hosting now"],
  ["speaks in", "navigated to", "hana", "Speaking now"],
  ["hosts", "opened as the app's first screen", "sam", "Hosting now"],
  ["speaks in", "opened as the app's first screen", "hana", "Speaking now"],
] as const)("the profile of an account that %s a room leads to it, %s", async (_, how, username, role) => {
  const on = fakeApp();
  const entry = on.fake.voice.entryOf(on.fake.accountId(username));
  if (entry === undefined) throw new Error(`test: ${username} is in no room on the fake`);
  if (how === "navigated to") {
    const app = await renderApp("/lists", { on });
    await app.navigate(`/@${username}`);
  } else await renderApp(`/@${username}`, { on });
  const link = await main().findByRole("link", { name: new RegExp(entry.name) });
  expect(link.getAttribute("href")).toBe(`/voice/${entry.id}`);
  expect(words(link)).toContain(role);
});
