// SPDX-License-Identifier: AGPL-3.0-only

// A call the reader is in, as the shell shows it on every screen: its bar, whose microphone mutes and unmutes, stays
// while the reader moves between screens, and a newer build of the app that waits is not offered meanwhile. What the
// bar's parts are is the design system's (shell.dom.test.tsx), what the store tells the voice feature the call's
// (web/app-features/src/call/store.test.ts), and when the toast shows the notices' (shell/notices.dom.test.tsx).
// A call joined on the fake API is one call held outside every screen (ADR-0101 §7): its audio, its stream and its
// heartbeat are the same across the screens, a change of the width and the app's unmount, and only leaving, the
// server's removed, replaced and ended, and the page going away end it. How each of those ends it, and the stream, the
// heartbeat and the grant on their own, are the feature's (web/app-features/src/call/calls.test.ts).

import { expect, onTestFinished, test } from "bun:test";
import { closeSession } from "@my/app-data";
import { currentCall, enterCall, heartbeatInterval, joinCall, lastCallEnd, leaveCall } from "@my/app-features";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import { renderApp, type TestApp } from "./test/app";
import { callsOnFakes, type FakeCalls } from "./test/call";
import { fakeApp } from "./test/server";
import { changingWidth, tabBar } from "./test/viewport";

test("the bar of a call stays across the screens, its microphone mutes and unmutes, and no toast offers a newer build meanwhile", async () => {
  enterCall({
    roomId: "morning-coffee",
    room: "Morning coffee chat",
    host: "Sam Rivera",
    participants: 12,
    muted: false,
  });
  onTestFinished(leaveCall);
  const on = fakeApp();
  on.watch.offerUpdate();
  const app = await renderApp("/lists", { on });
  /**
   * The call's bar, a group named by its room, and whether its microphone is muted. The shell keeps a bar per width
   * (beside the rail, and in a phone's dock), and no CSS chooses one on a simulation: the first of them.
   */
  const bar = () => {
    const [first] = screen.getAllByRole("group", { name: "Morning coffee chat" });
    if (first === undefined) throw new Error("test: the shell shows no bar of the call");
    return within(first);
  };
  // The microphone's button is named by what a press does now: Mute, and Unmute while muted.
  const mic = () => bar().getByRole("button", { name: /^(Mute|Unmute)$/ });
  const becomes = (name: string) =>
    waitFor(() => {
      if (mic().textContent !== name) throw new Error(`the microphone's button reads ${mic().textContent}`);
    });

  expect(mic().textContent).toBe("Mute");
  expect(screen.queryAllByRole("button", { name: "Update" })).toHaveLength(0);

  await app.user.click(mic());
  await becomes("Unmute");
  await app.navigate("/notifications/follow_requests");
  expect(mic().textContent).toBe("Unmute");

  await app.user.click(mic());
  await becomes("Mute");
  await app.navigate("/lists");
  expect(mic().textContent).toBe("Mute");
  expect(screen.queryAllByRole("button", { name: "Update" })).toHaveLength(0);

  // Once the call is left, the bar goes and the newer build is offered.
  await app.user.click(bar().getByRole("button", { name: "Leave" }));
  expect(await screen.findByRole("button", { name: "Update" })).toBeDefined();
  expect(screen.queryAllByRole("group", { name: "Morning coffee chat" })).toHaveLength(0);
});

/** The bars of the call the shell shows, each a group named by its room: beside the rail, and in a phone's dock. */
const bars = (room: string) => screen.queryAllByRole("group", { name: room });

/** Waits until the shell shows the bar of the call in `room`, or shows none when `shown` is false. */
const barShown = (room: string, shown = true) =>
  waitFor(() => {
    if (bars(room).length > 0 !== shown)
      throw new Error(`test: the bar of the call in ${room} is not ${String(shown)}`);
  });

