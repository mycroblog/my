// SPDX-License-Identifier: AGPL-3.0-only

// The reader's call, on the fake API and a fake RoomSession, with a clock the test moves: one call held outside every
// screen, its stream opened again on the same session, the heartbeat and the grant, and each way a call ends, once.

import { afterEach, beforeEach, expect, test } from "bun:test";
import { closeSession, createAppData, voiceStreamDelays, type AppData, type VoiceGrant } from "@my/app-data";
import { createFakeApi, fakeOrigin, type FakeApi } from "@my/e2e/fake";
import { RoomSessionFailure } from "@my/rtc";
import { createClient } from "@mycroblog/mykit";
import { fakeCallRuntime, settle, type FakeCallRuntime } from "../test/call";
import { currentCall, inCall, joinCall, lastCallEnd, leaveCallBeforeSignOut, runCallsOn } from "./calls";
import { callShown, leaveCall, muteCall } from "./store";
import { grantRenewalLead, heartbeatInterval, heartbeatTtl, roomGrant } from "./voice-call";

type Sent = { readonly line: string; readonly keepalive: boolean; readonly session?: string | null };

const minute = 60_000;

/** The fake's joins give a grant that expires `ms` from now, or one whose expiry cannot be read (NaN). */
function expiringIn(fake: FakeApi, ms: number) {
  fake.script("POST /voice/rooms/{id}/participation", {
    change: (join) => ({
      ...join,
      grant: {
        ...join.grant,
        expires_at: Number.isNaN(ms) ? "not a time" : new Date(on.clock.now() + ms).toISOString(),
      },
    }),
  });
}

