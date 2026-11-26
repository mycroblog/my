// SPDX-License-Identifier: AGPL-3.0-only
// Fakes of what a RoomSession takes from the browser, for the tests on Bun (which has no WebRTC): an
// RTCPeerConnection that models the JSEP states that the perfect negotiation pattern meets (offers, answers, the
// implicit rollback of an offer in glare, transceivers and their directions, negotiationneeded, restartIce,
// setConfiguration) and writes session descriptions with one `m=audio` line and the direction of each transceiver; a
// microphone whose tracks end and change device; the remote audio and the level meters; the WebSocket to a media node,
// on which the test plays the node; and an environment of them all. Nothing sends media: a remote description that
// sends audio fires `track` with a fake stream. A remote offer's `a=mid:` lines name the transceivers it makes.
import type { RoomEnvironment } from "../types";

type Direction = RTCRtpTransceiverDirection;

/** What sends: the direction of the other side, reversed. */
function reverse(direction: Direction): Direction {
  if (direction === "sendonly") return "recvonly";
  if (direction === "recvonly") return "sendonly";
  return direction;
}

/** The direction of an answer: what the answerer wants, within what the offer allows. */
function intersect(local: Direction, offered: Direction): Direction {
  const allowed = reverse(offered);
  const send = (local === "sendrecv" || local === "sendonly") && (allowed === "sendrecv" || allowed === "sendonly");
  const recv = (local === "sendrecv" || local === "recvonly") && (allowed === "sendrecv" || allowed === "recvonly");
  if (send && recv) return "sendrecv";
  if (send) return "sendonly";
  if (recv) return "recvonly";
  return "inactive";
}

/** The audio directions of a session description, in the order of its m= lines. */
export function sdpDirections(sdp: string): Direction[] {
  return [...sdp.matchAll(/^a=(sendrecv|sendonly|recvonly|inactive)$/gmu)].map((match) => match[1] as Direction);
}

export class FakeTrack {
  readonly kind = "audio";
  enabled = true;
  stopped = false;
  readyState: MediaStreamTrackState = "live";
  deviceId: string;
  onended: (() => void) | null = null;

  constructor(deviceId = "mic-1") {
    this.deviceId = deviceId;
  }

  getSettings(): MediaTrackSettings {
    return { deviceId: this.deviceId };
  }

  stop(): void {
    this.stopped = true;
    this.readyState = "ended";
  }

  /** The device went away: the track ends by itself. */
  end(): void {
    this.readyState = "ended";
    this.onended?.();
  }
}

let streamsMade = 0;

export class FakeStream {
  readonly id: string;
  readonly tracks: FakeTrack[];
  constructor(tracks: FakeTrack[] = [new FakeTrack()], id = `stream-${String(++streamsMade)}`) {
    this.tracks = tracks;
    this.id = id;
  }
  getTracks(): FakeTrack[] {
    return this.tracks;
  }
  getAudioTracks(): FakeTrack[] {
    return this.tracks;
  }
}

class FakeSender {
  track: FakeTrack | null = null;
  replaceTrack(track: FakeTrack | null): Promise<void> {
    this.track = track;
    return Promise.resolve();
  }
}

class FakeTransceiver {
  readonly #pc: FakePeerConnection;
  #direction: Direction;
  readonly sender = new FakeSender();
  readonly receiver = { track: new FakeTrack() };
  /** The mid of the m= section, once a description named it. */
  mid: string | null = null;

  constructor(pc: FakePeerConnection, direction: Direction) {
    this.#pc = pc;
    this.#direction = direction;
  }

  get direction(): Direction {
    return this.#direction;
  }

