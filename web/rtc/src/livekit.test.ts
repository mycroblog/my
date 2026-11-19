// SPDX-License-Identifier: AGPL-3.0-only
// The LiveKit RoomSession on a fake livekit-client (ADR-0101 §12, §15): the join with the grant, whose audio plays and
// is heard, the microphone once LiveKit lets the session publish, going back to listening, the SDK's own
// reconnection, joining again after the room went away, a grant to reconnect with, the server removing the session,
// an SDK that does not load, and leaving.
import { afterEach, beforeEach, describe, expect, jest, test } from "bun:test";
import { createRoomSession } from "./session";
import { sendWait } from "./sfu";
import { browser, FakeRemoteAudio, fakeEnvironment, rejection, resetFakes, settle, statsReport } from "./test/fake";
import { DisconnectReason, FakeRoom, loadFakeLiveKit, resetFakeLiveKit, room } from "./test/livekit";
import { type RoomEnvironment, type RoomGrant, type RoomRoster, type RoomSession, RoomSessionFailure } from "./types";

const grant: RoomGrant = {
  kind: "livekit",
  url: "wss://livekit.example",
  token: "token-1",
  options: {},
  iceServers: [],
  relayOnly: false,
  expiresAt: Date.parse("2026-10-26T18:00:00Z"),
};

const speaking: RoomRoster = {
  speaker: true,
  members: [
    { session: "s2", speaker: true },
    { session: "s3", speaker: false },
  ],
};

const listening: RoomRoster = { ...speaking, speaker: false };

/** Who the session said talks, each time. */
let talking: string[][] = [];

function join(environment: Partial<RoomEnvironment> = {}): RoomSession {
  const session = createRoomSession({
    session: "s1",
    grant,
    environment: fakeEnvironment({
      createPeerConnection: () => {
        throw new Error("LiveKit makes its own connections");
      },
      loadLiveKit: loadFakeLiveKit,
      ...environment,
    }),
  });
  session.on("talking", (sessions) => talking.push([...sessions].sort()));
  return session;
}

beforeEach(() => {
  resetFakes();
  resetFakeLiveKit();
  talking = [];
});

afterEach(() => {
  jest.useRealTimers();
});

test("joins the room with the grant's URL and token, audio only, without the microphone", async () => {
  const session = join();
  expect(session.snapshot().connection.state).toBe("connecting");
  await settle();
  expect(FakeRoom.all).toHaveLength(1);
  expect(room().connect).toHaveBeenCalledWith("wss://livekit.example", "token-1", { autoSubscribe: true });
  expect(room().options).toMatchObject({
    audioCaptureDefaults: { echoCancellation: true, noiseSuppression: true, autoGainControl: true },
  });
  expect(session.snapshot().connection).toEqual({ state: "connected", participants: [] });
});

describe("remote audio", () => {
  test("plays each audio track, heard only while its session speaks", async () => {
    const session = join();
    session.update(listening);
    await settle();
    room().subscribe("s2", "t2");
    room().subscribe("s3", "t3");
    expect(FakeRemoteAudio.all.map((a) => [a.playing, a.audible])).toEqual([
      [true, true],
      [true, false],
    ]);
    expect(session.snapshot().connection.participants).toEqual([
      { session: "s2", state: "connected" },
      { session: "s3", state: "connected" },
    ]);

    session.update({ speaker: false, members: [{ session: "s3", speaker: true }] });
    expect(FakeRemoteAudio.all.map((a) => a.audible)).toEqual([false, true]);

    room().emit("trackUnsubscribed", {}, { trackSid: "t3" }, { identity: "s3" });
    expect(FakeRemoteAudio.all[1]?.closed).toBe(true);
  });

  test("says when the browser blocks the audio, and that it plays again on resume", async () => {
    FakeRemoteAudio.blocked = true;
    const session = join();
    await settle();
    room().subscribe("s2", "t2");
    await settle();
    expect(session.snapshot().playbackBlocked).toBe(true);

    FakeRemoteAudio.blocked = false;
    await session.resumePlayback();
    expect(FakeRemoteAudio.all[0]?.playing).toBe(true);
    expect(session.snapshot().playbackBlocked).toBe(false);
  });
});

