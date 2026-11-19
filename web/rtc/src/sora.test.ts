// SPDX-License-Identifier: AGPL-3.0-only
// The Sora RoomSession on a fake sora-js-sdk (ADR-0101 §12, §15): the join in the grant's role with the grant's
// metadata and the session's client_id, whose audio is heard, joining again sendrecv with the same client_id to
// speak, the microphone (one change after another, the latest last), going back to recvonly, the connection lost and
// joined again, the microphone taken again when its device goes away, and leaving.
import { afterEach, beforeEach, describe, expect, jest, test } from "bun:test";
import { createRoomSession } from "./session";
import { sendWait } from "./sfu";
import { talkingSample, talkingThreshold } from "./talking";
import {
  browser,
  FakeLevelMeter,
  FakeRemoteAudio,
  fakeEnvironment,
  microphone,
  rejection,
  resetFakes,
  settle,
  statsReport,
} from "./test/fake";
import { FakeSoraConnection, loadFakeSora, resetFakeSora, sora } from "./test/sora";
import type { RoomGrant, RoomRoster, RoomSession } from "./types";

let grant: RoomGrant;
let sendrecv: RoomGrant;

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

function join(first: RoomGrant = grant): RoomSession {
  const session = createRoomSession({
    session: "s1",
    grant: first,
    environment: fakeEnvironment({
      createPeerConnection: () => {
        throw new Error("Sora makes its own connections");
      },
      loadSora: loadFakeSora,
    }),
  });
  session.on("talking", (sessions) => talking.push([...sessions].sort()));
  return session;
}

/** Joins with `first` and gives the session `roster`, as its caller does. */
async function joined(roster: RoomRoster, first: RoomGrant = grant): Promise<RoomSession> {
  const session = join(first);
  session.update(roster);
  await settle();
  return session;
}

function track(index = 0) {
  const found = microphone.streams[index]?.tracks[0];
  if (found === undefined) throw new Error(`no microphone ${String(index)}`);
  return found;
}

beforeEach(() => {
  resetFakes();
  resetFakeSora();
  talking = [];
  grant = {
    kind: "sora",
    url: "wss://sora1.example/signaling",
    token: "join-1",
    options: {
      channel_id: "room1",
      role: "recvonly",
      signaling_urls: "wss://sora1.example/signaling,wss://sora2.example/signaling",
    },
    iceServers: [],
    relayOnly: false,
    expiresAt: Date.now() + 60_000,
  };
  sendrecv = { ...grant, token: "join-2", options: { ...grant.options, role: "sendrecv" } };
});

afterEach(() => {
  jest.useRealTimers();
});

describe("joining", () => {
  test("a listener joins recvonly with the channel, the join value, and the session as the client_id", async () => {
    const session = await joined(listening);
    expect(FakeSoraConnection.all).toHaveLength(1);
    expect(sora(0)).toMatchObject({
      urls: ["wss://sora1.example/signaling", "wss://sora2.example/signaling"],
      role: "recvonly",
      channel: "room1",
      metadata: { join: "join-1" },
      options: { audio: true, video: false, audioCodecType: "OPUS", clientId: "s1" },
    });
    expect(sora(0).connect).toHaveBeenCalledWith();
    expect(microphone.streams).toHaveLength(0);
    expect(session.snapshot().connection.state).toBe("connected");
  });

  test("a speaker joins sendrecv with the microphone off, and Sora Cloud's access token goes into the metadata", async () => {
    await joined(speaking, { ...sendrecv, options: { ...sendrecv.options, access_token: "cloud" } });
    expect(sora(0)).toMatchObject({ role: "sendrecv", metadata: { join: "join-2", access_token: "cloud" } });
    expect(sora(0).connect).toHaveBeenCalledWith(microphone.streams[0]);
    expect(track().enabled).toBe(false);
  });

  test("a speaker whose browser refuses the microphone joins recvonly", async () => {
    microphone.refused = true;
    await joined(speaking, sendrecv);
    expect(FakeSoraConnection.all).toHaveLength(1);
    expect(sora(0)).toMatchObject({ role: "recvonly", metadata: { join: "join-2" } });
  });
});

