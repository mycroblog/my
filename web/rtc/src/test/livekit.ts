// SPDX-License-Identifier: AGPL-3.0-only
// A fake of the parts of livekit-client that the LiveKit session uses: a Room that records how it was joined and left,
// whose local participant LiveKit lets publish or not, and whose events the test emits; `loadFakeLiveKit` stands in
// for the SDK's import.
import { mock } from "bun:test";
import type { LiveKitSdk } from "../types";
import { statsReport } from "./fake";

type Listener = (...args: unknown[]) => void;

export const DisconnectReason = {
  UNKNOWN_REASON: 0,
  CLIENT_INITIATED: 1,
  DUPLICATE_IDENTITY: 2,
  SERVER_SHUTDOWN: 3,
  PARTICIPANT_REMOVED: 4,
  ROOM_DELETED: 5,
} as const;

class FakeLocalParticipant {
  permissions: { canPublish: boolean } | undefined = { canPublish: false };
  microphone: { track: { id: string; getRTCStatsReport: () => Promise<RTCStatsReport> } } | undefined;
  readonly setMicrophoneEnabled = mock((enabled: boolean) => {
    if (enabled) this.microphone = { track: { id: "mic", getRTCStatsReport: () => Promise.resolve(statsReport([])) } };
    return Promise.resolve(this.microphone);
  });
  readonly unpublishTrack = mock((_track: unknown, _stop?: boolean) => {
    this.microphone = undefined;
    return Promise.resolve(undefined);
  });
  getTrackPublication(source: string) {
    return source === "microphone" ? this.microphone : undefined;
  }
}

export class FakeRoom {
  static all: FakeRoom[] = [];
  static failNext = false;
  readonly options: unknown;
  readonly localParticipant = new FakeLocalParticipant();
  readonly remoteParticipants = new Map<string, { identity: string }>();
  readonly listeners = new Map<string, Listener[]>();
  readonly connect = mock((_url: string, _token: string, _options?: unknown) =>
    FakeRoom.failNext ? Promise.reject(new Error("refused")) : Promise.resolve(),
  );
  readonly disconnect = mock(() => {
    this.emit("disconnected", DisconnectReason.CLIENT_INITIATED);
    return Promise.resolve();
  });

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

  /** A remote participant joins, and its audio track is subscribed; the track's stats report `stats`. */
  subscribe(identity: string, sid: string, stats: RTCStatsReport = statsReport([])): void {
    this.join(identity);
    this.emit(
      "trackSubscribed",
      { kind: "audio", mediaStreamTrack: { id: sid }, getRTCStatsReport: () => Promise.resolve(stats) },
      { trackSid: sid },
      { identity },
    );
  }

  join(identity: string): void {
    if (this.remoteParticipants.has(identity)) return;
    const participant = { identity };
    this.remoteParticipants.set(identity, participant);
    this.emit("participantConnected", participant);
  }

  leave(identity: string): void {
    const participant = this.remoteParticipants.get(identity);
    this.remoteParticipants.delete(identity);
    this.emit("participantDisconnected", participant);
  }
}

const sdk = {
  Room: FakeRoom,
  RoomEvent: {
    TrackSubscribed: "trackSubscribed",
    TrackUnsubscribed: "trackUnsubscribed",
    ParticipantConnected: "participantConnected",
    ParticipantDisconnected: "participantDisconnected",
    ParticipantPermissionsChanged: "participantPermissionsChanged",
    ActiveSpeakersChanged: "activeSpeakersChanged",
    Reconnecting: "reconnecting",
    Reconnected: "reconnected",
    Disconnected: "disconnected",
  },
  Track: { Kind: { Audio: "audio" }, Source: { Microphone: "microphone" } },
  DisconnectReason,
};

/** Loads the fake SDK, as the session's `import("livekit-client")` would. */
export function loadFakeLiveKit(): Promise<LiveKitSdk> {
  return Promise.resolve(sdk as unknown as LiveKitSdk);
}

/** The room the session made `index`-th. */
export function room(index = 0): FakeRoom {
  const found = FakeRoom.all[index];
  if (found === undefined) throw new Error(`no room ${String(index)}`);
  return found;
}

export function resetFakeLiveKit(): void {
  FakeRoom.all = [];
  FakeRoom.failNext = false;
}
