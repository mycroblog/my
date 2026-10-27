// SPDX-License-Identifier: AGPL-3.0-only
// The LiveKit method with a mocked livekit-client (ADR-0102 §3, §6): the join with the grant, whose audio plays and is
// heard, the microphone once LiveKit lets the session publish, going back to listening, joining again after the room
// went away, a grant to reconnect with, and leaving.
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { FakeRemoteAudio, resetFakeRtc, settle } from "../test/fakeRtc";
import { connectVoice } from "./connect";
import { sendWait } from "./sfu";
import type { VoiceConnection, VoiceGrant, VoiceRoster } from "./types";

const fake = vi.hoisted(() => {
  type Listener = (...args: unknown[]) => void;

  class FakeLocalParticipant {
    permissions: { canPublish: boolean } | undefined = { canPublish: false };
    microphone: { track: { id: string } } | undefined;
    readonly setMicrophoneEnabled = vi.fn((enabled: boolean) => {
      if (enabled) this.microphone = { track: { id: "mic" } };
      return Promise.resolve(this.microphone);
    });
    readonly unpublishTrack = vi.fn((_track: unknown, _stop?: boolean) => {
      this.microphone = undefined;
      return Promise.resolve(undefined);
    });
    getTrackPublication(source: string) {
      return source === "microphone" ? this.microphone : undefined;
    }
  }

  class FakeRoom {
    static all: FakeRoom[] = [];
    readonly options: unknown;
    readonly localParticipant = new FakeLocalParticipant();
    readonly listeners = new Map<string, Listener[]>();
    readonly connect = vi.fn((_url: string, _token: string, _options?: unknown) => (FakeRoom.failNext ? Promise.reject(new Error("refused")) : Promise.resolve()));
    readonly disconnect = vi.fn(() => {
      this.emit("disconnected", 1);
      return Promise.resolve();
    });
    static failNext = false;

    constructor(options: unknown) {
      this.options = options;
      FakeRoom.all.push(this);
    }

    on(event: string, listener: Listener): this {
      this.listeners.set(event, [...(this.listeners.get(event) ?? []), listener]);
      return this;
    }

    emit(event: string, ...args: unknown[]): void {
      for (const listener of this.listeners.get(event) ?? []) listener(...args);
    }

    /** LiveKit changes what the local participant may do. */
    permit(canPublish: boolean): void {
      const previous = this.localParticipant.permissions;
      this.localParticipant.permissions = { canPublish };
      this.emit("participantPermissionsChanged", previous, this.localParticipant);
    }

    /** A remote participant's audio track is subscribed. */
    subscribe(identity: string, sid: string): void {
      this.emit("trackSubscribed", { kind: "audio", mediaStreamTrack: { id: sid } }, { trackSid: sid }, { identity });
    }
  }

  return { FakeRoom };
});

vi.mock("livekit-client", () => ({
  Room: fake.FakeRoom,
  RoomEvent: {
    TrackSubscribed: "trackSubscribed",
    TrackUnsubscribed: "trackUnsubscribed",
    ParticipantPermissionsChanged: "participantPermissionsChanged",
    ActiveSpeakersChanged: "activeSpeakersChanged",
    Disconnected: "disconnected",
  },
  Track: { Kind: { Audio: "audio" }, Source: { Microphone: "microphone" } },
}));

const { FakeRoom } = fake;

const grant: VoiceGrant = {
  kind: "livekit",
  url: "wss://livekit.example",
  token: "token-1",
  options: {},
  iceServers: [],
  relayOnly: false,
  expiresAt: Date.parse("2026-10-26T18:00:00Z"),
};

const speaking: VoiceRoster = {
  speaker: true,
  members: [
    { session: "s2", account: "b", speaker: true },
    { session: "s3", account: "c", speaker: false },
  ],
};

const listening: VoiceRoster = { ...speaking, speaker: false };

let onPlaybackBlocked = vi.fn();
let getMicrophone = vi.fn();
/** Who the connection said talks, each time. */
let talking: string[][] = [];

function join(): VoiceConnection {
  return connectVoice({
    session: "s1",
    grant,
    environment: {
      createPeerConnection: () => {
        throw new Error("LiveKit makes its own connections");
      },
      getMicrophone,
      createRemoteAudio: () => new FakeRemoteAudio(),
    },
    events: {
      sendSignal: () => Promise.reject(new Error("LiveKit sends no signals")),
      onPlaybackBlocked,
      onTalking: (sessions) => talking.push([...sessions].sort()),
    },
  });
}