describe("remote audio", () => {
  test("plays each connection's audio, heard once Sora names its session and while it speaks", async () => {
    const session = await joined(listening);
    sora(0).track("c2");
    expect(FakeRemoteAudio.all[0]).toMatchObject({ playing: true, audible: false });

    sora(0).created([
      { connection_id: "c2", client_id: "s2" },
      { connection_id: "c3", client_id: "s3" },
    ]);
    sora(0).track("c3");
    expect(FakeRemoteAudio.all.map((a) => a.audible)).toEqual([true, false]);
    expect(session.snapshot().connection.participants).toEqual([
      { session: "s2", state: "connected" },
      { session: "s3", state: "connected" },
    ]);

    session.update({ speaker: false, members: [{ session: "s3", speaker: true }] });
    expect(FakeRemoteAudio.all.map((a) => a.audible)).toEqual([false, true]);

    sora(0).emit("notify", { type: "notify", event_type: "connection.destroyed", connection_id: "c3" });
    expect(FakeRemoteAudio.all[1]?.closed).toBe(true);
    sora(0).emit("removetrack", { target: { id: "c2" } });
    expect(FakeRemoteAudio.all[0]?.closed).toBe(true);
  });
});

describe("who is talking", () => {
  test("a remote speaker once Sora names it, and the user while the microphone sends", async () => {
    jest.useFakeTimers();
    const session = await joined(speaking, sendrecv);
    sora(0).track("c2");
    expect(FakeLevelMeter.open()).toEqual([]);
    sora(0).created([{ connection_id: "c2", client_id: "s2" }]);
    const remote = FakeLevelMeter.open()[0];
    if (remote === undefined) throw new Error("no meter of c2");
    remote.value = talkingThreshold * 5;
    jest.advanceTimersByTime(talkingSample);
    expect(talking).toEqual([["s2"]]);

    await session.setMicrophone(true);
    FakeLevelMeter.of(microphone.streams[0]).value = talkingThreshold * 5;
    jest.advanceTimersByTime(talkingSample);
    expect(talking.at(-1)).toEqual(["s1", "s2"]);
    await session.setMicrophone(false);
    expect(talking.at(-1)).toEqual(["s2"]);

    session.update({ ...speaking, members: [{ session: "s2", speaker: false }] });
    expect(talking.at(-1)).toEqual([]);
    expect(FakeLevelMeter.open()).toEqual([]);
  });
});

describe("speaking", () => {
  test("a grant to reconnect with joins sendrecv with the same client_id, then drops the recvonly connection", async () => {
    const session = await joined(listening);
    sora(0).track("c2");
    session.update(speaking);
    session.reconnect(sendrecv);
    await settle();
    expect(sora(1)).toMatchObject({ role: "sendrecv", metadata: { join: "join-2" }, options: { clientId: "s1" } });
    expect(sora(1).connect).toHaveBeenCalledWith(microphone.streams[0]);
    expect(track().enabled).toBe(false);
    expect(sora(0).disconnect).toHaveBeenCalled();
    expect(FakeRemoteAudio.all[0]?.closed).toBe(true);

    await session.setMicrophone(true);
    expect(track().enabled).toBe(true);
    expect(session.snapshot().microphone).toBe(true);
    expect(FakeSoraConnection.all).toHaveLength(2);
  });

  test("turning the microphone on waits for the sendrecv grant", async () => {
    const session = await joined(listening);
    session.update(speaking);
    const on = session.setMicrophone(true);
    await settle();
    expect(FakeSoraConnection.all).toHaveLength(1);

    session.reconnect(sendrecv);
    await on;
    expect(sora(1)).toMatchObject({ role: "sendrecv" });
    expect(track().enabled).toBe(true);
  });

  test("turning the microphone on rejects when no sendrecv grant comes in time", async () => {
    jest.useFakeTimers();
    const session = await joined(listening);
    session.update(speaking);
    let rejected = false;
    session.setMicrophone(true).catch(() => {
      rejected = true;
    });
    await settle();
    jest.advanceTimersByTime(sendWait);
    await settle();
    expect(rejected).toBe(true);
    expect(microphone.streams).toHaveLength(0);
  });

  test("after the browser refused the microphone, turning it on asks again with the valid sendrecv grant", async () => {
    const session = await joined(listening);
    session.update(speaking);
    microphone.refused = true;
    session.reconnect(sendrecv);
    await settle();
    expect(FakeSoraConnection.all).toHaveLength(1);
    expect(sora(0).disconnect).not.toHaveBeenCalled();
    expect(await rejection(session.setMicrophone(true))).toMatchObject({ name: "NotAllowedError" });

    microphone.refused = false;
    await session.setMicrophone(true);
    expect(sora(1)).toMatchObject({ role: "sendrecv", metadata: { join: "join-2" } });
    expect(track().enabled).toBe(true);
  });

  test("going back to listening turns the microphone off, and the recvonly grant stops it and joins recvonly", async () => {
    const session = await joined(speaking, sendrecv);
    await session.setMicrophone(true);

    session.update(listening);
    expect(track().enabled).toBe(false);
    session.reconnect({ ...grant, token: "join-3" });
    await settle();
    expect(track().stopped).toBe(true);
    expect(sora(1)).toMatchObject({ role: "recvonly", metadata: { join: "join-3" }, options: { clientId: "s1" } });
    expect(sora(0).disconnect).toHaveBeenCalled();
  });
});