describe("who is talking", () => {
  const speakers = (...identities: string[]) => {
    room().emit(
      "activeSpeakersChanged",
      identities.map((identity) => ({ identity })),
    );
  };

  test("maps LiveKit's active speakers to the sessions that speak, and reports only changes", async () => {
    const session = join();
    session.update(speaking);
    await settle();
    speakers("s2");
    speakers("s2");
    // s3 does not speak in the latest roster: LiveKit's word alone does not make it talk.
    speakers("s3", "s2");
    expect(talking).toEqual([["s2"]]);
    speakers();
    expect(talking).toEqual([["s2"], []]);
  });

  test("the user talks only while the microphone is on", async () => {
    const session = join();
    session.update(speaking);
    await settle();
    room().permit(true);
    speakers("s1", "s2");
    expect(talking.at(-1)).toEqual(["s2"]);
    await session.setMicrophone(true);
    expect(talking.at(-1)).toEqual(["s1", "s2"]);
    expect(session.snapshot().microphone).toBe(true);
    session.update(listening);
    await settle();
    expect(talking.at(-1)).toEqual(["s2"]);
  });
});

describe("the microphone", () => {
  test("goes on once LiveKit lets the session publish, and off again", async () => {
    const session = join();
    session.update(speaking);
    await settle();
    const on = session.setMicrophone(true);
    await settle();
    expect(room().localParticipant.setMicrophoneEnabled).not.toHaveBeenCalled();

    room().permit(true);
    await on;
    expect(room().localParticipant.setMicrophoneEnabled).toHaveBeenCalledWith(true);

    await session.setMicrophone(false);
    expect(room().localParticipant.setMicrophoneEnabled).toHaveBeenLastCalledWith(false);
  });

  test("rejects when LiveKit does not let the session publish in time", async () => {
    jest.useFakeTimers();
    const session = join();
    session.update(speaking);
    await settle();
    const on = session.setMicrophone(true);
    let rejected = false;
    on.catch(() => {
      rejected = true;
    });
    await settle();
    jest.advanceTimersByTime(sendWait);
    await settle();
    expect(rejected).toBe(true);
    expect(room().localParticipant.setMicrophoneEnabled).not.toHaveBeenCalled();
  });

  test("rejects when the browser refuses it, and says the microphone is off", async () => {
    const session = join();
    session.update(speaking);
    await settle();
    room().permit(true);
    room().localParticipant.setMicrophoneEnabled.mockImplementationOnce(() =>
      Promise.reject(new DOMException("denied", "NotAllowedError")),
    );
    expect(await rejection(session.setMicrophone(true))).toMatchObject({ name: "NotAllowedError" });
    expect(session.snapshot().microphone).toBe(false);
  });

  test("is unpublished and stopped when the user goes back to listening", async () => {
    const session = join();
    session.update(speaking);
    await settle();
    room().permit(true);
    await session.setMicrophone(true);

    session.update(listening);
    await settle();
    expect(room().localParticipant.unpublishTrack).toHaveBeenCalledWith(expect.objectContaining({ id: "mic" }), true);
  });
});