function room(index = 0) {
  const found = FakeRoom.all[index];
  if (found === undefined) throw new Error(`no room ${index}`);
  return found;
}

beforeEach(() => {
  resetFakeRtc();
  FakeRoom.all = [];
  FakeRoom.failNext = false;
  onPlaybackBlocked = vi.fn();
  getMicrophone = vi.fn();
  talking = [];
  vi.stubGlobal(
    "MediaStream",
    class {
      readonly tracks: unknown[];
      constructor(tracks: unknown[]) {
        this.tracks = tracks;
      }
    },
  );
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

test("joins the room with the grant's URL and token, audio only", async () => {
  join();
  await settle();
  expect(FakeRoom.all).toHaveLength(1);
  expect(room().connect).toHaveBeenCalledWith("wss://livekit.example", "token-1", { autoSubscribe: true });
  expect(room().options).toMatchObject({ audioCaptureDefaults: { echoCancellation: true, noiseSuppression: true, autoGainControl: true } });
  expect(getMicrophone).not.toHaveBeenCalled();
});

describe("remote audio", () => {
  test("plays each audio track, heard only while its session speaks", async () => {
    const connection = join();
    connection.update(listening);
    await settle();
    room().subscribe("s2", "t2");
    room().subscribe("s3", "t3");
    expect(FakeRemoteAudio.all.map((a) => [a.playing, a.audible])).toEqual([
      [true, true],
      [true, false],
    ]);

    connection.update({ speaker: false, members: [{ session: "s3", account: "c", speaker: true }] });
    expect(FakeRemoteAudio.all.map((a) => a.audible)).toEqual([false, true]);

    room().emit("trackUnsubscribed", {}, { trackSid: "t3" }, { identity: "s3" });
    expect(FakeRemoteAudio.all[1]?.closed).toBe(true);
  });

  test("tells when the browser blocks the audio, and plays it again on resume", async () => {
    FakeRemoteAudio.blocked = true;
    const connection = join();
    await settle();
    room().subscribe("s2", "t2");
    await settle();
    expect(onPlaybackBlocked).toHaveBeenCalled();

    await connection.resumePlayback();
    expect(FakeRemoteAudio.all[0]?.playing).toBe(true);
  });
});

describe("who is talking", () => {
  /** LiveKit's active speakers, loudest first, the local participant among them. */
  const speakers = (...identities: string[]) => {
    room().emit("activeSpeakersChanged", identities.map((identity) => ({ identity })));
  };

  test("maps LiveKit's active speakers to the sessions that speak, and reports only changes", async () => {
    const connection = join();
    connection.update(speaking);
    await settle();
    speakers("s2");
    speakers("s2");
    // s3 does not speak in the latest state: LiveKit's word alone does not make it talk.
    speakers("s3", "s2");
    expect(talking).toEqual([["s2"]]);
    speakers();
    expect(talking).toEqual([["s2"], []]);
  });

  test("the user talks only while the microphone is on", async () => {
    const connection = join();
    connection.update(speaking);
    await settle();
    room().permit(true);
    speakers("s1", "s2");
    expect(talking.at(-1)).toEqual(["s2"]);
    await connection.setMicrophone(true);
    expect(talking.at(-1)).toEqual(["s1", "s2"]);
    connection.update(listening);
    await settle();
    expect(talking.at(-1)).toEqual(["s2"]);
  });

  test("nobody talks once the room goes away or the user leaves", async () => {
    const connection = join();
    connection.update(speaking);
    await settle();
    speakers("s2");
    room().emit("disconnected", 1);
    expect(talking.at(-1)).toEqual([]);

    connection.reconnect(grant);
    await settle();
    room(1).emit("activeSpeakersChanged", [{ identity: "s2" }]);
    // The room that went away says nothing more.
    room(0).emit("activeSpeakersChanged", []);
    expect(talking.at(-1)).toEqual(["s2"]);
    connection.close();
    expect(talking.at(-1)).toEqual([]);
  });
});

describe("the microphone", () => {
  test("goes on once LiveKit lets the session publish, and off again", async () => {
    const connection = join();
    connection.update(speaking);
    await settle();
    const on = connection.setMicrophone(true);
    await settle();
    expect(room().localParticipant.setMicrophoneEnabled).not.toHaveBeenCalled();

    room().permit(true);
    await on;
    expect(room().localParticipant.setMicrophoneEnabled).toHaveBeenCalledWith(true);

    await connection.setMicrophone(false);
    expect(room().localParticipant.setMicrophoneEnabled).toHaveBeenLastCalledWith(false);
  });

  test("rejects when LiveKit does not let the session publish in time", async () => {
    vi.useFakeTimers();
    const connection = join();
    connection.update(speaking);
    await settle();
    const on = connection.setMicrophone(true);
    const rejected = expect(on).rejects.toThrow();
    await vi.advanceTimersByTimeAsync(sendWait);
    await rejected;
    expect(room().localParticipant.setMicrophoneEnabled).not.toHaveBeenCalled();
  });

  test("applies one change after another, the latest last", async () => {
    const connection = join();
    connection.update(speaking);
    await settle();
    room().permit(true);
    const on = connection.setMicrophone(true);
    const off = connection.setMicrophone(false);
    await Promise.all([on, off]);
    expect(room().localParticipant.setMicrophoneEnabled.mock.calls).toEqual([[false], [false]]);

    await connection.setMicrophone(true);
    expect(room().localParticipant.setMicrophoneEnabled).toHaveBeenLastCalledWith(true);
  });

  test("stays off for a listener", async () => {
    const connection = join();
    connection.update(listening);
    await settle();
    room().permit(true);
    await connection.setMicrophone(true);
    expect(room().localParticipant.setMicrophoneEnabled).not.toHaveBeenCalledWith(true);
  });

  test("is unpublished and stopped when the user goes back to listening", async () => {
    const connection = join();
    connection.update(speaking);
    await settle();
    room().permit(true);
    await connection.setMicrophone(true);

    connection.update(listening);
    await settle();
    expect(room().localParticipant.unpublishTrack).toHaveBeenCalledWith({ id: "mic" }, true);
  });
});

describe("the grant", () => {
  test("a renewed grant changes nothing while connected: LiveKit renews its token itself", async () => {
    const connection = join();
    await settle();
    connection.renew({ ...grant, token: "token-2" });
    await settle();
    expect(FakeRoom.all).toHaveLength(1);
  });

  test("joins again with the renewed grant after the room went away, with the microphone on again", async () => {
    const connection = join();
    connection.update(speaking);
    await settle();
    room().permit(true);
    await connection.setMicrophone(true);
    room().subscribe("s2", "t2");

    room().emit("disconnected", 3);
    expect(FakeRemoteAudio.all[0]?.closed).toBe(true);
    connection.renew({ ...grant, token: "token-2" });
    await settle();
    expect(FakeRoom.all).toHaveLength(2);
    expect(room(1).connect).toHaveBeenCalledWith("wss://livekit.example", "token-2", { autoSubscribe: true });
    room(1).permit(true);
    await settle();
    expect(room(1).localParticipant.setMicrophoneEnabled).toHaveBeenCalledWith(true);
  });

  test("a failed join is tried again with the next grant", async () => {
    FakeRoom.failNext = true;
    const connection = join();
    await settle();
    FakeRoom.failNext = false;
    await expect(connection.setMicrophone(false)).resolves.toBeUndefined();
    connection.renew({ ...grant, token: "token-2" });
    await settle();
    expect(room(1).connect).toHaveBeenCalledWith("wss://livekit.example", "token-2", { autoSubscribe: true });
  });

  test("a grant to reconnect with leaves the room and joins again with it", async () => {
    const connection = join();
    await settle();
    room().subscribe("s2", "t2");
    connection.reconnect({ ...grant, token: "token-2" });
    await settle();
    expect(room(0).disconnect).toHaveBeenCalled();
    expect(FakeRemoteAudio.all[0]?.closed).toBe(true);
    expect(room(1).connect).toHaveBeenCalledWith("wss://livekit.example", "token-2", { autoSubscribe: true });
    // The previous room's Disconnected is the user's own: no join follows the next renewal.
    connection.renew({ ...grant, token: "token-3" });
    await settle();
    expect(FakeRoom.all).toHaveLength(2);
  });
});

test("leaving disconnects and stops the audio", async () => {
  const connection = join();
  await settle();
  room().subscribe("s2", "t2");
  connection.close();
  expect(room().disconnect).toHaveBeenCalled();
  expect(FakeRemoteAudio.all[0]?.closed).toBe(true);
  connection.renew({ ...grant, token: "token-2" });
  await settle();
  expect(FakeRoom.all).toHaveLength(1);
});
