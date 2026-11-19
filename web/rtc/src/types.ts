// SPDX-License-Identifier: AGPL-3.0-only
// The boundary of the audio of a call (ADR-0101 §7): one RoomSession per joined session of a room, whatever carries
// its audio (P2P, LiveKit or Sora on the web; a native implementation behind the same interface later). It knows
// nothing of mycroblog: its caller hands it the grant, the roster of each room state and the signals of P2P, and
// hears what it does through its events. It holds no React and no DOM subtree, so a session outlives every screen.

/** What carries the audio of a room (ADR-0101 §1). */
export type RoomKind = "p2p" | "livekit" | "sora";

/** A STUN or TURN server, as the grant names it (the browser's RTCIceServer). */
export type IceServer = {
  readonly urls: readonly string[];
  readonly username?: string | undefined;
  readonly credential?: string | undefined;
};

/** What a session needs to connect to the audio of a room (ADR-0101 §2 `Grant`). */
export type RoomGrant = {
  /** What carries the audio: `p2p`, `livekit` or `sora`; any other is refused by `createRoomSession`. */
  readonly kind: string;
  /** The SFU's endpoint; absent for P2P. */
  readonly url?: string | undefined;
  /** The SFU's access token (Sora: the signed join value); absent for P2P. */
  readonly token?: string | undefined;
  /** The options of the kind (ADR-0101 §12): Sora's `role`, `channel_id`, `signaling_urls`, `access_token`. */
  readonly options: Readonly<Record<string, string>>;
  readonly iceServers: readonly IceServer[];
  /** Whether the connections go through TURN only (`iceTransportPolicy: "relay"`, ADR-0101 §5). */
  readonly relayOnly: boolean;
  /** When the grant expires, in milliseconds since the epoch: the caller renews it before then. */
  readonly expiresAt: number;
};

/** Another live session of the room, as the latest room state has it. */
export type RoomMember = {
  readonly session: string;
  /** Whether the participant may speak: only a speaker's audio is played (ADR-0101 §4). */
  readonly speaker: boolean;
};

/** The room as the latest room state has it: whether this session speaks, and the other live sessions. */
export type RoomRoster = { readonly speaker: boolean; readonly members: readonly RoomMember[] };

/** A signal of P2P (ADR-0101 §4). */
export type SignalKind = "offer" | "answer" | "candidate" | "restart";

/** `sdp` of an offer or an answer, `candidate` (the JSON of an RTCIceCandidateInit) of a candidate. */
export type Signal = {
  readonly kind: SignalKind;
  readonly sdp?: string | undefined;
  readonly candidate?: string | undefined;
};

/** A signal this session sends to another. */
export type OutgoingSignal = Signal & { readonly to: string };

/** A signal another session sent to this one. */
export type IncomingSignal = Signal & { readonly from: string };

/**
 * How P2P reaches the other sessions: `send` carries a signal out (the caller posts it to the server), and the caller
 * feeds each signal that comes in to `RoomSession.receiveSignal`. A failed send is retried by the session.
 */
export type SignalingPort = { readonly send: (signal: OutgoingSignal) => Promise<void> };

/**
 * How the media path stands: `connecting` until it is up, `connected`, `reconnecting` while it recovers by itself (an
 * ICE restart, the SDK's own reconnection), `lost` when it went away and needs a new grant (`reconnect`), and
 * `closed` once the session closed or failed.
 */
export type ConnectionState = "connecting" | "connected" | "reconnecting" | "lost" | "closed";

/** The media path to one other session: P2P has one per pair, an SFU one per participant it reports. */
export type ParticipantLink = {
  readonly session: string;
  readonly state: "connecting" | "connected" | "reconnecting";
};

/** The connection of the session, and of each participant it reaches. */
export type RoomConnection = { readonly state: ConnectionState; readonly participants: readonly ParticipantLink[] };

/**
 * The quality of the media path from WebRTC's statistics, sampled periodically (ADR-0101 §7): the round-trip time
 * and the jitter in milliseconds, and the fraction of the packets lost since the previous sample (0–1); null where
 * the statistics do not say.
 */
