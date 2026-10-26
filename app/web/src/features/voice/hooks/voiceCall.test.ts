// SPDX-License-Identifier: AGPL-3.0-only
// The session of a call with a fake server and a fake WebRTC (ADR-0101 §3, §5, §6): the heartbeats every 15 seconds,
// joining again after a 404, the room stream opened again after it ends, the renewal of the grant 10 minutes before it
// expires with setConfiguration and restartIce, a `grant` that asks to reconnect, and the notices of the user's role.
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { ApiError } from "../../../lib/errors";
import { otherAccountFixture, viewerFixture } from "../../../test/fixtures";
import type { VoiceGrantData, VoiceStreamEvent, VoiceStreamOptions } from "../api/call";
import type { VoiceRoom } from "../api/voice";
import { FakePeerConnection, FakeRemoteAudio, FakeStream, resetFakeRtc, settle } from "../test/fakeRtc";
import { participantFixture, voiceRoomFixture } from "../test/handlers";
import { grantRenewalLead, heartbeatInterval, reactionDuration, VoiceCall, type VoiceCallNotice, type VoiceCallServer, type VoiceJoin } from "./voiceCall";

const now = Date.parse("2026-10-26T12:00:00Z");

function grantFixture(overrides: Partial<VoiceGrantData> = {}): VoiceGrantData {
  return { kind: "p2p", options: {}, ice_servers: [{ urls: ["stun:stun.example"] }], relay_only: false, expires_at: new Date(now + 60 * 60_000).toISOString(), ...overrides };
}

const live: VoiceRoom = voiceRoomFixture({
  participants: [participantFixture(otherAccountFixture, { role: "speaker", host: true, session: "s9" }), participantFixture(viewerFixture, { session: "s5" })],
});

const notFound = () => new ApiError({ type: "about:blank", title: "Not Found", status: 404 });

/** A call with a fake server whose room stream the test drives. */
function setup(join: VoiceJoin = { session: "s5", grant: grantFixture(), room: live }) {
  const streams: { session: string; options: VoiceStreamOptions; end: () => void; fail: (error: unknown) => void }[] = [];
  const notices: VoiceCallNotice[] = [];
  const server = {
    join: vi.fn<VoiceCallServer["join"]>(() => Promise.resolve({ ...join, session: "s7" })),
    heartbeat: vi.fn<VoiceCallServer["heartbeat"]>(() => Promise.resolve()),
    refreshGrant: vi.fn<VoiceCallServer["refreshGrant"]>(() => Promise.resolve(grantFixture({ ice_servers: [{ urls: ["turn:turn.example"], username: "u2", credential: "c2" }], relay_only: true, expires_at: new Date(now + 120 * 60_000).toISOString() }))),
    sendSignal: vi.fn<VoiceCallServer["sendSignal"]>(() => Promise.resolve()),
    setMic: vi.fn<VoiceCallServer["setMic"]>(() => Promise.resolve()),
    openStream: vi.fn<VoiceCallServer["openStream"]>(
      (session, options) =>
        new Promise<void>((resolve, reject) => {
          streams.push({ session, options, end: resolve, fail: reject });
          options.onOpen();
        }),
    ),
    onRoom: vi.fn<VoiceCallServer["onRoom"]>(),
    onEnd: vi.fn<VoiceCallServer["onEnd"]>(),
  } satisfies VoiceCallServer;
  const call = new VoiceCall({
    accountId: viewerFixture.id,
    join,
    server,
    environment: {
      createPeerConnection: (configuration) => new FakePeerConnection(configuration) as unknown as RTCPeerConnection,
      getMicrophone: () => Promise.resolve(new FakeStream() as unknown as MediaStream),
      createRemoteAudio: () => new FakeRemoteAudio(),
    },
    onNotice: (notice) => notices.push(notice),
  });
  const send = (event: VoiceStreamEvent) => {
    streams.at(-1)?.options.onEvent(event);
  };
  return { call, server, streams, notices, send };
}

beforeEach(() => {
  resetFakeRtc();
  vi.useFakeTimers({ now });
});

afterEach(() => {
  vi.useRealTimers();
});

test("sends a heartbeat at once and every 15 seconds", async () => {
  const { call, server } = setup();
  call.start();
  await vi.advanceTimersByTimeAsync(heartbeatInterval * 2);
  expect(server.heartbeat.mock.calls).toEqual([["s5"], ["s5"], ["s5"]]);
  call.close();
  await vi.advanceTimersByTimeAsync(heartbeatInterval);
  expect(server.heartbeat).toHaveBeenCalledTimes(3);
});

test("joins again with a new session when the heartbeat says the session is gone", async () => {
  const { call, server, streams } = setup();
  server.heartbeat.mockRejectedValueOnce(notFound());
  call.start();
  await settle();
  expect(server.join).toHaveBeenCalledTimes(1);
  expect(call.session).toBe("s7");
  // The stream of the old session ends, and the next one opens with the new session.
  streams[0]?.end();
  await vi.advanceTimersByTimeAsync(1_000);
  expect(streams.map((s) => s.session)).toEqual(["s5", "s7"]);
  await vi.advanceTimersByTimeAsync(heartbeatInterval);
  expect(server.heartbeat).toHaveBeenLastCalledWith("s7");
});

