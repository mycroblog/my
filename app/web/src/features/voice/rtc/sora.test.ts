// SPDX-License-Identifier: AGPL-3.0-only
// The Sora method with a mocked sora-js-sdk (ADR-0102 §3, §6): the recvonly join with the grant's metadata and the
// session's client_id, whose audio is heard, joining again sendrecv with the same client_id to speak, muting, going
// back to recvonly, joining again after the connection went away, and leaving.
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { FakeRemoteAudio, FakeStream, resetFakeRtc, settle } from "../test/fakeRtc";
import { connectVoice } from "./connect";
import { sendWait } from "./sfu";
import type { VoiceConnection, VoiceGrant, VoiceRoster } from "./types";

const fake = vi.hoisted(() => {
  type Listener = (event: unknown) => void;

  class FakeSoraConnection {
    static all: FakeSoraConnection[] = [];
    static failNext = false;
    readonly listeners = new Map<string, Listener>();
    readonly connect = vi.fn((_stream?: unknown) => (FakeSoraConnection.failNext ? Promise.reject(new Error("refused")) : Promise.resolve()));
    readonly disconnect = vi.fn(() => {
      // The SDK calls the disconnect callback for a disconnect of its own too.
      this.listeners.get("disconnect")?.({ type: "normal", title: "DISCONNECT" });
      return Promise.resolve();
    });
    readonly urls: string[];
    readonly role: string;
    readonly channel: string;
    readonly metadata: unknown;
    readonly options: unknown;

    constructor(urls: string[], role: string, channel: string, metadata: unknown, options: unknown) {
      this.urls = urls;
      this.role = role;
      this.channel = channel;
      this.metadata = metadata;
      this.options = options;
      FakeSoraConnection.all.push(this);
    }

    on(kind: string, listener: Listener): void {
      this.listeners.set(kind, listener);
    }

    emit(kind: string, event: unknown): void {
      this.listeners.get(kind)?.(event);
    }

    /** A remote connection's audio arrives in the stream of its connection ID. */
    track(connectionId: string): void {
      this.emit("track", { track: { kind: "audio" }, streams: [{ id: connectionId }] });
    }

    /** Sora names the client_id of connections. */
    created(connections: { connection_id: string; client_id: string }[]): void {
      this.emit("notify", { type: "notify", event_type: "connection.created", data: connections });
    }
  }

  const connection = (urls: string[]) => ({
    sendrecv: (channel: string, metadata: unknown, options: unknown) => new FakeSoraConnection(urls, "sendrecv", channel, metadata, options),
    recvonly: (channel: string, metadata: unknown, options: unknown) => new FakeSoraConnection(urls, "recvonly", channel, metadata, options),
  });

  return { FakeSoraConnection, connection };
});

vi.mock("sora-js-sdk", () => ({ default: { connection: fake.connection } }));

const { FakeSoraConnection } = fake;

const grant: VoiceGrant = {
  kind: "sora",
  url: "wss://sora1.example/signaling",
  token: "join-1",
  options: { channel_id: "room1", role: "recvonly", signaling_urls: "wss://sora1.example/signaling,wss://sora2.example/signaling" },
  iceServers: [],
  relayOnly: false,
  expiresAt: Date.parse("2026-10-26T18:00:00Z"),
};

const sendrecv: VoiceGrant = { ...grant, token: "join-2", options: { ...grant.options, role: "sendrecv" } };

const speaking: VoiceRoster = {
  speaker: true,
  members: [
    { session: "s2", account: "b", speaker: true },
    { session: "s3", account: "c", speaker: false },
  ],
};

const listening: VoiceRoster = { ...speaking, speaker: false };

let microphones: FakeStream[] = [];
let onPlaybackBlocked = vi.fn();

function join(first: VoiceGrant = grant): VoiceConnection {
  return connectVoice({
    session: "s1",
    grant: first,
    environment: {
      createPeerConnection: () => {
        throw new Error("Sora makes its own connections");
      },
      getMicrophone: () => {
        const stream = new FakeStream();
        microphones.push(stream);
        return Promise.resolve(stream as unknown as MediaStream);
      },
      createRemoteAudio: () => new FakeRemoteAudio(),
    },
    events: { sendSignal: () => Promise.reject(new Error("Sora sends no signals")), onPlaybackBlocked },
  });
}

