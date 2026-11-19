// SPDX-License-Identifier: AGPL-3.0-only

// The reader's call, on the fake API and a fake RoomSession, with a clock the test moves: one call held outside every
// screen, its stream opened again on the same session, the heartbeat and the grant, and each way a call ends, once.

import { afterEach, beforeEach, expect, test } from "bun:test";
import { closeSession, createAppData, type AppData } from "@my/app-data";
import { createFakeApi, fakeOrigin, type FakeApi } from "@my/e2e/fake";
import { createClient } from "@mycroblog/mykit";
import { fakeCallRuntime, settle, type FakeCallRuntime } from "../test/call";
import { currentCall, inCall, joinCall, lastCallEnd, runCallsOn } from "./calls";
import { callShown, leaveCall, muteCall } from "./store";
import { grantRenewalLead, heartbeatInterval, heartbeatTtl } from "./voice-call";

type Sent = { readonly line: string; readonly keepalive: boolean };

/** The app's data on a fake API, with what it sent. */
function server(): { fake: FakeApi; data: AppData; sent: Sent[] } {
  const fake = createFakeApi();
  const sent: Sent[] = [];
  const api = createClient({
    server: fakeOrigin,
    retry: { disabled: true },
    fetch: (request) => {
      const url = new URL(request.url);
      sent.push({ line: `${request.method} ${url.pathname.replace("/api/v1", "")}`, keepalive: request.keepalive });
      return fake.serve(request);
    },
  });
  return { fake, data: createAppData(api), sent };
}

let on: FakeCallRuntime;
let restore: () => void;

beforeEach(() => {
  on = fakeCallRuntime();
  restore = runCallsOn(on.runtime);
});

afterEach(async () => {
  await currentCall()
    ?.leave()
    .catch(() => undefined);
  currentCall()?.unload();
  restore();
});

/** Joins the fake's first room and lets its stream open. */
async function joined() {
  const s = server();
  const roomId = s.fake.voice.roomId(0);
  const call = await joinCall(s.data, roomId);
  await on.clock.advance(0);
  return { ...s, roomId, call };
}

test("a join makes one call, which a second join of the room gives again, and its stream opens on its session", async () => {
  const s = server();
  const roomId = s.fake.voice.roomId(0);
  const [first, second] = await Promise.all([joinCall(s.data, roomId), joinCall(s.data, roomId)]);
  expect(first).toBe(second);
  expect(await joinCall(s.data, roomId)).toBe(first);
  await on.clock.advance(0);
  expect(s.sent.filter((r) => r.line.endsWith("/participation"))).toHaveLength(1);
  expect(on.sessions).toHaveLength(1);
  expect(first.getState().phase).toBe("live");
  expect(first.session).toBe(s.fake.voice.joined()?.session ?? "");
  expect(on.sessions[0]?.options.session).toBe(first.session);
  expect(on.holds()).toBe(1);
  expect(inCall()).toBe(true);
});

test("the stream that drops opens again on the same session, and the call, its audio and its heartbeat stay", async () => {
  const { fake, call } = await joined();
  const { audio, stream, heartbeat, session } = call;
  fake.voice.drop();
  await on.clock.advance(0);
  expect(call.getState().phase).toBe("reconnecting");
  await on.clock.advance(1_000);
  expect(fake.voice.opened()).toBe(2);
  expect(call.getState().phase).toBe("live");
  expect(call.ended).toBe(false);
  expect(currentCall()).toBe(call);
  expect([call.audio, call.stream, call.heartbeat, call.session]).toEqual([audio, stream, heartbeat, session]);
});

test("a heartbeat goes every 15 seconds, and none through for 45 seconds ends the call as lost", async () => {
  const { fake, call } = await joined();
  const beats = fake.script("POST /voice/rooms/{id}/heartbeat", { fail: 503 });
  await on.clock.advance(heartbeatInterval * 2);
  expect(beats.seen).toHaveLength(2);
  expect(call.ended).toBe(false);
  await on.clock.advance(heartbeatTtl - heartbeatInterval * 2);
  expect(call.getState().end).toEqual({ kind: "lost", reason: "heartbeat" });
  expect(lastCallEnd()).toEqual({ kind: "lost", reason: "heartbeat" });
  expect(on.sessions[0]?.closed()).toBe(true);
  expect(call.stream.aborted).toBe(true);
  expect(on.holds()).toBe(0);
  await on.clock.advance(heartbeatInterval * 2);
  expect(beats.seen).toHaveLength(3);
});

test("a heartbeat the server answers with 404 ends the call at once", async () => {
  const { fake, call } = await joined();
  fake.script("POST /voice/rooms/{id}/heartbeat", { fail: 404 });
  await on.clock.advance(heartbeatInterval);
  expect(call.getState().end).toEqual({ kind: "lost", reason: "session" });
});