export type ConnectionQuality = {
  readonly rtt: number | null;
  readonly jitter: number | null;
  readonly packetLoss: number | null;
};

/**
 * Why a session ended by itself: `unavailable` when what carries the audio cannot run (its SDK did not load),
 * `removed` when the media server removed the session (LiveKit: removed, replaced by the same identity, or the room
 * deleted). The session is closed after it.
 */
export type RoomFailureReason = "unavailable" | "removed";

/** The terminal failure of a session. */
export class RoomSessionFailure extends Error {
  override name = "RoomSessionFailure";
  readonly reason: RoomFailureReason;

  constructor(reason: RoomFailureReason, cause?: unknown) {
    super(`rtc: the session ended: ${reason}`, cause === undefined ? undefined : { cause });
    this.reason = reason;
  }
}

/** What a session tells its caller; each listener hears only changes. */
export type RoomSessionEvents = {
  connection: (connection: RoomConnection) => void;
  /** The sessions talking now, this session's own among them while its microphone sends. */
  talking: (sessions: ReadonlySet<string>) => void;
  /** True when the browser did not let the audio play (`resumePlayback` from a gesture), false once it plays. */
  playbackBlocked: (blocked: boolean) => void;
  quality: (quality: ConnectionQuality) => void;
  failed: (failure: RoomSessionFailure) => void;
};

/** Everything the events have said, as one value that changes identity only when something changed. */
export type RoomSessionSnapshot = {
  readonly connection: RoomConnection;
  readonly talking: ReadonlySet<string>;
  readonly playbackBlocked: boolean;
  /** Whether the microphone sends: it is on and the session speaks. */
  readonly microphone: boolean;
  readonly quality: ConnectionQuality | null;
  readonly failure: RoomSessionFailure | null;
};

/** What a session can do where it runs. */
export type RoomSessionCapabilities = {
  /** Whether `setOutputDevice` can choose where the audio plays (setSinkId). */
  readonly outputSelection: boolean;
};

/**
 * The connection of one joined session to the audio of a room. It reconnects by itself (an ICE restart on a failure
 * or a network change, the SDK's own reconnection) and takes the microphone again when its device goes away; what
 * it cannot recover from alone it says as `lost` (give it a new grant with `reconnect`) or as `failed`.
 */
export interface RoomSession {
  readonly kind: RoomKind;
  /** This session's ID. */
  readonly session: string;
  readonly capabilities: RoomSessionCapabilities;
  /** Takes the roster of the latest room state: whom it connects to, whose audio it plays, whether it may send. */
  update(roster: RoomRoster): void;
  /**
   * Turns the microphone on (asking the browser for it the first time) or off. Turning it on rejects when the browser
   * does not give it, or when the media server does not let the session send in time. Only a speaker sends: call it
   * after `update` made the session a speaker. Calls apply one after another, the latest one last.
   */
  setMicrophone(on: boolean): Promise<void>;
  /** Plays the audio on the output device `deviceId` (`""` for the default); rejects without `outputSelection`. */
  setOutputDevice(deviceId: string): Promise<void>;
  /**
   * Takes a renewed grant on the live connections (ADR-0101 §5): P2P sets its ICE servers on each connection and
   * restarts ICE with them; an SFU keeps it, and joins again with it when its connection went away.
   */
  renew(grant: RoomGrant): void;
  /** Connects again with a new grant, for the same session (the server asked for it, or the connection was lost). */
  reconnect(grant: RoomGrant): void;
  /** Takes a signal of P2P from another session; other kinds ignore it. */
  receiveSignal(signal: IncomingSignal): void;
  /** Plays the audio the browser blocked, from the user's gesture; rejects while some stays blocked. */
  resumePlayback(): Promise<void>;
  snapshot(): RoomSessionSnapshot;
  /** Listens to `event` until the function it returns is called. */
  on<E extends keyof RoomSessionEvents>(event: E, listener: RoomSessionEvents[E]): () => void;
  /**
   * Leaves: stops every track, closes every connection, audio and the environment's audio context. Idempotent; the
   * session says nothing after it but its last `talking` (nobody, if anyone talked) and `connection` (closed).
   */
  close(): void;
}