function sora(index: number) {
  const found = FakeSoraConnection.all[index];
  if (found === undefined) throw new Error(`no connection ${index}`);
  return found;
}

beforeEach(() => {
  resetFakeRtc();
  FakeSoraConnection.all = [];
  FakeSoraConnection.failNext = false;
  microphones = [];
  onPlaybackBlocked = vi.fn();
});

afterEach(() => {
  vi.useRealTimers();
});

test("joins recvonly with the channel, the join value, and the session as the client_id", async () => {
  join();
  await settle();
  expect(FakeSoraConnection.all).toHaveLength(1);
  expect(sora(0)).toMatchObject({
    urls: ["wss://sora1.example/signaling", "wss://sora2.example/signaling"],
    role: "recvonly",
    channel: "room1",
    metadata: { join: "join-1" },
    options: { audio: true, video: false, audioCodecType: "OPUS", clientId: "s1" },
  });
  expect(sora(0).connect).toHaveBeenCalledWith();
});

test("a speaker joins recvonly too, and Sora Cloud's access token goes into the metadata", async () => {
  join({ ...sendrecv, options: { ...sendrecv.options, access_token: "cloud" } });
  await settle();
  expect(sora(0)).toMatchObject({ role: "recvonly", metadata: { join: "join-2", access_token: "cloud" } });
  expect(microphones).toHaveLength(0);
});

describe("remote audio", () => {
  test("plays each connection's audio, heard once Sora names its session and while it speaks", async () => {
    const connection = join();
    connection.update(listening);
    await settle();
    sora(0).track("c2");
    expect(FakeRemoteAudio.all[0]).toMatchObject({ playing: true, audible: false });

    sora(0).created([
      { connection_id: "c2", client_id: "s2" },
      { connection_id: "c3", client_id: "s3" },
    ]);
    sora(0).track("c3");
    expect(FakeRemoteAudio.all.map((a) => a.audible)).toEqual([true, false]);

    connection.update({ speaker: false, members: [{ session: "s3", account: "c", speaker: true }] });
    expect(FakeRemoteAudio.all.map((a) => a.audible)).toEqual([false, true]);

    sora(0).emit("notify", { type: "notify", event_type: "connection.destroyed", connection_id: "c3" });
    expect(FakeRemoteAudio.all[1]?.closed).toBe(true);
    sora(0).emit("removetrack", { target: { id: "c2" } });
    expect(FakeRemoteAudio.all[0]?.closed).toBe(true);
  });

  test("tells when the browser blocks the audio, and plays it again on resume", async () => {
    FakeRemoteAudio.blocked = true;
    const connection = join();
    await settle();
    sora(0).track("c2");
    await settle();
    expect(onPlaybackBlocked).toHaveBeenCalled();

    await connection.resumePlayback();
    expect(FakeRemoteAudio.all[0]?.playing).toBe(true);
  });
});

