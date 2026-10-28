// SPDX-License-Identifier: AGPL-3.0-only
// The Sora method with a mocked sora-js-sdk (ADR-0102 §3, §6): the join in the grant's role with the grant's metadata
// and the session's client_id, whose audio is heard, joining again sendrecv with the same client_id to speak, the
// microphone (one change after another, the latest last), going back to recvonly, joining again after the connection
// went away, and leaving.
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { FakeLevelMeter, FakeRemoteAudio, FakeStream, resetFakeRtc, settle } from "../test/fakeRtc";
import { connectVoice } from "./connect";
import { sendWait } from "./sfu";
import { talkingSample, talkingThreshold } from "./talking";
import type { VoiceConnection, VoiceGrant, VoiceRoster } from "./types";

const fake = vi.hoisted(() => {
  type Listener = (event: unknown) => void;

  class FakeSoraConnection {
    static all: FakeSoraConnection[] = [];
    static failNext = false;
    readonly listeners = new Map<string, Listener>();
    readonly connect = vi.fn((_stream?: unknown) =>
      FakeSoraConnection.failNext ? Promise.reject(new Error("refused")) : Promise.resolve(),
    );
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
    sendrecv: (channel: string, metadata: unknown, options: unknown) =>
      new FakeSoraConnection(urls, "sendrecv", channel, metadata, options),
    recvonly: (channel: string, metadata: unknown, options: unknown) =>
      new FakeSoraConnection(urls, "recvonly", channel, metadata, options),
  });

  return { FakeSoraConnection, connection };
});

vi.mock("sora-js-sdk", () => ({ default: { connection: fake.connection } }));

const { FakeSoraConnection } = fake;

let grant: VoiceGrant;
let sendrecv: VoiceGrant;

const speaking: VoiceRoster = {
  speaker: true,
  members: [
    { session: "s2", account: "b", speaker: true },
    { session: "s3", account: "c", speaker: false },
  ],
};

const listening: VoiceRoster = { ...speaking, speaker: false };

let microphones: FakeStream[] = [];
let microphoneRefused = false;
let onPlaybackBlocked = vi.fn();
/** Who the connection said talks, each time. */
let talking: string[][] = [];

function join(first: VoiceGrant = grant): VoiceConnection {
  return connectVoice({
    session: "s1",
    grant: first,
    environment: {
      createPeerConnection: () => {
        throw new Error("Sora makes its own connections");
      },
      getMicrophone: () => {
        if (microphoneRefused) return Promise.reject(new DOMException("denied", "NotAllowedError"));
        const stream = new FakeStream();
        microphones.push(stream);
        return Promise.resolve(stream as unknown as MediaStream);
      },
      createRemoteAudio: () => new FakeRemoteAudio(),
      createLevelMeter: (stream) => new FakeLevelMeter(stream),
    },
    events: {
      sendSignal: () => Promise.reject(new Error("Sora sends no signals")),
      onPlaybackBlocked,
      onTalking: (sessions) => talking.push([...sessions].sort()),
    },
  });
}

/** Joins with `first` and makes the user a speaker or a listener, as the hooks do. */
async function joined(roster: VoiceRoster, first: VoiceGrant = grant): Promise<VoiceConnection> {
  const connection = join(first);
  connection.update(roster);
  await settle();
  return connection;
}

function sora(index: number) {
  const found = FakeSoraConnection.all[index];
  if (found === undefined) throw new Error(`no connection ${index}`);
  return found;
}

function microphone(index = 0) {
  const track = microphones[index]?.tracks[0];
  if (track === undefined) throw new Error(`no microphone ${index}`);
  return track;
}

beforeEach(() => {
  resetFakeRtc();
  FakeSoraConnection.all = [];
  FakeSoraConnection.failNext = false;
  microphones = [];
  microphoneRefused = false;
  onPlaybackBlocked = vi.fn();
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
  vi.useRealTimers();
});

describe("joining", () => {
  test("a listener joins recvonly with the channel, the join value, and the session as the client_id", async () => {
    await joined(listening);
    expect(FakeSoraConnection.all).toHaveLength(1);
    expect(sora(0)).toMatchObject({
      urls: ["wss://sora1.example/signaling", "wss://sora2.example/signaling"],
      role: "recvonly",
      channel: "room1",
      metadata: { join: "join-1" },
      options: { audio: true, video: false, audioCodecType: "OPUS", clientId: "s1" },
    });
    expect(sora(0).connect).toHaveBeenCalledWith();
    expect(microphones).toHaveLength(0);
  });

  test("a speaker joins sendrecv with the microphone off, and Sora Cloud's access token goes into the metadata", async () => {
    await joined(speaking, { ...sendrecv, options: { ...sendrecv.options, access_token: "cloud" } });
    expect(sora(0)).toMatchObject({ role: "sendrecv", metadata: { join: "join-2", access_token: "cloud" } });
    expect(sora(0).connect).toHaveBeenCalledWith(microphones[0]);
    expect(microphone().enabled).toBe(false);
  });

  test("a speaker whose browser refuses the microphone joins recvonly", async () => {
    microphoneRefused = true;
    await joined(speaking, sendrecv);
    expect(FakeSoraConnection.all).toHaveLength(1);
    expect(sora(0)).toMatchObject({ role: "recvonly", metadata: { join: "join-2" } });
  });
});