  set direction(direction: Direction) {
    if (direction === this.#direction) return;
    this.#direction = direction;
    this.#pc.needNegotiation();
  }
}

type Description = { type: RTCSdpType; sdp: string };

let connectionsMade = 0;

/** A statistics report: a Map of the stats by their IDs, as RTCStatsReport is. */
export function statsReport(
  stats: readonly (Record<string, unknown> & { id: string; type: string })[],
): RTCStatsReport {
  // A Map has every member of RTCStatsReport's maplike.
  const report: RTCStatsReport = new Map(stats.map((stat) => [stat.id, stat]));
  return report;
}

/** What `promise` rejected with, or undefined when it resolved. */
export function rejection(promise: Promise<unknown>): Promise<unknown> {
  return promise.then(
    () => undefined,
    (error: unknown) => error,
  );
}

export class FakePeerConnection {
  /** Every connection made, in order. */
  static all: FakePeerConnection[] = [];
  /** Whether setConfiguration throws, as a browser that refuses the change does. */
  static refusesConfiguration = false;

  configuration: RTCConfiguration;
  readonly configurations: RTCConfiguration[] = [];
  signalingState: RTCSignalingState = "stable";
  iceConnectionState: RTCIceConnectionState = "new";
  localDescription: Description | null = null;
  remoteDescription: Description | null = null;
  readonly transceivers: FakeTransceiver[] = [];
  readonly candidates: RTCIceCandidateInit[] = [];
  restarts = 0;
  closed = false;
  /** What getStats answers. */
  stats: RTCStatsReport = statsReport([]);
  onnegotiationneeded: (() => void) | null = null;
  onicecandidate: ((event: { candidate: { toJSON: () => RTCIceCandidateInit } | null }) => void) | null = null;
  ontrack: ((event: { track: FakeTrack; streams: FakeStream[] }) => void) | null = null;
  oniceconnectionstatechange: (() => void) | null = null;
  #needed = false;
  #restart = false;
  #version = 0;
  /** The ICE generation, which a restart moves on. */
  #generation = 0;
  readonly #id = ++connectionsMade;

  constructor(configuration: RTCConfiguration) {
    this.configuration = configuration;
    FakePeerConnection.all.push(this);
  }

  #ufrag(): string {
    return `u${String(this.#id)}g${String(this.#generation)}`;
  }

  /** Marks that negotiation is needed, and fires negotiationneeded once the connection is stable. */
  needNegotiation(): void {
    this.#needed = true;
    this.#fireNeeded();
  }

  #fireNeeded(): void {
    queueMicrotask(() => {
      if (this.closed || !this.#needed || this.signalingState !== "stable") return;
      this.onnegotiationneeded?.();
    });
  }

  addTransceiver(_kind: "audio", init: { direction: Direction }): FakeTransceiver {
    const transceiver = new FakeTransceiver(this, init.direction);
    this.transceivers.push(transceiver);
    this.needNegotiation();
    return transceiver;
  }

  getTransceivers(): FakeTransceiver[] {
    return this.transceivers;
  }

  getStats(): Promise<RTCStatsReport> {
    return Promise.resolve(this.stats);
  }

  setConfiguration(configuration: RTCConfiguration): void {
    if (FakePeerConnection.refusesConfiguration) throw new DOMException("refused", "InvalidModificationError");
    this.configuration = configuration;
    this.configurations.push(configuration);
  }

  restartIce(): void {
    this.restarts++;
    this.#restart = true;
    this.#generation++;
    this.needNegotiation();
  }