test("the grant is taken again 10 minutes before it expires, and given to the audio", async () => {
  const s = server();
  // A grant that expires in 11 minutes: it is taken again in one.
  s.fake.script("POST /voice/rooms/{id}/participation", {
    change: (join) => ({
      ...join,
      grant: { ...join.grant, expires_at: new Date(on.clock.now() + grantRenewalLead + 60_000).toISOString() },
    }),
  });
  const grants = s.fake.script("POST /voice/rooms/{id}/grant");
  const call = await joinCall(s.data, s.fake.voice.roomId(0));
  await on.clock.advance(59_000);
  expect(grants.seen).toHaveLength(0);
  await on.clock.advance(2_000);
  expect(grants.seen).toHaveLength(1);
  expect(grants.seen[0]?.body).toEqual({ session: call.session });
  expect(on.sessions[0]?.renewed).toHaveLength(1);
  expect(call.ended).toBe(false);
});

test("removed, replaced and ended each end the call once, with the server's reason", async () => {
  for (const event of ["removed", "replaced", "ended"] as const) {
    const { fake, call } = await joined();
    const ends: unknown[] = [];
    call.onEnd((end) => ends.push(end));
    const reason = event === "removed" ? "removed" : event === "replaced" ? "replaced" : "host";
    fake.voice.send({ event, data: { reason } });
    await on.clock.advance(0);
    expect(ends).toEqual([{ kind: event, reason }]);
    expect(currentCall()).toBeNull();
    // Nothing after the end ends it again.
    await call.leave();
    call.unload();
    expect(ends).toHaveLength(1);
  }
});

test("leaving sends the leave and ends the call; the shell's bar goes with it", async () => {
  const { sent, call, roomId } = await joined();
  await call.leave();
  expect(sent.at(-1)?.line).toBe(`DELETE /voice/rooms/${roomId}/participation`);
  expect(call.getState().end?.kind).toBe("left");
  expect(currentCall()).toBeNull();
});

test("a host the server keeps in (they did not choose who hosts next) stays in the call", async () => {
  const { fake, call } = await joined();
  fake.script("DELETE /voice/rooms/{id}/participation", {
    fail: { type: "/api/v1/problems/voice-host-must-choose", title: "Choose", status: 409 },
  });
  expect(await call.leave().catch((error: unknown) => error)).toMatchObject({ status: 409 });
  await settle();
  expect(call.ended).toBe(false);
  expect(currentCall()).toBe(call);
});

test("leaving from the shell's bar leaves the call", async () => {
  const { call } = await joined();
  leaveCall();
  await settle();
  expect(call.getState().end?.kind).toBe("left");
});

test("the page going away sends the leave with keepalive and ends the call", async () => {
  const { sent, call, roomId } = await joined();
  on.hidePage();
  await settle();
  expect(sent.at(-1)).toEqual({ line: `DELETE /voice/rooms/${roomId}/participation`, keepalive: true });
  expect(call.getState().end?.kind).toBe("unloaded");
});

test("the session of the app ending ends the call, with nothing more asked of the server", async () => {
  const { data, sent, call } = await joined();
  const before = sent.length;
  await closeSession(data);
  await on.clock.advance(heartbeatInterval);
  expect(call.getState().end?.kind).toBe("signed_out");
  expect(sent.length).toBe(before);
});

test("joining another room ends the call of the first once the server took the join", async () => {
  const { data, fake, call } = await joined();
  const other = await joinCall(data, fake.voice.roomId(1));
  expect(call.getState().end?.kind).toBe("switched");
  expect(currentCall()).toBe(other);
  expect(on.holds()).toBe(1);
});

test("a join the server refuses leaves the call of another room as it was", async () => {
  const { data, fake, call } = await joined();
  fake.script("POST /voice/rooms/{id}/participation", { fail: 409 });
  expect(await joinCall(data, fake.voice.roomId(1)).catch((error: unknown) => error)).toMatchObject({ status: 409 });
  await settle();
  expect(currentCall()).toBe(call);
  expect(call.ended).toBe(false);
});

test("the shell's bar shows the room, and its microphone is the call's", async () => {
  const { call } = await joined();
  expect(callShown()).toMatchObject({
    roomId: call.roomId,
    room: call.getState().room.name,
    muted: true,
    phase: "live",
  });
  muteCall(false);
  await settle();
  // A listener does not send: the microphone stays off, and the bar says so.
  expect(call.getState().micFailure).toBe("refused");
  expect(callShown()?.muted).toBe(true);
});
