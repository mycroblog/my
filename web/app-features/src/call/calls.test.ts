// SPDX-License-Identifier: AGPL-3.0-only

// The reader's call, on the fake API and a fake RoomSession, with a clock the test moves: one call held outside every
// screen, its stream opened again on the same session, the heartbeat and the grant, and each way a call ends, once.

import { afterEach, beforeEach, expect, test } from "bun:test";
import { closeSession, createAppData, voiceStreamDelays, type AppData, type VoiceGrant } from "@my/app-data";
import { createFakeApi, fakeOrigin, type FakeApi } from "@my/e2e/fake";
import { createClient } from "@mycroblog/mykit";
import { fakeCallRuntime, settle, type FakeCallRuntime } from "../test/call";
import { currentCall, inCall, joinCall, lastCallEnd, runCallsOn } from "./calls";
import { callShown, leaveCall, muteCall } from "./store";
import { grantRenewalLead, heartbeatInterval, heartbeatTtl, roomGrant } from "./voice-call";

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

test("a stream that cannot open again for a while is tried again on the same session, the heartbeat going on meanwhile, and the call goes on from its first state", async () => {
  const { fake, call, sent, roomId } = await joined();
  const { audio, stream, heartbeat, session } = call;
  const reopens = fake.script("GET /voice/rooms/{id}/stream", { times: 2, fail: "network" });
  const beats = fake.script("POST /voice/rooms/{id}/heartbeat");
  fake.voice.drop();
  // While the stream is down the reader's hand goes up elsewhere (another tab of theirs): no stream says so.
  await fake.serve(new Request(`${fakeOrigin}/api/v1/voice/rooms/${roomId}/hand`, { method: "POST" }));
  const handUp = () =>
    call.getState().room.participants.find((p) => p.account.id === fake.accountId("yuki"))?.hand_raised;
  expect(handUp()).toBe(false);
  // The first two tries get no answer; the third, after the delays of the first three, opens.
  const [first = 0, second = 0, third = 0] = voiceStreamDelays;
  await on.clock.advance(first + second);
  expect(reopens.seen).toHaveLength(2);
  expect(call.getState().phase).toBe("reconnecting");
  await on.clock.advance(Math.max(third, heartbeatInterval));
  expect(reopens.seen.map((r) => r.url.searchParams.get("session"))).toEqual([session, session, session]);
  expect(fake.voice.opened()).toBe(2);
  expect(beats.seen.length).toBeGreaterThan(0);
  expect({ phase: call.getState().phase, ended: call.ended, current: currentCall() === call }).toEqual({
    phase: "live",
    ended: false,
    current: true,
  });
  expect([call.audio, call.stream, call.heartbeat, call.session]).toEqual([audio, stream, heartbeat, session]);
  expect(sent.filter((r) => r.line.endsWith("/participation"))).toHaveLength(1);
  // The stream opened again catches up from its first state, and the call hears what it says next.
  expect(handUp()).toBe(true);
  fake.voice.send({ event: "removed", data: { reason: "removed" } });
  await on.clock.advance(0);
  expect(call.getState().end).toEqual({ kind: "removed", reason: "removed" });
});

test("a heartbeat that fails for less than 45 seconds leaves the call on", async () => {
  const { fake, call } = await joined();
  fake.script("POST /voice/rooms/{id}/heartbeat", { times: 1, fail: "network" });
  await on.clock.advance(heartbeatTtl * 2);
  expect(call.ended).toBe(false);
  expect(call.getState().phase).toBe("live");
});

test("a heartbeat that gets no answer for 45 seconds ends the call with its reason", async () => {
  const { fake, call } = await joined();
  fake.script("POST /voice/rooms/{id}/heartbeat", { fail: "network" });
  await on.clock.advance(heartbeatTtl - 1);
  expect(call.ended).toBe(false);
  await on.clock.advance(heartbeatInterval);
  expect(call.getState().end).toEqual({ kind: "lost", reason: "heartbeat" });
  expect(currentCall()).toBeNull();
});

test("the grant the server gave again is the one handed to the audio, and it is taken again before it expires in turn", async () => {
  const s = server();
  const expiresIn = (ms: number) => new Date(on.clock.now() + grantRenewalLead + ms).toISOString();
  s.fake.script("POST /voice/rooms/{id}/participation", {
    change: (join) => ({ ...join, grant: { ...join.grant, expires_at: expiresIn(60_000) } }),
  });
  const answered: VoiceGrant[] = [];
  const grants = s.fake.script("POST /voice/rooms/{id}/grant", {
    change: (grant) => {
      const given = { ...grant, expires_at: expiresIn(120_000), options: { round: String(answered.length) } };
      answered.push(given);
      return given;
    },
  });
  const call = await joinCall(s.data, s.fake.voice.roomId(0));
  await on.clock.advance(61_000);
  expect(grants.seen).toHaveLength(1);
  expect(on.sessions[0]?.renewed).toEqual(answered.map(roomGrant));
  await on.clock.advance(118_000);
  expect(grants.seen).toHaveLength(1);
  await on.clock.advance(4_000);
  expect(grants.seen).toHaveLength(2);
  expect(on.sessions[0]?.renewed).toEqual(answered.map(roomGrant));
  expect(on.sessions[0]?.reconnected).toEqual([]);
  expect({ ended: call.ended, audio: call.audio === on.sessions[0] }).toEqual({ ended: false, audio: true });
});

test("a grant the stream sends is handed to the audio, to connect again with when the server says so", async () => {
  const { fake, call } = await joined();
  const grant = (round: string): VoiceGrant => ({
    kind: "p2p",
    options: { round },
    ice_servers: [],
    relay_only: false,
    expires_at: new Date(on.clock.now() + 6 * 60 * 60_000).toISOString(),
  });
  fake.voice.send({ event: "grant", data: { grant: grant("renew"), reconnect: false } });
  await on.clock.advance(0);
  fake.voice.send({ event: "grant", data: { grant: grant("reconnect"), reconnect: true } });
  await on.clock.advance(0);
  expect({ renewed: on.sessions[0]?.renewed, reconnected: on.sessions[0]?.reconnected }).toEqual({
    renewed: [roomGrant(grant("renew"))],
    reconnected: [roomGrant(grant("reconnect"))],
  });
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