  #sdp(directions: Direction[]): string {
    this.#version++;
    const restart = this.#restart ? "a=ice-options:restart\r\n" : "";
    const media = directions.map((d) => `m=audio 9 UDP/TLS/RTP/SAVPF 111\r\na=${d}\r\n`).join("");
    return `v=0\r\no=- 1 ${String(this.#version)} IN IP4 0.0.0.0\r\na=ice-ufrag:${this.#ufrag()}\r\n${restart}${media}`;
  }

  setLocalDescription(description?: { type: "rollback" }): Promise<void> {
    if (this.closed) return Promise.reject(new DOMException("closed", "InvalidStateError"));
    if (description?.type === "rollback") {
      this.signalingState = "stable";
      this.localDescription = null;
      this.#fireNeeded();
      return Promise.resolve();
    }
    if (this.signalingState === "have-remote-offer") {
      const offered = sdpDirections(this.remoteDescription?.sdp ?? "");
      const sdp = this.#sdp(this.transceivers.map((t, i) => intersect(t.direction, offered[i] ?? "inactive")));
      this.localDescription = { type: "answer", sdp };
      this.signalingState = "stable";
      this.#needed = false;
      this.#restart = false;
      this.#candidate();
      this.#fireNeeded();
      return Promise.resolve();
    }
    this.localDescription = { type: "offer", sdp: this.#sdp(this.transceivers.map((t) => t.direction)) };
    this.signalingState = "have-local-offer";
    this.#needed = false;
    this.#candidate();
    return Promise.resolve();
  }

  #candidate(): void {
    queueMicrotask(() => {
      if (this.closed) return;
      const usernameFragment = this.#ufrag();
      this.onicecandidate?.({
        candidate: {
          toJSON: () => ({
            candidate: "candidate:1 1 udp 1 192.0.2.1 9 typ host",
            sdpMid: "0",
            sdpMLineIndex: 0,
            usernameFragment,
          }),
        },
      });
    });
  }

  setRemoteDescription(description: Description): Promise<void> {
    if (this.closed) return Promise.reject(new DOMException("closed", "InvalidStateError"));
    const directions = sdpDirections(description.sdp);
    if (description.type === "offer") {
      // An offer in have-local-offer rolls the local one back first (JSEP §5.10).
      if (this.signalingState === "have-local-offer") this.localDescription = null;
      while (this.transceivers.length < directions.length)
        this.transceivers.push(new FakeTransceiver(this, "recvonly"));
      const mids = [...description.sdp.matchAll(/^a=mid:(\S+)$/gmu)].map((match) => match[1] ?? null);
      this.transceivers.forEach((transceiver, i) => {
        transceiver.mid ??= mids[i] ?? null;
      });
      this.remoteDescription = description;
      this.signalingState = "have-remote-offer";
    } else {
      if (this.signalingState !== "have-local-offer")
        return Promise.reject(new DOMException("no offer", "InvalidStateError"));
      this.remoteDescription = description;
      this.signalingState = "stable";
      this.#restart = false;
      this.#fireNeeded();
    }
    if (directions.some((d) => d === "sendrecv" || d === "sendonly")) {
      queueMicrotask(() => {
        if (!this.closed) this.ontrack?.({ track: new FakeTrack(), streams: [new FakeStream()] });
      });
    }
    return Promise.resolve();
  }

  addIceCandidate(candidate: RTCIceCandidateInit): Promise<void> {
    if (this.remoteDescription === null)
      return Promise.reject(new DOMException("no remote description", "InvalidStateError"));
    // A candidate of another ICE generation than the remote description's (RFC 8839 §5.4, JSEP §4.1.17).
    const remote = /^a=ice-ufrag:(\S+)/mu.exec(this.remoteDescription.sdp)?.[1];
    if (
      candidate.usernameFragment !== undefined &&
      candidate.usernameFragment !== null &&
      candidate.usernameFragment !== remote
    )
      return Promise.reject(new DOMException("unknown ufrag", "OperationError"));
    this.candidates.push(candidate);
    return Promise.resolve();
  }

  /** Moves the ICE connection to `state`, as the network does. */
  setIce(state: RTCIceConnectionState): void {
    this.iceConnectionState = state;
    this.oniceconnectionstatechange?.();
  }

  close(): void {
    this.closed = true;
    this.signalingState = "closed";
  }
}

/** The audio of a remote participant, which records what the session did with it. */
export class FakeRemoteAudio {
  static all: FakeRemoteAudio[] = [];
  /** Whether play rejects as a browser that blocks autoplay does. */
  static blocked = false;
  audible = false;
  playing = false;
  closed = false;
  stream: unknown = null;