/** Renders the app at a light screen, and joins the fake's room at `index` as a screen's Join does. */
async function inACall(calls: FakeCalls, index = 0) {
  const app = await renderApp("/lists");
  const call = await joinCall(app.data, app.fake.voice.roomId(index));
  await calls.clock.advance(0);
  const room = call.getState().room.name;
  await barShown(room);
  return { app, call, room };
}

/** Moves the clock a heartbeat on: one heartbeat goes while the call lasts. */
const aBeat = (calls: FakeCalls) => calls.clock.advance(heartbeatInterval);

test("a call is the same call, with the same audio, stream and heartbeat, on every screen, at either width and with the app unmounted", async () => {
  const width = changingWidth();
  const calls = callsOnFakes();
  const { app, call, room } = await inACall(calls);
  const { audio, stream, heartbeat, session } = call;
  const beats = app.fake.script("POST /voice/rooms/{id}/heartbeat");
  /** What says the call is the one joined, live, with its first stream; `where` names the step that failed. */
  const state = (where: string) => ({
    where,
    current: currentCall() === call,
    audio: call.audio === audio && audio === calls.sessions[0],
    stream: call.stream === stream && !stream.aborted,
    heartbeat: call.heartbeat === heartbeat,
    session: call.session,
    phase: call.getState().phase,
    ended: call.ended,
    sessions: calls.sessions.length,
    streamsOpened: app.fake.voice.opened(),
    unloadHeld: calls.holds(),
    bar: bars(room).length > 0,
  });
  /** What `state` says of the call that goes on, with the shell's bar `shown`. */
  const same = (where: string, shown = true): ReturnType<typeof state> => ({
    where,
    current: true,
    audio: true,
    stream: true,
    heartbeat: true,
    session,
    phase: "live",
    ended: false,
    sessions: 1,
    streamsOpened: 1,
    unloadHeld: 1,
    bar: shown,
  });
  expect(state("joined")).toEqual(same("joined"));

  for (const to of ["/notifications/follow_requests", "/settings", "/bookmarks"]) {
    await app.navigate(to);
    await aBeat(calls);
    expect(state(to)).toEqual(same(to));
  }
  await app.back();
  await aBeat(calls);
  expect(state("back")).toEqual(same("back"));

  // From a PC's width to a phone's and back, as a window resized or a phone turned; that the app took the width is
  // what its menus say: a sheet, whose head ends with Close, on a phone.
  const menuIsASheet = async () => {
    await app.user.click(within(tabBar()).getByRole("button", { name: /^Menu(?: \d+ unread in direct messages)?$/ }));
    await screen.findByRole("menu", { name: "Menu" });
    const sheet = screen.queryAllByRole("button", { name: "Close" }).length > 0;
    await app.user.keyboard("{Escape}");
    return sheet;
  };
  width.phone(true);
  expect(await menuIsASheet()).toBe(true);
  await app.navigate("/lists");
  await aBeat(calls);
  expect(state("a phone's width")).toEqual(same("a phone's width"));
  width.phone(false);
  expect(await menuIsASheet()).toBe(false);
  await app.navigate("/notifications/follow_requests");
  await aBeat(calls);
  expect(state("a PC's width")).toEqual(same("a PC's width"));

  // No component holds the call: with the whole app unmounted it goes on, and still beats.
  cleanup();
  await aBeat(calls);
  expect(state("unmounted")).toEqual(same("unmounted", false));

  expect(beats.seen.map((beat) => beat.body)).toEqual(Array.from({ length: 7 }, () => ({ session })));
  // The call has not ended: a join forgets the end of the call before it, whatever test left one.
  expect(lastCallEnd()).toBeNull();
});