describe("speaking", () => {
  test("joins again sendrecv with the same client_id once the sendrecv grant comes, then drops the recvonly connection", async () => {
    const connection = join();
    connection.update(speaking);
    await settle();
    sora(0).track("c2");
    const on = connection.setMicrophone(true);
    await settle();
    expect(FakeSoraConnection.all).toHaveLength(1);

    // The grant event of a speaker whose microphone is off: it keeps listening until the microphone goes on.
    connection.reconnect(sendrecv);
    await on;
    expect(FakeSoraConnection.all).toHaveLength(2);
    expect(sora(1)).toMatchObject({ role: "sendrecv", metadata: { join: "join-2" }, options: { clientId: "s1" } });
    expect(sora(1).connect).toHaveBeenCalledWith(microphones[0]);
    expect(microphones[0]?.tracks[0]?.enabled).toBe(true);
    expect(sora(0).disconnect).toHaveBeenCalled();
    expect(FakeRemoteAudio.all[0]?.closed).toBe(true);
  });

  test("joins sendrecv at once with a sendrecv grant in hand", async () => {
    const connection = join(sendrecv);
    connection.update(speaking);
    await settle();
    await connection.setMicrophone(true);
    expect(sora(1)).toMatchObject({ role: "sendrecv" });
  });

  test("rejects when no sendrecv grant comes in time", async () => {
    vi.useFakeTimers();
    const connection = join();
    connection.update(speaking);
    await settle();
    const rejected = expect(connection.setMicrophone(true)).rejects.toThrow();
    await vi.advanceTimersByTimeAsync(sendWait);
    await rejected;
    expect(FakeSoraConnection.all).toHaveLength(1);
    expect(microphones).toHaveLength(0);
  });

  test("a failed sendrecv join keeps the recvonly connection and stops the microphone", async () => {
    const connection = join(sendrecv);
    connection.update(speaking);
    await settle();
    FakeSoraConnection.failNext = true;
    await expect(connection.setMicrophone(true)).rejects.toThrow();
    expect(sora(0).disconnect).not.toHaveBeenCalled();
    expect(microphones[0]?.tracks[0]?.stopped).toBe(true);
  });

  test("muting turns the track off on the sendrecv connection, without joining again", async () => {
    const connection = join(sendrecv);
    connection.update(speaking);
    await settle();
    await connection.setMicrophone(true);
    await connection.setMicrophone(false);
    expect(microphones[0]?.tracks[0]?.enabled).toBe(false);
    await connection.setMicrophone(true);
    expect(microphones[0]?.tracks[0]?.enabled).toBe(true);
    expect(FakeSoraConnection.all).toHaveLength(2);
    expect(microphones).toHaveLength(1);
  });

  test("stays off for a listener", async () => {
    const connection = join(sendrecv);
    connection.update(listening);
    await settle();
    await connection.setMicrophone(true);
    expect(FakeSoraConnection.all).toHaveLength(1);
  });

  test("going back to listening stops the microphone and joins recvonly with the new grant", async () => {
    const connection = join(sendrecv);
    connection.update(speaking);
    await settle();
    await connection.setMicrophone(true);

    connection.update(listening);
    expect(microphones[0]?.tracks[0]?.stopped).toBe(true);
    connection.reconnect({ ...grant, token: "join-3" });
    await settle();
    expect(sora(2)).toMatchObject({ role: "recvonly", metadata: { join: "join-3" }, options: { clientId: "s1" } });
    expect(sora(1).disconnect).toHaveBeenCalled();
  });
});

describe("the grant", () => {
  test("a renewed grant is kept for the next join, without joining again", async () => {
    const connection = join(sendrecv);
    connection.update(speaking);
    await settle();
    connection.renew({ ...sendrecv, token: "join-4" });
    await settle();
    expect(FakeSoraConnection.all).toHaveLength(1);
    await connection.setMicrophone(true);
    expect(sora(1)).toMatchObject({ metadata: { join: "join-4" } });
  });

  test("joins again with the renewed grant after the connection went away, in the role it had", async () => {
    const connection = join(sendrecv);
    connection.update(speaking);
    await settle();
    await connection.setMicrophone(true);
    sora(1).track("c2");

    sora(1).emit("disconnect", { type: "abend", title: "WEBSOCKET-ONCLOSE" });
    expect(FakeRemoteAudio.all[0]?.closed).toBe(true);
    connection.renew({ ...sendrecv, token: "join-5" });
    await settle();
    expect(sora(2)).toMatchObject({ role: "sendrecv", metadata: { join: "join-5" } });
    expect(sora(2).connect).toHaveBeenCalledWith(microphones[0]);
  });

  test("a grant to reconnect with joins again in the same role", async () => {
    const connection = join();
    await settle();
    connection.reconnect({ ...grant, token: "join-6" });
    await settle();
    expect(sora(1)).toMatchObject({ role: "recvonly", metadata: { join: "join-6" } });
    expect(sora(0).disconnect).toHaveBeenCalled();
  });
});

test("leaving disconnects, stops the microphone, and joins no more", async () => {
  const connection = join(sendrecv);
  connection.update(speaking);
  await settle();
  await connection.setMicrophone(true);
  connection.close();
  expect(sora(1).disconnect).toHaveBeenCalled();
  expect(microphones[0]?.tracks[0]?.stopped).toBe(true);
  connection.renew(sendrecv);
  await settle();
  expect(FakeSoraConnection.all).toHaveLength(2);
});
