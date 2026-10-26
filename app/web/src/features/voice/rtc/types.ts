// SPDX-License-Identifier: AGPL-3.0-only
// The method-neutral boundary of the audio of a call (ADR-0101 §7, ADR-0102): one connection of the user's session to
// the audio of a room, whatever the method of the server (p2p here; livekit and sora behind the same boundary). It is
// plain TypeScript without React, and only the hooks of features/voice use it. It knows nothing of the client API:
// the hooks hand it the grant, the participants of each `state`, and the signals of the room stream, and it sends its
// signals through `sendSignal`.

/** A STUN or TURN server, as the grant names it (the browser's RTCIceServer). */
type VoiceIceServer = { urls: readonly string[]; username?: string | undefined; credential?: string | undefined };

/** What the session needs to connect to the audio: the method, its endpoint and token, and the ICE servers. */
export type VoiceGrant = {
  /** The method of the server: `p2p`, `livekit`, or `sora`. */
  kind: string;
  url?: string | undefined;
  token?: string | undefined;
  options: Readonly<Record<string, string>>;
  iceServers: readonly VoiceIceServer[];
  /** Whether the connections go through TURN only (`iceTransportPolicy: "relay"`). */
  relayOnly: boolean;
  /** When the grant expires, in milliseconds since the epoch. */
  expiresAt: number;
};

/** Another live session of the room. */
export type VoiceMember = {
  session: string;
  /** The account of the participant. */
  account: string;
  /** Whether the participant is a speaker in the latest `state`. */
  speaker: boolean;
};

/** The room as the latest `state` has it: whether the user speaks, and the other live sessions. */
export type VoiceRoster = { speaker: boolean; members: readonly VoiceMember[] };

type VoiceSignalKind = "offer" | "answer" | "candidate" | "restart";

/** A signal of P2P: `sdp` of an offer or an answer, `candidate` (the JSON of an RTCIceCandidateInit) of a candidate. */
export type VoiceSignal = { kind: VoiceSignalKind; sdp?: string | undefined; candidate?: string | undefined };

export type OutgoingVoiceSignal = VoiceSignal & { to: string };

export type IncomingVoiceSignal = VoiceSignal & { from: string };

/** The audio of one remote participant. */
export type RemoteAudio = {
  /** Plays `stream`; rejects when the browser does not let it play without a gesture. */
  play: (stream: MediaStream) => Promise<void>;
  /** Plays the audio again after the browser blocked it, from the user's gesture; rejects while it stays blocked. */
  resume: () => Promise<void>;
  /** Whether the user hears it: only a speaker of the latest `state` is heard (ADR-0101 §4). */
  setAudible: (audible: boolean) => void;
  close: () => void;
};

/** What a connection takes from the browser; the tests give fakes. */
export type VoiceEnvironment = {
  createPeerConnection: (configuration: RTCConfiguration) => RTCPeerConnection;
  /** The microphone, with echo cancellation, noise suppression, and automatic gain control, audio only. */
  getMicrophone: () => Promise<MediaStream>;
  createRemoteAudio: () => RemoteAudio;
};

/** What a connection tells the hooks. */
type VoiceConnectionEvents = {
  /** Sends a signal to another session (P2P). A failure is the hooks' to report; the connection recovers by ICE restarts. */
  sendSignal: (signal: OutgoingVoiceSignal) => Promise<void>;
  /** The browser did not let the audio play: the user starts it with `resumePlayback` from a button. */
  onPlaybackBlocked: () => void;
};

/**
 * The connection of the user's session to the audio of a room. `update` sets whether the user speaks and whom the
 * session connects to; `setMicrophone` turns the microphone on (asking the browser for it the first time) or off;
 * `renew` takes a new grant on the live connections and `reconnect` connects again with it; `close` leaves.
 */
export type VoiceConnection = {
  update: (roster: VoiceRoster) => void;
  /**
   * Turns the microphone on or off; turning it on rejects when the browser does not give it. Only a speaker sends:
   * call it after `update` has made the user a speaker. Calls apply one after another, the latest one last.
   */
  setMicrophone: (on: boolean) => Promise<void>;
  /**
   * Takes a new grant on the live connections: for P2P its ICE servers (setConfiguration), and a restart of ICE with
   * them unless `restartIce` is false.
   */
  renew: (grant: VoiceGrant, options?: { restartIce?: boolean }) => void;
  /** Connects again with a new grant, for the same session (the server asked for it, ADR-0101 §2). */
  reconnect: (grant: VoiceGrant) => void;
  receive: (signal: IncomingVoiceSignal) => void;
  /** Plays the audio that the browser blocked, from the user's gesture; rejects while some stays blocked. */
  resumePlayback: () => Promise<void>;
  close: () => void;
};

export type VoiceConnectionOptions = {
  /** The user's session. */
  session: string;
  grant: VoiceGrant;
  environment: VoiceEnvironment;
  events: VoiceConnectionEvents;
};

/** The configuration of an RTCPeerConnection from a grant (ADR-0101 §5). */
export function rtcConfiguration(grant: VoiceGrant): RTCConfiguration {
  return {
    iceServers: grant.iceServers.map((server) => ({
      urls: [...server.urls],
      ...(server.username === undefined ? {} : { username: server.username }),
      ...(server.credential === undefined ? {} : { credential: server.credential }),
    })),
    iceTransportPolicy: grant.relayOnly ? "relay" : "all",
  };
}