  constructor() {
    FakeRemoteAudio.all.push(this);
  }

  play = (stream: MediaStream): Promise<void> => {
    this.stream = stream;
    if (FakeRemoteAudio.blocked) return Promise.reject(new DOMException("blocked", "NotAllowedError"));
    this.playing = true;
    return Promise.resolve();
  };

  resume = (): Promise<void> => {
    if (FakeRemoteAudio.blocked) return Promise.reject(new DOMException("blocked", "NotAllowedError"));
    this.playing = true;
    return Promise.resolve();
  };

  setAudible = (audible: boolean): void => {
    this.audible = audible;
  };

  close = (): void => {
    this.closed = true;
  };
}

/** A LevelMeter whose level the test sets. */
export class FakeLevelMeter {
  static all: FakeLevelMeter[] = [];
  readonly stream: unknown;
  value = 0;
  closed = false;

  constructor(stream: unknown) {
    this.stream = stream;
    FakeLevelMeter.all.push(this);
  }

  /** The meters not closed yet. */
  static open(): FakeLevelMeter[] {
    return FakeLevelMeter.all.filter((meter) => !meter.closed);
  }

  /** The open meter of `stream`. */
  static of(stream: unknown): FakeLevelMeter {
    const found = FakeLevelMeter.open().find((meter) => meter.stream === stream);
    if (found === undefined) throw new Error("no open meter of the stream");
    return found;
  }

  level = (): number => this.value;

  close = (): void => {
    this.closed = true;
  };
}

/** A WebSocket to a media node, on which the test plays the node: what the session sends is kept parsed. */
export class FakeSocket {
  /** Every socket opened, in order. */
  static all: FakeSocket[] = [];
  readonly url: string;
  readyState = 0;
  /** The messages the session sent, parsed. */
  readonly sent: Record<string, unknown>[] = [];
  /** The code the session closed the socket with, or null while it did not. */
  closedWith: number | null = null;
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: unknown }) => void) | null = null;
  onclose: ((event: { code: number }) => void) | null = null;
  onerror: (() => void) | null = null;

  constructor(url: string) {
    this.url = url;
    FakeSocket.all.push(this);
  }

  /** The latest socket opened. */
  static latest(): FakeSocket {
    const socket = FakeSocket.all.at(-1);
    if (socket === undefined) throw new Error("no socket was opened");
    return socket;
  }

  /** The ticket of the URL it was opened with. */
  get ticket(): string | null {
    return new URL(this.url).searchParams.get("ticket");
  }

  /** The messages sent of `type`. */
  ofType(type: string): Record<string, unknown>[] {
    return this.sent.filter((message) => message["type"] === type);
  }

  /** The node accepted the WebSocket. */
  open(): void {
    this.readyState = 1;
    this.onopen?.();
  }

  /** The node sends `message` (its JSON), or a text frame as it is. */
  receive(message: Record<string, unknown> | string): void {
    this.onmessage?.({ data: typeof message === "string" ? message : JSON.stringify(message) });
  }

  /** The node closes the WebSocket, or it drops. */
  drop(code = 1006): void {
    this.readyState = 3;
    this.onclose?.({ code });
  }

  send(data: string): void {
    if (this.readyState !== 1) throw new DOMException("not open", "InvalidStateError");
    this.sent.push(JSON.parse(data) as Record<string, unknown>);
  }

  close(code = 1005): void {
    if (this.readyState === 3) return;
    this.readyState = 3;
    this.closedWith = code;
    queueMicrotask(() => this.onclose?.({ code }));
  }
}

/**
 * The SDP of a media node's offer: the m= section that receives the session's microphone (mid 0, `recvonly`), then one
 * that sends each speaker (mids 1 and up, `sendonly`), under the ICE username fragment `ufrag`.
 */