describe("the connection", () => {
  test("says the SDK's own reconnection", async () => {
    const session = join();
    await settle();
    const states: string[] = [];
    session.on("connection", (connection) => states.push(connection.state));
    room().emit("reconnecting");
    room().emit("reconnected");
    expect(states).toEqual(["reconnecting", "connected"]);
  });

  test("a renewed grant changes nothing while connected: LiveKit renews its token itself", async () => {
    const session = join();
    await settle();
    session.renew({ ...grant, token: "token-2" });
    await settle();
    expect(FakeRoom.all).toHaveLength(1);
  });

  test("is lost when the room goes away, and joins again with the renewed grant, the microphone on again", async () => {
    const session = join();
    session.update(speaking);
    await settle();
    room().permit(true);
    await session.setMicrophone(true);
    room().subscribe("s2", "t2");

    room().emit("disconnected", DisconnectReason.SERVER_SHUTDOWN);
    expect(FakeRemoteAudio.all[0]?.closed).toBe(true);
    expect(session.snapshot().connection.state).toBe("lost");
    session.renew({ ...grant, token: "token-2" });
    await settle();
    expect(FakeRoom.all).toHaveLength(2);
    expect(room(1).connect).toHaveBeenCalledWith("wss://livekit.example", "token-2", { autoSubscribe: true });
    room(1).permit(true);
    await settle();
    expect(room(1).localParticipant.setMicrophoneEnabled).toHaveBeenCalledWith(true);
    expect(session.snapshot().connection.state).toBe("connected");
  });

  test("a grant to reconnect with leaves the room and joins again with it", async () => {
    const session = join();
    await settle();
    room().subscribe("s2", "t2");
    session.reconnect({ ...grant, token: "token-2" });
    await settle();
    expect(room(0).disconnect).toHaveBeenCalled();
    expect(FakeRemoteAudio.all[0]?.closed).toBe(true);
    expect(room(1).connect).toHaveBeenCalledWith("wss://livekit.example", "token-2", { autoSubscribe: true });
    session.renew({ ...grant, token: "token-3" });
    await settle();
    expect(FakeRoom.all).toHaveLength(2);
  });

  test("fails, and closes, when the server removed the session", async () => {
    const session = join();
    await settle();
    const failures: RoomSessionFailure[] = [];
    session.on("failed", (failure) => failures.push(failure));
    room().emit("disconnected", DisconnectReason.PARTICIPANT_REMOVED);
    expect(failures.map((failure) => failure.reason)).toEqual(["removed"]);
    expect(session.snapshot().connection.state).toBe("closed");
    expect(browser.closed).toBe(1);
  });

  test.each([
    ["another session took its identity", DisconnectReason.DUPLICATE_IDENTITY],
    ["the room was deleted", DisconnectReason.ROOM_DELETED],
  ])("fails as removed, and closes, when %s", async (_why, reason) => {
    const session = join();
    await settle();
    const failures: RoomSessionFailure[] = [];
    session.on("failed", (failure) => failures.push(failure));
    room().emit("disconnected", reason);
    expect(failures.map((failure) => failure.reason)).toEqual(["removed"]);
    expect(failures[0]).toBeInstanceOf(RoomSessionFailure);
    expect(session.snapshot().connection.state).toBe("closed");
    expect(browser.closed).toBe(1);
  });

  test("is lost, without failing, when it is disconnected for another reason", async () => {
    const session = join();
    await settle();
    const failures: RoomSessionFailure[] = [];
    session.on("failed", (failure) => failures.push(failure));
    room().emit("disconnected", DisconnectReason.UNKNOWN_REASON);
    expect(failures).toEqual([]);
    expect(session.snapshot().connection.state).toBe("lost");
    expect(browser.closed).toBe(0);
  });

  test("fails as unavailable when the SDK does not load", async () => {
    const session = join({ loadLiveKit: () => Promise.reject(new Error("offline")) });
    await settle();
    expect(session.snapshot().failure?.reason).toBe("unavailable");
    expect(session.snapshot().connection.state).toBe("closed");
  });
});

test("samples the quality from the statistics of its tracks", async () => {
  jest.useFakeTimers();
  const session = join();
  await settle();
  room().subscribe(
    "s2",
    "t2",
    statsReport([
      { id: "R1", type: "remote-inbound-rtp", roundTripTime: 0.05 },
      { id: "I1", type: "inbound-rtp", kind: "audio", jitter: 0.004, packetsLost: 1, packetsReceived: 99 },
    ]),
  );
  jest.advanceTimersByTime(5_000);
  await settle();
  expect(session.snapshot().quality).toEqual({ rtt: 50, jitter: 4, packetLoss: 0.01 });
});

test("leaving disconnects and stops the audio, once", async () => {
  const session = join();
  await settle();
  room().subscribe("s2", "t2");
  session.close();
  session.close();
  expect(room().disconnect).toHaveBeenCalledTimes(1);
  expect(FakeRemoteAudio.all[0]?.closed).toBe(true);
  expect(browser.closed).toBe(1);
  session.renew({ ...grant, token: "token-2" });
  await settle();
  expect(FakeRoom.all).toHaveLength(1);
});
