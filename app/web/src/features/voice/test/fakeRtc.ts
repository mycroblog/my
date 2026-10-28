// SPDX-License-Identifier: AGPL-3.0-only
// A fake of the browser's WebRTC for the tests of the call (web-client/README.md T4: jsdom has no WebRTC): an
// RTCPeerConnection that models the JSEP states that the perfect negotiation pattern meets (offers, answers, the
// implicit rollback of an offer in glare, transceivers and their directions, negotiationneeded, restartIce,
// setConfiguration), and writes session descriptions with one `m=audio` line and the direction of each transceiver.
// It sends no media: a remote description that sends audio fires `track` with a fake stream.

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
  stop(): void {
    this.stopped = true;
  }
}

export class FakeStream {
  readonly tracks: FakeTrack[];
  constructor(tracks: FakeTrack[] = [new FakeTrack()]) {
    this.tracks = tracks;
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
  /** Whether a description has carried it. */
  negotiated = false;

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

function nextConnectionId(): number {
  return ++connectionsMade;
}

export class FakePeerConnection {
  /** Every connection made, in order. */
  static all: FakePeerConnection[] = [];

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
  onnegotiationneeded: (() => void) | null = null;
  onicecandidate: ((event: { candidate: { toJSON: () => RTCIceCandidateInit } | null }) => void) | null = null;
  ontrack: ((event: { track: FakeTrack; streams: FakeStream[] }) => void) | null = null;
  oniceconnectionstatechange: (() => void) | null = null;
  #needed = false;
  #restart = false;
  #version = 0;
  /** The ICE generation, which a restart moves on. */
  #generation = 0;
  readonly #id = nextConnectionId();
  /** Whether setConfiguration throws, as a browser that refuses the change does. */
  static refusesConfiguration = false;

  #ufrag(): string {
    return `u${String(this.#id)}g${String(this.#generation)}`;
  }

  constructor(configuration: RTCConfiguration) {
    this.configuration = configuration;
    FakePeerConnection.all.push(this);
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
    return `v=0\r\no=- 1 ${String(this.#version)} IN IP4 0.0.0.0\r\na=ice-ufrag:${this.#ufrag()}\r\n${restart}${directions.map((d) => `m=audio 9 UDP/TLS/RTP/SAVPF 111\r\na=${d}\r\n`).join("")}`;
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
      for (const t of this.transceivers) t.negotiated = true;
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
      this.remoteDescription = description;
      this.signalingState = "have-remote-offer";
    } else {
      if (this.signalingState !== "have-local-offer")
        return Promise.reject(new DOMException("no offer", "InvalidStateError"));
      this.remoteDescription = description;
      this.signalingState = "stable";
      for (const t of this.transceivers) t.negotiated = true;
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
    ) {
      return Promise.reject(new DOMException("unknown ufrag", "OperationError"));
    }
    this.candidates.push(candidate);
    return Promise.resolve();
  }

  /** Makes ICE fail, as a network change does. */
  failIce(): void {
    this.iceConnectionState = "failed";
    this.oniceconnectionstatechange?.();
  }

  close(): void {
    this.closed = true;
    this.signalingState = "closed";
  }
}

/** The audio of a remote participant, which records what the connection did with it. */
export class FakeRemoteAudio {
  static all: FakeRemoteAudio[] = [];
  audible = false;
  playing = false;
  closed = false;
  /** Whether play rejects as a browser that blocks autoplay does. */
  static blocked = false;

  constructor() {
    FakeRemoteAudio.all.push(this);
  }

  play = (): Promise<void> => {
    if (FakeRemoteAudio.blocked) return Promise.reject(new DOMException("blocked", "NotAllowedError"));
    this.playing = true;
    return Promise.resolve();
  };

  resume = (): Promise<void> => {
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

/** A LevelMeter of the environment whose level the test sets. */
export class FakeLevelMeter {
  /** Every meter made, in order. */
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

/** An AnalyserNode whose samples the test sets. */
export class FakeAnalyser {
  fftSize = 2048;
  /** The value of every sample. */
  sample = 0;
  connected = true;
  getFloatTimeDomainData(samples: Float32Array): void {
    samples.fill(this.sample);
  }
  connect(): void {
    this.connected = true;
  }
  disconnect(): void {
    this.connected = false;
  }
}

/** The browser's AudioContext, running, for the audio of the calls of the integration tests. */
export class FakeAudioContext {
  /** Every context made, in order. */
  static all: FakeAudioContext[] = [];
  state: AudioContextState = "running";
  readonly destination = {};
  readonly analysers: FakeAnalyser[] = [];
  readonly sources: { connected: boolean }[] = [];
  constructor() {
    FakeAudioContext.all.push(this);
  }
  createAnalyser(): FakeAnalyser {
    const analyser = new FakeAnalyser();
    this.analysers.push(analyser);
    return analyser;
  }
  createGain() {
    return { gain: { value: 0 }, connect: () => undefined, disconnect: () => undefined };
  }
  createMediaStreamSource() {
    const source = {
      connected: false,
      connect: () => {
        source.connected = true;
      },
      disconnect: () => {
        source.connected = false;
      },
    };
    this.sources.push(source);
    return source;
  }
  resume(): Promise<void> {
    return Promise.resolve();
  }
}

/** Forgets the connections and the audio of the previous test. */
export function resetFakeRtc(): void {
  FakePeerConnection.all = [];
  FakePeerConnection.refusesConfiguration = false;
  FakeRemoteAudio.all = [];
  FakeRemoteAudio.blocked = false;
  FakeLevelMeter.all = [];
}

/** Lets the promises and microtasks of the fakes and the connections run. */
export async function settle(): Promise<void> {
  for (let i = 0; i < 300; i++) await Promise.resolve();
}