/** The audio of one remote participant. */
export type RemoteAudio = {
  /** Plays `stream`; rejects with a `NotAllowedError` when the browser does not let it play without a gesture. */
  play: (stream: MediaStream) => Promise<void>;
  /** Plays again after the browser blocked it, from the user's gesture; rejects while it stays blocked. */
  resume: () => Promise<void>;
  /** Whether the user hears it. */
  setAudible: (audible: boolean) => void;
  close: () => void;
};

/** The level of the audio of one stream, for telling who is talking. */
export type LevelMeter = {
  /** The level now: the RMS of the latest samples, from 0 (silence) to 1. */
  level: () => number;
  close: () => void;
};

/** The SDKs, loaded only once a grant names them (ADR-0101 §15). */
export type LiveKitSdk = typeof import("livekit-client");
export type SoraSdk = (typeof import("sora-js-sdk"))["default"];

/**
 * What a session takes from where it runs; `browserEnvironment` gives the browser's, and the tests give fakes. A
 * session owns its environment: `close` releases it.
 */
export type RoomEnvironment = {
  createPeerConnection: (configuration: RTCConfiguration) => RTCPeerConnection;
  /** The microphone: echo cancellation, noise suppression and automatic gain control, audio only (ADR-0101 §7). */
  getMicrophone: () => Promise<MediaStream>;
  /** A stream of `tracks` (a remote track that came without its stream). */
  createStream: (tracks: readonly MediaStreamTrack[]) => MediaStream;
  createRemoteAudio: () => RemoteAudio;
  /** Measures the level of `stream`; absent, or null, where it cannot (then nobody shows as talking). */
  createLevelMeter?: ((stream: MediaStream) => LevelMeter | null) | undefined;
  /** Plays every remote audio on the output device `deviceId`; absent where it cannot. */
  setOutputDevice?: ((deviceId: string) => Promise<void>) | undefined;
  /** The IDs of the audio inputs there are now. */
  audioInputs?: (() => Promise<readonly string[]>) | undefined;
  /** Calls `onChange` when the media devices change, until the function it returns is called. */
  watchDevices?: ((onChange: () => void) => () => void) | undefined;
  /** Calls `onChange` when the network comes back or changes, until the function it returns is called. */
  watchNetwork?: ((onChange: () => void) => () => void) | undefined;
  /** Loads livekit-client; by default `import("livekit-client")`. */
  loadLiveKit?: (() => Promise<LiveKitSdk>) | undefined;
  /** Loads sora-js-sdk; by default `import("sora-js-sdk")`. */
  loadSora?: (() => Promise<SoraSdk>) | undefined;
  /** Releases what the environment holds (its audio context). */
  close?: (() => void) | undefined;
};

/** What `createRoomSession` takes. */
export type RoomSessionOptions = {
  readonly grant: RoomGrant;
  /** This session's ID: P2P's side of each pair, the SFU's identity (LiveKit) and client_id (Sora). */
  readonly session: string;
  /** How P2P sends its signals; required for P2P, unused by an SFU. */
  readonly signaling?: SignalingPort | undefined;
  readonly environment: RoomEnvironment;
  /** How often the quality is sampled, in milliseconds (5000 by default). */
  readonly qualityInterval?: number | undefined;
};

/** The configuration of an RTCPeerConnection from a grant (ADR-0101 §5). */
export function rtcConfiguration(grant: RoomGrant): RTCConfiguration {
  return {
    iceServers: grant.iceServers.map((server) => ({
      urls: [...server.urls],
      ...(server.username === undefined ? {} : { username: server.username }),
      ...(server.credential === undefined ? {} : { credential: server.credential }),
    })),
    iceTransportPolicy: grant.relayOnly ? "relay" : "all",
  };
}