export function nodeOffer(speakers: number, ufrag = "node1"): string {
  const section = (mid: number, direction: string) =>
    `m=audio 9 UDP/TLS/RTP/SAVPF 111\r\na=mid:${String(mid)}\r\na=${direction}\r\n`;
  const media = [section(0, "recvonly"), ...Array.from({ length: speakers }, (_, i) => section(i + 1, "sendonly"))];
  return `v=0\r\no=- 1 1 IN IP4 192.0.2.10\r\na=ice-lite\r\na=ice-ufrag:${ufrag}\r\n${media.join("")}`;
}

/** The microphone of the fake environment: each grant a new stream on the current device. */
export const microphone = {
  streams: [] as FakeStream[],
  /** The device the next stream is taken from. */
  device: "mic-1",
  /** The audio inputs there are. */
  inputs: ["mic-1"] as string[],
  /** Whether the browser refuses the microphone. */
  refused: false,
  latest(): FakeStream {
    const stream = microphone.streams.at(-1);
    if (stream === undefined) throw new Error("no microphone was taken");
    return stream;
  },
};

const deviceWatchers = new Set<() => void>();
const networkWatchers = new Set<() => void>();

/** What the fake environments were asked, and the changes they let the test make to every session. */
export const browser = {
  outputDevices: [] as string[],
  closed: 0,
  /** The media devices changed (`devicechange`). */
  deviceChange: (): void => {
    for (const watcher of [...deviceWatchers]) watcher();
  },
  /** The network came back or changed. */
  networkChange: (): void => {
    for (const watcher of [...networkWatchers]) watcher();
  },
};

/** An environment of the fakes; `overrides` replaces or removes what it gives. */
export function fakeEnvironment(overrides: Partial<RoomEnvironment> = {}): RoomEnvironment {
  return {
    createPeerConnection: (configuration) => new FakePeerConnection(configuration) as unknown as RTCPeerConnection,
    getMicrophone: () => {
      if (microphone.refused) return Promise.reject(new DOMException("denied", "NotAllowedError"));
      const stream = new FakeStream([new FakeTrack(microphone.device)]);
      microphone.streams.push(stream);
      return Promise.resolve(stream as unknown as MediaStream);
    },
    createStream: (tracks) => new FakeStream([...tracks] as unknown as FakeTrack[]) as unknown as MediaStream,
    createRemoteAudio: () => new FakeRemoteAudio(),
    createLevelMeter: (stream) => new FakeLevelMeter(stream),
    setOutputDevice: (deviceId) => {
      browser.outputDevices.push(deviceId);
      return Promise.resolve();
    },
    audioInputs: () => Promise.resolve([...microphone.inputs]),
    watchDevices: (onChange) => {
      deviceWatchers.add(onChange);
      return () => {
        deviceWatchers.delete(onChange);
      };
    },
    watchNetwork: (onChange) => {
      networkWatchers.add(onChange);
      return () => {
        networkWatchers.delete(onChange);
      };
    },
    openSocket: (url) => new FakeSocket(url) as unknown as WebSocket,
    close: () => {
      browser.closed++;
    },
    ...overrides,
  };
}

/** Forgets the connections, the audio and the microphone of the previous test. */
export function resetFakes(): void {
  FakePeerConnection.all = [];
  FakePeerConnection.refusesConfiguration = false;
  FakeRemoteAudio.all = [];
  FakeRemoteAudio.blocked = false;
  FakeLevelMeter.all = [];
  FakeSocket.all = [];
  microphone.streams = [];
  microphone.device = "mic-1";
  microphone.inputs = ["mic-1"];
  microphone.refused = false;
  browser.outputDevices = [];
  browser.closed = 0;
  deviceWatchers.clear();
  networkWatchers.clear();
}

/** Lets the promises and microtasks of the fakes and the session run. */
export async function settle(): Promise<void> {
  for (let i = 0; i < 300; i++) await Promise.resolve();
}