describe("remote audio", () => {
  test("plays each connection's audio, heard once Sora names its session and while it speaks", async () => {
    const connection = await joined(listening);
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

  test("tells when the browser blocks the audio, and rejects resuming while it stays blocked", async () => {
    FakeRemoteAudio.blocked = true;
    const connection = await joined(listening);
    sora(0).track("c2");
    await settle();
    expect(onPlaybackBlocked).toHaveBeenCalled();

    const audio = FakeRemoteAudio.all[0];
    if (audio === undefined) throw new Error("no audio");
    audio.resume = () => Promise.reject(new DOMException("blocked", "NotAllowedError"));
    await expect(connection.resumePlayback()).rejects.toThrow();
  });
});

describe("who is talking", () => {
  /** The open meter of the remote stream of the connection `id`. */
  const remoteMeter = (id: string) => {
    const found = FakeLevelMeter.open().find((meter) => (meter.stream as { id?: string }).id === id);
    if (found === undefined) throw new Error(`no meter of ${id}`);
    return found;
  };

  test("a remote speaker once Sora names it, and the user while the microphone sends", async () => {
    vi.useFakeTimers();
    const connection = await joined(speaking, sendrecv);
    sora(0).track("c2");
    // Whose stream it is is not known yet: it is not heard, nor measured.
    expect(FakeLevelMeter.open()).toEqual([]);
    sora(0).created([{ connection_id: "c2", client_id: "s2" }]);
    remoteMeter("c2").value = talkingThreshold * 5;
    await vi.advanceTimersByTimeAsync(talkingSample);
    expect(talking).toEqual([["s2"]]);

    // The microphone is off: it is not measured.
    expect(FakeLevelMeter.open()).toHaveLength(1);
    await connection.setMicrophone(true);
    FakeLevelMeter.of(microphones[0]).value = talkingThreshold * 5;
    await vi.advanceTimersByTimeAsync(talkingSample);
    expect(talking.at(-1)).toEqual(["s1", "s2"]);
    await connection.setMicrophone(false);
    expect(talking.at(-1)).toEqual(["s2"]);

    // A participant who no longer speaks is not heard, so does not talk.
    connection.update({ ...speaking, members: [{ session: "s2", account: "b", speaker: false }] });
    expect(talking.at(-1)).toEqual([]);
    expect(FakeLevelMeter.open()).toEqual([]);
  });

  test("a track that goes away, and a connection that replaces another, release their meters", async () => {
    const connection = await joined(listening);
    sora(0).created([{ connection_id: "c2", client_id: "s2" }]);
    sora(0).track("c2");
    sora(0).track("c3");
    expect(FakeLevelMeter.open()).toHaveLength(1);
    sora(0).emit("removetrack", { target: { id: "c2" } });
    expect(FakeLevelMeter.open()).toEqual([]);

    sora(0).track("c2");
    connection.update(speaking);
    connection.reconnect(sendrecv);
    await settle();
    expect(FakeLevelMeter.open()).toEqual([]);
    // The new connection hears the same remote connection under its own key.
    sora(1).created([{ connection_id: "c2", client_id: "s2" }]);
    sora(1).track("c2");
    expect(FakeLevelMeter.open()).toHaveLength(1);

    connection.close();
    expect(FakeLevelMeter.open()).toEqual([]);
  });
});

describe("speaking", () => {
  test("a grant to reconnect with joins sendrecv with the same client_id, then drops the recvonly connection", async () => {
    const connection = await joined(listening);
    sora(0).track("c2");
    connection.update(speaking);
    connection.reconnect(sendrecv);
    await settle();
    expect(sora(1)).toMatchObject({ role: "sendrecv", metadata: { join: "join-2" }, options: { clientId: "s1" } });
    expect(sora(1).connect).toHaveBeenCalledWith(microphones[0]);
    expect(microphone().enabled).toBe(false);
    expect(sora(0).disconnect).toHaveBeenCalled();
    expect(FakeRemoteAudio.all[0]?.closed).toBe(true);

    await connection.setMicrophone(true);
    expect(microphone().enabled).toBe(true);
    expect(FakeSoraConnection.all).toHaveLength(2);
  });

  test("turning the microphone on waits for the sendrecv grant that follows the state", async () => {
    const connection = await joined(listening);
    connection.update(speaking);
    const on = connection.setMicrophone(true);
    await settle();
    expect(FakeSoraConnection.all).toHaveLength(1);

    connection.reconnect(sendrecv);
    await on;
    expect(sora(1)).toMatchObject({ role: "sendrecv" });
    expect(microphone().enabled).toBe(true);
  });

  test("turning the microphone on rejects when no sendrecv grant comes in time", async () => {
    vi.useFakeTimers();
    const connection = await joined(listening);
    connection.update(speaking);
    const rejected = expect(connection.setMicrophone(true)).rejects.toThrow();
    await vi.advanceTimersByTimeAsync(sendWait);
    await rejected;
    expect(FakeSoraConnection.all).toHaveLength(1);
    expect(microphones).toHaveLength(0);
  });

  test("after the browser refused the microphone, turning it on asks again with the valid sendrecv grant", async () => {
    const connection = await joined(listening);
    connection.update(speaking);
    microphoneRefused = true;
    connection.reconnect(sendrecv);
    await settle();
    expect(FakeSoraConnection.all).toHaveLength(1);
    expect(sora(0).disconnect).not.toHaveBeenCalled();
    // The browser's error reaches the hooks as it is.
    await expect(connection.setMicrophone(true)).rejects.toMatchObject({ name: "NotAllowedError" });

    microphoneRefused = false;
    await connection.setMicrophone(true);
    expect(sora(1)).toMatchObject({ role: "sendrecv", metadata: { join: "join-2" } });
    expect(microphone().enabled).toBe(true);
  });

  test("a failed sendrecv join keeps the recvonly connection and stops the microphone", async () => {
    const connection = await joined(listening);
    connection.update(speaking);
    FakeSoraConnection.failNext = true;
    connection.reconnect(sendrecv);
    await settle();
    expect(sora(0).disconnect).not.toHaveBeenCalled();
    expect(microphone().stopped).toBe(true);
  });

  test("the microphone goes on and off on the sendrecv connection, the latest change last", async () => {
    const connection = await joined(speaking, sendrecv);
    const on = connection.setMicrophone(true);
    const off = connection.setMicrophone(false);
    await Promise.all([on, off]);
    expect(microphone().enabled).toBe(false);
    await connection.setMicrophone(true);
    expect(microphone().enabled).toBe(true);
    expect(FakeSoraConnection.all).toHaveLength(1);
    expect(microphones).toHaveLength(1);
  });

  test("stays off for a listener", async () => {
    const connection = await joined(listening, sendrecv);
    await connection.setMicrophone(true);
    expect(microphone().enabled).toBe(false);
    expect(FakeSoraConnection.all).toHaveLength(1);
  });

  test("going back to listening turns the microphone off, and the recvonly grant stops it and joins recvonly", async () => {
    const connection = await joined(speaking, sendrecv);
    await connection.setMicrophone(true);

    connection.update(listening);
    expect(microphone().enabled).toBe(false);
    connection.reconnect({ ...grant, token: "join-3" });
    await settle();
    expect(microphone().stopped).toBe(true);
    expect(sora(1)).toMatchObject({ role: "recvonly", metadata: { join: "join-3" }, options: { clientId: "s1" } });
    expect(sora(0).disconnect).toHaveBeenCalled();
  });
});

describe("the grant", () => {
  test("a renewed grant is kept, without joining again", async () => {
    const connection = await joined(listening);
    connection.renew({ ...grant, token: "join-4" }, { restartIce: false });
    await settle();
    expect(FakeSoraConnection.all).toHaveLength(1);
  });

  test("joins again with the renewed grant after the connection went away, in its role", async () => {
    const connection = await joined(speaking, sendrecv);
    await connection.setMicrophone(true);
    sora(0).track("c2");

    sora(0).emit("disconnect", { type: "abend", title: "WEBSOCKET-ONCLOSE" });
    expect(FakeRemoteAudio.all[0]?.closed).toBe(true);
    connection.renew({ ...sendrecv, token: "join-5" }, { restartIce: false });
    await settle();
    expect(sora(1)).toMatchObject({ role: "sendrecv", metadata: { join: "join-5" } });
    expect(sora(1).connect).toHaveBeenCalledWith(microphones[0]);
  });

  test("a grant to reconnect with joins again in the same role", async () => {
    const connection = await joined(listening);
    connection.reconnect({ ...grant, token: "join-6" });
    await settle();
    expect(sora(1)).toMatchObject({ role: "recvonly", metadata: { join: "join-6" } });
    expect(sora(0).disconnect).toHaveBeenCalled();
  });
});

test("leaving disconnects, stops the microphone, and joins no more", async () => {
  const connection = await joined(speaking, sendrecv);
  await connection.setMicrophone(true);
  connection.close();
  expect(sora(0).disconnect).toHaveBeenCalled();
  expect(microphone().stopped).toBe(true);
  connection.reconnect(sendrecv);
  await settle();
  expect(FakeSoraConnection.all).toHaveLength(1);
});