/** The ways a call is ended for the reader, each with what the end says. */
const endings: readonly (readonly [
  string,
  (on: { app: TestApp; room: string; calls: FakeCalls }) => unknown,
  { kind: string; reason?: string },
])[] = [
  [
    "leaving from the shell's bar",
    async ({ app, room }) => {
      const [bar] = bars(room);
      if (bar === undefined) throw new Error("test: the shell shows no bar of the call");
      await app.user.click(within(bar).getByRole("button", { name: "Leave" }));
    },
    { kind: "left" },
  ],
  [
    "the server's removed",
    ({ app }) => {
      app.fake.voice.send({ event: "removed", data: { reason: "removed" } });
    },
    { kind: "removed", reason: "removed" },
  ],
  [
    "the server's replaced",
    ({ app }) => {
      app.fake.voice.send({ event: "replaced", data: { reason: "replaced" } });
    },
    { kind: "replaced", reason: "replaced" },
  ],
  [
    "the server's ended",
    ({ app }) => {
      app.fake.voice.send({ event: "ended", data: { reason: "host" } });
    },
    { kind: "ended", reason: "host" },
  ],
  [
    "the page going away",
    ({ calls }) => {
      calls.hidePage();
    },
    { kind: "unloaded" },
  ],
];

test.each(endings)(
  "a call that went on across the screens ends once by %s: its audio, stream, heartbeat and bar go",
  async (_, end, says) => {
    const calls = callsOnFakes();
    const { app, call, room } = await inACall(calls);
    const ends: unknown[] = [];
    call.onEnd((how) => ends.push(how));
    await app.navigate("/notifications/follow_requests");
    await aBeat(calls);
    expect(call.ended).toBe(false);
    const beats = app.fake.script("POST /voice/rooms/{id}/heartbeat");

    await end({ app, room, calls });
    await calls.clock.advance(0);
    await barShown(room, false);
    expect(ends).toHaveLength(1);
    expect(ends[0]).toMatchObject(says);
    expect(call.getState().end).toMatchObject(says);
    expect(lastCallEnd()).toMatchObject(says);
    expect({
      current: currentCall(),
      phase: call.getState().phase,
      audioClosed: calls.sessions[0]?.closed(),
      streamAborted: call.stream.aborted,
      unloadHeld: calls.holds(),
      // The server holds the reader in no room: the leave reached it, or it ended the session itself.
      onTheServer: app.fake.voice.joined(),
    }).toEqual({
      current: null,
      phase: "ended",
      audioClosed: true,
      streamAborted: true,
      unloadHeld: 0,
      onTheServer: null,
    });

    // Nothing beats for it any longer, no screen brings its bar back, and nothing ends it a second time.
    await app.navigate("/lists");
    await calls.clock.advance(heartbeatInterval * 3);
    expect(beats.seen).toHaveLength(0);
    expect(bars(room)).toHaveLength(0);
    calls.hidePage();
    await call.leave().catch(() => undefined);
    await calls.clock.advance(0);
    expect(ends).toHaveLength(1);
  },
);

test("joining another room ends the call once the server took the join, and the bar shows the new room", async () => {
  const calls = callsOnFakes();
  const { app, call, room } = await inACall(calls);
  await app.navigate("/notifications/follow_requests");
  const other = await joinCall(app.data, app.fake.voice.roomId(1));
  await calls.clock.advance(0);
  const otherRoom = other.getState().room.name;
  await barShown(otherRoom);
  expect(otherRoom).not.toBe(room);
  expect(bars(room)).toHaveLength(0);
  expect(call.getState().end).toMatchObject({ kind: "switched" });
  expect({ current: currentCall() === other, firstClosed: calls.sessions[0]?.closed(), held: calls.holds() }).toEqual({
    current: true,
    firstClosed: true,
    held: 1,
  });
});

test("the reader's session ending ends the call, and its bar goes", async () => {
  const calls = callsOnFakes();
  const { app, call, room } = await inACall(calls);
  await closeSession(app.data);
  await calls.clock.advance(0);
  expect(call.getState().end).toMatchObject({ kind: "signed_out" });
  expect(currentCall()).toBeNull();
  expect(calls.holds()).toBe(0);
  await barShown(room, false);
});