describe("stability", () => {
  test("is lost when the connection goes away, and joins again with the renewed grant in its role", async () => {
    const session = await joined(speaking, sendrecv);
    await session.setMicrophone(true);
    sora(0).track("c2");

    sora(0).emit("disconnect", { type: "abend", title: "WEBSOCKET-ONCLOSE" });
    expect(FakeRemoteAudio.all[0]?.closed).toBe(true);
    expect(session.snapshot().connection.state).toBe("lost");
    session.renew({ ...sendrecv, token: "join-5" });
    await settle();
    expect(sora(1)).toMatchObject({ role: "sendrecv", metadata: { join: "join-5" } });
    expect(sora(1).connect).toHaveBeenCalledWith(microphone.streams[0]);
    expect(session.snapshot().connection.state).toBe("connected");
  });

  test("joins again when the network comes back while the grant is valid", async () => {
    const session = await joined(listening);
    sora(0).emit("disconnect", { type: "abend", title: "WEBSOCKET-ONCLOSE" });
    browser.networkChange();
    await settle();
    expect(sora(1)).toMatchObject({ role: "recvonly", metadata: { join: "join-1" } });
    expect(session.snapshot().connection.state).toBe("connected");
  });

  test("stays lost when the network comes back after the grant expired", async () => {
    const session = await joined(listening, { ...grant, expiresAt: Date.now() - 1 });
    sora(0).emit("disconnect", { type: "abend", title: "WEBSOCKET-ONCLOSE" });
    browser.networkChange();
    await settle();
    expect(FakeSoraConnection.all).toHaveLength(1);
    expect(session.snapshot().connection.state).toBe("lost");
  });

  test("a disconnect is lost, never a failure", async () => {
    const session = await joined(listening);
    const failures: unknown[] = [];
    session.on("failed", (failure) => failures.push(failure));
    sora(0).emit("disconnect", { type: "abend", title: "WEBSOCKET-ONCLOSE" });
    expect(failures).toEqual([]);
    expect(browser.closed).toBe(0);
  });

  test("fails as unavailable when the SDK does not load", async () => {
    const session = createRoomSession({
      session: "s1",
      grant,
      environment: fakeEnvironment({ loadSora: () => Promise.reject(new Error("offline")) }),
    });
    session.update(listening);
    await settle();
    expect(session.snapshot().failure?.reason).toBe("unavailable");
    expect(session.snapshot().connection.state).toBe("closed");
  });

  test("a renewed grant is kept, without joining again", async () => {
    const session = await joined(listening);
    session.renew({ ...grant, token: "join-4" });
    await settle();
    expect(FakeSoraConnection.all).toHaveLength(1);
  });

  test("takes the microphone again when its device goes away, and replaces the sent track", async () => {
    const session = await joined(speaking, sendrecv);
    await session.setMicrophone(true);
    microphone.device = "mic-2";
    microphone.inputs = ["mic-2"];
    browser.deviceChange();
    await settle();
    expect(microphone.streams).toHaveLength(2);
    expect(track(0).stopped).toBe(true);
    expect(track(1).enabled).toBe(true);
    expect(sora(0).replaceAudioTrack).toHaveBeenCalledWith(microphone.streams[0], track(1));
  });

  test("samples the quality from the connection's statistics", async () => {
    jest.useFakeTimers();
    const session = await joined(listening);
    sora(0).stats = statsReport([
      { id: "P1", type: "candidate-pair", nominated: true, state: "succeeded", currentRoundTripTime: 0.03 },
      { id: "I1", type: "inbound-rtp", kind: "audio", jitter: 0.002, packetsLost: 0, packetsReceived: 50 },
    ]);
    jest.advanceTimersByTime(5_000);
    await settle();
    expect(session.snapshot().quality).toEqual({ rtt: 30, jitter: 2, packetLoss: 0 });
  });
});

test("leaving disconnects, stops the microphone, releases the environment, and joins no more", async () => {
  const session = await joined(speaking, sendrecv);
  await session.setMicrophone(true);
  session.close();
  session.close();
  expect(sora(0).disconnect).toHaveBeenCalledTimes(1);
  expect(track().stopped).toBe(true);
  expect(browser.closed).toBe(1);
  session.reconnect(sendrecv);
  await settle();
  expect(FakeSoraConnection.all).toHaveLength(1);
});