/** The app's data on a fake API, with what it sent. */
function server(): { fake: FakeApi; data: AppData; sent: Sent[] } {
  const fake = createFakeApi();
  const sent: Sent[] = [];
  const api = createClient({
    server: fakeOrigin,
    retry: { disabled: true },
    fetch: (request) => {
      const url = new URL(request.url);
      const line = `${request.method} ${url.pathname.replace("/api/v1", "")}`;
      const session = url.searchParams.get("session");
      sent.push(
        request.method === "DELETE"
          ? { line, keepalive: request.keepalive, session }
          : { line, keepalive: request.keepalive },
      );
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
  // A grant that expires in 30 minutes: it is taken again in 20.
  expiringIn(s.fake, 30 * minute);
  const grants = s.fake.script("POST /voice/rooms/{id}/grant");
  const call = await joinCall(s.data, s.fake.voice.roomId(0));
  await on.clock.advance(30 * minute - grantRenewalLead - 1_000);
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
  expect(sent.at(-1)).toEqual({
    line: `DELETE /voice/rooms/${roomId}/participation`,
    keepalive: true,
    session: call.session,
  });
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
  // Grants that live longer than twice the lead, so that each is taken again the lead before it expires.
  const expiresIn = (ms: number) => new Date(on.clock.now() + 2 * grantRenewalLead + ms).toISOString();
  s.fake.script("POST /voice/rooms/{id}/participation", {
    change: (join) => ({ ...join, grant: { ...join.grant, expires_at: expiresIn(60_000) } }),
  });
  const answered: VoiceGrant[] = [];
  const grants = s.fake.script("POST /voice/rooms/{id}/grant", {
    change: (grant) => {
      const given = { ...grant, expires_at: expiresIn(120_000), token: `round-${String(answered.length)}` };
      answered.push(given);
      return given;
    },
  });
  const call = await joinCall(s.data, s.fake.voice.roomId(0));
  await on.clock.advance(grantRenewalLead + 59_000);
  expect(grants.seen).toHaveLength(0);
  await on.clock.advance(2_000);
  expect(grants.seen).toHaveLength(1);
  expect(on.sessions[0]?.renewed).toEqual(answered.map(roomGrant));
  await on.clock.advance(grantRenewalLead + 117_000);
  expect(grants.seen).toHaveLength(1);
  await on.clock.advance(4_000);
  expect(grants.seen).toHaveLength(2);
  expect(on.sessions[0]?.renewed).toEqual(answered.map(roomGrant));
  expect(on.sessions[0]?.reconnected).toEqual([]);
  expect({ ended: call.ended, audio: call.audio === on.sessions[0] }).toEqual({ ended: false, audio: true });
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

test("audio that stays lost asks for a grant again after a growing wait, and at once as the network comes back", async () => {
  const { fake, call } = await joined();
  const grants = fake.script("POST /voice/rooms/{id}/grant", { fail: "network" });
  const audio = on.sessions[0];
  audio?.emit("connection", { state: "lost", participants: [] });
  await on.clock.advance(0);
  expect(grants.seen).toHaveLength(1);
  await on.clock.advance(1_000);
  expect(grants.seen).toHaveLength(2);
  await on.clock.advance(1_500);
  expect(grants.seen).toHaveLength(2);
  on.goOnline();
  await settle();
  expect(grants.seen).toHaveLength(3);
  grants.end();
  on.goOnline();
  await settle();
  expect(audio?.reconnected).toHaveLength(1);
  expect(call.ended).toBe(false);
  // Once the call ends, nothing asks again.
  await call.leave();
  const after = grants.seen.length;
  await on.clock.advance(60_000);
  expect(grants.seen).toHaveLength(after);
});

test("a short grant is taken again halfway through its life, and no sooner than 30 seconds after it came", async () => {
  const s = server();
  expiringIn(s.fake, 5 * minute);
  const grants = s.fake.script("POST /voice/rooms/{id}/grant");
  await joinCall(s.data, s.fake.voice.roomId(0));
  await on.clock.advance(2.5 * minute - 1_000);
  expect(grants.seen).toHaveLength(0);
  await on.clock.advance(2_000);
  expect(grants.seen).toHaveLength(1);
});

test("a grant that already expired is taken again after 30 seconds, not at once", async () => {
  const s = server();
  expiringIn(s.fake, -minute);
  const grants = s.fake.script("POST /voice/rooms/{id}/grant");
  await joinCall(s.data, s.fake.voice.roomId(0));
  await on.clock.advance(29_000);
  expect(grants.seen).toHaveLength(0);
  await on.clock.advance(2_000);
  expect(grants.seen).toHaveLength(1);
});

test("a grant whose expiry cannot be read is not taken again on a timer", async () => {
  const s = server();
  expiringIn(s.fake, Number.NaN);
  const grants = s.fake.script("POST /voice/rooms/{id}/grant");
  await joinCall(s.data, s.fake.voice.roomId(0));
  await on.clock.advance(5 * minute);
  expect(grants.seen).toHaveLength(0);
});

test("the same credentials again are not given to the audio, and are asked for again after a wait", async () => {
  const s = server();
  const expires = new Date(on.clock.now() + 30 * minute).toISOString();
  expiringIn(s.fake, 30 * minute);
  const grants = s.fake.script("POST /voice/rooms/{id}/grant", {
    change: (grant) => ({ ...grant, expires_at: expires }),
  });
  await joinCall(s.data, s.fake.voice.roomId(0));
  await on.clock.advance(20 * minute + 1_000);
  expect(grants.seen).toHaveLength(1);
  expect(on.sessions[0]?.renewed).toHaveLength(0);
  await on.clock.advance(30_000);
  expect(grants.seen).toHaveLength(2);
});

test("signing out while a join is on its way leaves the room the join entered", async () => {
  const s = server();
  const join = s.fake.script("POST /voice/rooms/{id}/participation", { hold: "response" });
  const call = joinCall(s.data, s.fake.voice.roomId(0));
  const leaving = leaveCallBeforeSignOut();
  await join.served();
  join.release();
  await leaving;
  expect((await call).getState().end?.kind).toBe("left");
  expect(s.sent.at(-1)?.line).toBe(`DELETE /voice/rooms/${s.fake.voice.roomId(0)}/participation`);
});

test("a call whose session of the app ended before it started hears nothing of its audio", async () => {
  const s = server();
  const join = s.fake.script("POST /voice/rooms/{id}/participation", { hold: "response" });
  const call = joinCall(s.data, s.fake.voice.roomId(0));
  await join.served();
  await closeSession(s.data);
  join.release();
  expect((await call).getState().end?.kind).toBe("signed_out");
  expect(on.sessions[0]?.listening()).toBe(0);
});

/** Makes the reader a speaker in the call's room, as the stream says. */
async function speaking() {
  const joinedNow = await joined();
  await joinedNow.call.act("speak");
  await on.clock.advance(0);
  return joinedNow;
}

test("the server hears of the microphone whenever what applies differs from what it last heard", async () => {
  const { fake, call } = await speaking();
  const mics = fake.script("POST /voice/rooms/{id}/mic");
  await call.setMicrophone(true);
  // The reader listens again: the microphone goes off, and the server hears so.
  await call.act("listen");
  await on.clock.advance(0);
  expect(mics.seen.map((m) => m.body.on)).toEqual([true, false]);
  expect(call.getState().microphone).toBe(false);
});

test("a host's mute turns the microphone off without telling the server, and turning it on again tells it", async () => {
  const { fake, call } = await speaking();
  const mics = fake.script("POST /voice/rooms/{id}/mic");
  await call.setMicrophone(true);
  fake.voice.send({ event: "muted", data: { by: "host" } });
  await on.clock.advance(0);
  expect(call.getState()).toMatchObject({ microphone: false, mutedBy: "host" });
  await call.setMicrophone(true);
  expect(mics.seen.map((m) => m.body.on)).toEqual([true, true]);
});

test("the participants' microphones are those of the speakers in the latest state", async () => {
  const { fake, call } = await speaking();
  const self = call.getState().self?.account.id ?? "";
  fake.voice.send({ event: "mic", data: { participant: "gone", on: true } });
  fake.voice.send({ event: "mic", data: { participant: self, on: true } });
  await on.clock.advance(0);
  expect(call.getState().mics.size).toBe(2);
  fake.voice.send({ event: "state" });
  await on.clock.advance(0);
  expect([...call.getState().mics]).toEqual([[self, true]]);
  await call.act("listen");
  await on.clock.advance(0);
  expect(call.getState().mics.size).toBe(0);
});

test("a join of another room that the server refuses keeps a leave on its way", async () => {
  const { data, fake, call } = await joined();
  const leave = fake.script("DELETE /voice/rooms/{id}/participation", { hold: "request" });
  const leaving = call.leave();
  await leave.arrived();
  fake.script("POST /voice/rooms/{id}/participation", { fail: 409 });
  expect(await joinCall(data, fake.voice.roomId(1)).catch((error: unknown) => error)).toMatchObject({ status: 409 });
  leave.release();
  await leaving;
  expect(call.getState().end?.kind).toBe("left");
});

test("once the stream said how the room stands, a write's answer does not change it", async () => {
  const { fake, call } = await joined();
  const name = call.getState().room.name;
  fake.script("POST /voice/rooms/{id}/hand", { change: (room) => ({ ...room, name: "An older name" }) });
  await call.act("raise-hand");
  await on.clock.advance(0);
  expect(call.getState().room.name).toBe(name);
  expect(call.getState().self?.hand_raised).toBe(true);
});

test("each leave names the call's session, so that a tab another replaced leaves nothing", async () => {
  const left = await joined();
  await left.call.leave();
  expect(left.sent.at(-1)).toMatchObject({
    line: `DELETE /voice/rooms/${left.roomId}/participation`,
    session: left.call.session,
  });
  const failed = await joined();
  on.sessions.at(-1)?.emit("failed", new RoomSessionFailure("unavailable"));
  await settle();
  expect(failed.call.getState().end?.kind).toBe("failed");
  expect(failed.sent.at(-1)).toMatchObject({
    line: `DELETE /voice/rooms/${failed.roomId}/participation`,
    session: failed.call.session,
  });
});

test("giving the runtime back ends its call and leaves no end behind, and a join on its way is undone when it lands", async () => {
  const { call, sent } = await joined();
  restore();
  expect(call.ended).toBe(true);
  expect(currentCall()).toBeNull();
  expect(lastCallEnd()).toBeNull();
  expect(on.holds()).toBe(0);
  await settle();
  expect(sent.filter((r) => r.line.endsWith("/participation") && r.keepalive)).toHaveLength(1);

  restore = runCallsOn(on.runtime);
  const s = server();
  const joins = s.fake.script("POST /voice/rooms/{id}/participation", { hold: "response" });
  const late = joinCall(s.data, s.fake.voice.roomId(0));
  await joins.served();
  restore();
  joins.release();
  expect(
    await late.then(
      () => "joined",
      () => "undone",
    ),
  ).toBe("undone");
  await settle();
  expect(currentCall()).toBeNull();
  expect(lastCallEnd()).toBeNull();
  expect(s.sent.filter((r) => r.line.startsWith("DELETE"))).toHaveLength(1);
  restore = runCallsOn(on.runtime);
});