test("opens the room stream again after it ends, and says it reconnected", async () => {
  const { call, streams, notices } = setup();
  call.start();
  await settle();
  expect(call.getSnapshot().phase).toBe("live");
  streams[0]?.fail(new Error("network"));
  await settle();
  expect(call.getSnapshot().phase).toBe("reconnecting");
  expect(notices).toEqual([{ kind: "reconnecting" }]);
  await vi.advanceTimersByTimeAsync(1_000);
  expect(streams).toHaveLength(2);
  expect(call.getSnapshot().phase).toBe("live");
  expect(notices).toEqual([{ kind: "reconnecting" }, { kind: "reconnected" }]);
});

test("asks for a new grant 10 minutes before it expires, and restarts ICE with it on every connection", async () => {
  const { call, server, send } = setup();
  call.start();
  send({ type: "state", room: live });
  await settle();
  const pc = FakePeerConnection.all[0];
  expect(pc?.configuration.iceTransportPolicy).toBe("all");

  await vi.advanceTimersByTimeAsync(60 * 60_000 - grantRenewalLead - 1_000);
  expect(server.refreshGrant).not.toHaveBeenCalled();
  await vi.advanceTimersByTimeAsync(1_000);
  expect(server.refreshGrant).toHaveBeenCalledWith("s5");
  await settle();
  expect(pc?.configurations).toEqual([{ iceServers: [{ urls: ["turn:turn.example"], username: "u2", credential: "c2" }], iceTransportPolicy: "relay" }]);
  expect(pc?.restarts).toBe(1);
  call.close();
});

test("connects again with a grant that asks to reconnect", async () => {
  const { call, send } = setup();
  call.start();
  send({ type: "state", room: live });
  await settle();
  send({ type: "grant", grant: { grant: grantFixture({ relay_only: true }), reconnect: true } });
  await settle();
  expect(FakePeerConnection.all).toHaveLength(2);
  expect(FakePeerConnection.all[0]?.closed).toBe(true);
  expect(FakePeerConnection.all[1]?.configuration.iceTransportPolicy).toBe("relay");
  call.close();
});

describe("the room stream's events", () => {
  test("a state goes to the Query cache and to the audio; a role change is a notice", async () => {
    const { call, server, send, notices } = setup();
    call.start();
    send({ type: "state", room: live });
    expect(server.onRoom).toHaveBeenCalledWith(live);
    send({ type: "state", room: { ...live, participants: live.participants.map((p) => (p.session === "s5" ? { ...p, role: "speaker" } : p)) } });
    expect(notices).toContainEqual({ kind: "speaker" });
    await settle();
    await call.setMicrophone(true);
    expect(server.setMic).toHaveBeenCalledWith("s5", true);
    expect(call.getSnapshot().micOn).toBe(true);
    send({ type: "muted", muted: { by: otherAccountFixture.id } });
    await settle();
    expect(server.setMic).toHaveBeenLastCalledWith("s5", false);
    expect(notices).toContainEqual({ kind: "muted" });
    call.close();
  });

  test("tells the hosts of a raised hand", () => {
    const hosted: VoiceRoom = {
      ...live,
      participants: [participantFixture(viewerFixture, { role: "speaker", host: true, session: "s5" }), participantFixture(otherAccountFixture, { session: "s9" })],
    };
    const { call, send, notices } = setup({ session: "s5", grant: grantFixture(), room: hosted });
    call.start();
    send({ type: "state", room: hosted });
    send({ type: "state", room: { ...hosted, participants: hosted.participants.map((p) => (p.session === "s9" ? { ...p, hand_raised: true } : p)) } });
    expect(notices).toEqual([{ kind: "hand-raised", participant: expect.objectContaining({ session: "s9" }) as unknown }]);
    call.close();
  });

  test("a reaction shows for a while, and a microphone's state is kept", async () => {
    const { call, send } = setup();
    call.start();
    send({ type: "reaction", reaction: { participant: otherAccountFixture.id, emoji: "👏" } });
    send({ type: "mic", mic: { participant: otherAccountFixture.id, on: true } });
    expect(call.getSnapshot().reactions).toEqual([{ id: 1, participant: otherAccountFixture.id, emoji: "👏" }]);
    expect(call.getSnapshot().mics.get(otherAccountFixture.id)).toBe(true);
    await vi.advanceTimersByTimeAsync(reactionDuration);
    expect(call.getSnapshot().reactions).toEqual([]);
    call.close();
  });

  test("ended ends the call with its reason, and stops everything", async () => {
    const { call, server, send } = setup();
    call.start();
    send({ type: "state", room: live });
    await settle();
    send({ type: "ended", reason: "host" });
    expect(call.getSnapshot()).toMatchObject({ phase: "ended", end: { kind: "ended", reason: "host" } });
    expect(server.onEnd).toHaveBeenCalled();
    expect(FakePeerConnection.all[0]?.closed).toBe(true);
    await vi.advanceTimersByTimeAsync(heartbeatInterval);
    expect(server.heartbeat).toHaveBeenCalledTimes(1);
  });
});

test("a method that this client does not have ends the call at once", () => {
  const { call } = setup({ session: "s5", grant: grantFixture({ kind: "mystery" }), room: live });
  call.start();
  expect(call.getSnapshot()).toMatchObject({ phase: "ended", end: { kind: "unsupported", reason: "mystery" } });
});
