// SPDX-License-Identifier: AGPL-3.0-only
// The P2P RoomSession (ADR-0101 §4, §5): one RTCPeerConnection to each other live session of the room when at least
// one of the two speaks (two listeners never connect). Of the two, the session with the greater ID sends the first
// offer; after that, either renegotiates by the perfect negotiation pattern of WebRTC, with the session of the smaller
// ID polite (MDN, "Establishing a connection: The WebRTC perfect negotiation pattern"). Signals go out through the
// signaling port, one at a time and in order for each pair, and come in through `receiveSignal`. Only the browser's
// WebRTC is used.
//
// Each pair carries one audio transceiver. A speaker sends and receives; anyone else only receives (`recvonly`), which
// is all the server relays from a participant who may not speak. A speaker who goes back to listening stops the
// microphone and turns its transceivers to `recvonly`, which renegotiates. The audio of a participant whom the latest
// roster does not list as a speaker is not played (the receiver's side of the enforcement).
//
// Signals get lost (a receiver's stream that does not take one within 30 seconds, a refused or failed request), so no
// pair waits for one forever: an offer that gets no answer is rolled back and made again, a description that could not
// be sent is made again a little later, and a polite side that has had no offer asks the other side for one with a
// `restart` signal. A candidate of an ICE generation that a restart replaced is dropped.
//
// A pair restarts ICE when ICE failed, when it stayed disconnected for `disconnectWait`, and when the network comes
// back or changes. New ICE servers (a renewed grant) go onto every connection with setConfiguration, and restartIce
// gathers new candidates with them; a browser that has no setConfiguration, or refuses the new configuration, gets a
// new connection for that pair instead. When the device of the microphone goes away, the microphone is taken again
// and its new track replaces the old on every pair.
//
// Who is talking comes from the level of each remote stream that is heard, and of the microphone while it sends.
import { Microphone, SessionCore, warn } from "./core";
import { TalkingDetector } from "./talking";
import {
  type IncomingSignal,
  type ParticipantLink,
  rtcConfiguration,
  type RoomGrant,
  type RoomMember,
  type RoomRoster,
  type RoomSession,
  type RoomSessionCapabilities,
  type RoomSessionEvents,
  type RoomSessionOptions,
  type RoomSessionSnapshot,
  type Signal,
  type SignalingPort,
} from "./types";

/** How long a polite side waits for its first offer, and an offer for its answer, before asking again. */
export const offerWait = 10_000;

/** How long after a description could not be sent the pair negotiates again. */
export const resendWait = 3_000;

/** How long a pair whose ICE is disconnected waits for it to come back by itself before it restarts ICE. */
export const disconnectWait = 3_000;

/** The JSON of a candidate, with only the members the server relays. */
function candidateJson(candidate: RTCIceCandidate): string {
  const init = candidate.toJSON();
  return JSON.stringify({
    candidate: init.candidate ?? "",
    sdpMid: init.sdpMid ?? null,
    sdpMLineIndex: init.sdpMLineIndex ?? null,
    ...(init.usernameFragment === undefined || init.usernameFragment === null
      ? {}
      : { usernameFragment: init.usernameFragment }),
  });
}

/** The ICE username fragments of a session description (`a=ice-ufrag:`). */
function usernameFragments(sdp: string | undefined): Set<string> {
  return new Set([...(sdp ?? "").matchAll(/^a=ice-ufrag:(\S+)/gmu)].map((match) => match[1] ?? ""));
}

type PeerOptions = {
  remote: string;
  polite: boolean;
  configuration: RTCConfiguration;
  speaker: boolean;
  track: MediaStreamTrack | null;
  audible: boolean;
  core: SessionCore;
  signaling: SignalingPort;
  talking: TalkingDetector;
  onState: () => void;
};

/** The connection to one other session. */
class Peer {
  readonly remote: string;
  readonly #polite: boolean;
  readonly #pc: RTCPeerConnection;
  readonly #audio;
  readonly #core: SessionCore;
  readonly #signaling: SignalingPort;
  readonly #talking: TalkingDetector;
  readonly #onState: () => void;
  /** The remote stream, measured for who is talking while it is heard. */
  #stream: MediaStream | null = null;
  #audible: boolean;
  #transceiver: RTCRtpTransceiver | null = null;
  #speaker: boolean;
  #track: MediaStreamTrack | null;
  #makingOffer = false;
  #ignoreOffer = false;
  #settingRemoteAnswer = false;
  #described = false;
  #closed = false;
  /** Whether ICE was ever up: before, the pair connects; after, it reconnects. */
  #wasConnected = false;
  /** The wait of a polite side for its first offer. */
  #offerTimer: ReturnType<typeof setTimeout> | undefined;
  /** The wait of an offer for its answer. */
  #answerTimer: ReturnType<typeof setTimeout> | undefined;
  #resendTimer: ReturnType<typeof setTimeout> | undefined;
  #disconnectTimer: ReturnType<typeof setTimeout> | undefined;
  /** The candidates that came before the remote description they belong to. */
  #pendingCandidates: RTCIceCandidateInit[] = [];
  /** The signals to send, one after another. */
  #sending: Promise<void> = Promise.resolve();
  /** The signals received, handled one after another. */
  #receiving: Promise<void> = Promise.resolve();

  constructor(options: PeerOptions) {
    this.remote = options.remote;
    this.#polite = options.polite;
    this.#core = options.core;
    this.#signaling = options.signaling;
    this.#talking = options.talking;
    this.#onState = options.onState;
    this.#audible = options.audible;
    this.#speaker = options.speaker;
    this.#track = options.track;
    const environment = options.core.environment;
    this.#audio = environment.createRemoteAudio();
    this.#audio.setAudible(options.audible);
    const pc = environment.createPeerConnection(options.configuration);
    this.#pc = pc;
    pc.onnegotiationneeded = () => {
      void this.#offer();
    };
    pc.onicecandidate = ({ candidate }) => {
      if (candidate !== null) this.#send({ kind: "candidate", candidate: candidateJson(candidate) });
    };
    pc.ontrack = ({ track, streams }) => {
      const stream = streams[0] ?? environment.createStream([track]);
      this.#stream = stream;
      this.#measure();
      this.#core.play(this.#audio, stream);
    };
    pc.oniceconnectionstatechange = () => {
      this.#iceChanged();
    };
    if (this.#polite) {
      this.#waitForOffer();
    } else {
      // The greater ID sends the first offer: the transceiver fires negotiationneeded.
      this.#transceiver = pc.addTransceiver("audio", { direction: this.#direction() });
      void this.#applyTrack();
    }
  }

  /** How the pair stands, from its ICE connection. */
  get state(): ParticipantLink["state"] {
    switch (this.#pc.iceConnectionState) {
      case "connected":
      case "completed":
        return "connected";
      case "disconnected":
      case "failed":
        return "reconnecting";
      default:
        return this.#wasConnected ? "reconnecting" : "connecting";
    }
  }

  #iceChanged(): void {
    if (this.#closed) return;
    const ice = this.#pc.iceConnectionState;
    if (ice === "connected" || ice === "completed") this.#wasConnected = true;
    clearTimeout(this.#disconnectTimer);
    if (ice === "failed") this.restartIce();
    else if (ice === "disconnected")
      this.#disconnectTimer = setTimeout(() => {
        if (!this.#closed && this.#pc.iceConnectionState === "disconnected") this.restartIce();
      }, disconnectWait);
    this.#onState();
  }

  /** Gathers new candidates and renegotiates (a failure, a network change). */
  restartIce(): void {
    if (this.#closed) return;
    this.#pc.restartIce();
  }

  #direction(): RTCRtpTransceiverDirection {
    return this.#speaker ? "sendrecv" : "recvonly";
  }

  async #applyTrack(): Promise<void> {
    const sender = this.#transceiver?.sender;
    const track = this.#speaker ? this.#track : null;
    if (sender === undefined || sender.track === track) return;
    try {
      await sender.replaceTrack(track);
    } catch (error) {
      warn("could not change the sent track", error);
    }
  }

  /** Applies the session's role and microphone: a change of direction renegotiates. */
  setSending(speaker: boolean, track: MediaStreamTrack | null): void {
    this.#speaker = speaker;
    this.#track = track;
    if (this.#transceiver === null) return;
    const direction = this.#direction();
    if (this.#transceiver.direction !== direction) this.#transceiver.direction = direction;
    void this.#applyTrack();
  }

  setAudible(audible: boolean): void {
    this.#audible = audible;
    this.#audio.setAudible(audible);
    this.#measure();
  }

  /** Measures the remote stream while it is heard: a participant who does not speak never talks. */
  #measure(): void {
    if (this.#stream !== null && this.#audible && !this.#closed)
      this.#talking.watch(this.remote, this.remote, this.#stream);
    else this.#talking.unwatch(this.remote);
  }

  resume(): Promise<void> {
    return this.#audio.resume();
  }

  stats(): Promise<RTCStatsReport> {
    return this.#pc.getStats();
  }

  /**
   * Takes new ICE servers and restarts ICE with them. False when the browser cannot change the configuration of this
   * connection: the session connects the pair again.
   */
  renew(configuration: RTCConfiguration): boolean {
    // Firefox before 99 has no setConfiguration (MDN browser-compat-data).
    if (typeof this.#pc.setConfiguration !== "function") return false;
    try {
      this.#pc.setConfiguration(configuration);
    } catch (error) {
      warn("could not set the new ICE servers", error);
      return false;
    }
    this.#pc.restartIce();
    return true;
  }

  #waitForOffer(): void {
    clearTimeout(this.#offerTimer);
    this.#offerTimer = setTimeout(() => {
      if (this.#closed || this.#described) return;
      this.#send({ kind: "restart" });
      this.#waitForOffer();
    }, offerWait);
  }

  /** Waits for the answer of the offer just sent; without one, rolls it back and offers again. */
  #waitForAnswer(): void {
    clearTimeout(this.#answerTimer);
    this.#answerTimer = setTimeout(() => {
      if (this.#closed || this.#pc.signalingState !== "have-local-offer") return;
      void this.#renegotiate();
    }, offerWait);
  }

  /** Drops an offer that got no answer and negotiates again with new ICE credentials. */
  async #renegotiate(): Promise<void> {
    if (this.#closed) return;
    try {
      if (this.#pc.signalingState === "have-local-offer") await this.#pc.setLocalDescription({ type: "rollback" });
      this.#pc.restartIce();
    } catch (error) {
      warn("could not negotiate again", error);
    }
  }

  #send(signal: Signal): void {
    const to = this.remote;
    this.#sending = this.#sending.then(async () => {
      if (this.#closed) return;
      try {
        await this.#signaling.send({ ...signal, to });
      } catch (error) {
        warn(`could not send the ${signal.kind}`, error);
        // A description that the other side never gets leaves the pair waiting: negotiate again a little later, with
        // the role that the latest roster gives (the server refuses a description that its role does not allow).
        if (signal.kind === "offer" || signal.kind === "answer") {
          clearTimeout(this.#resendTimer);
          this.#resendTimer = setTimeout(() => {
            void this.#renegotiate();
          }, resendWait);
        }
      }
    });
  }

  async #offer(): Promise<void> {
    if (this.#closed) return;
    try {
      this.#makingOffer = true;
      await this.#pc.setLocalDescription();
      const description = this.#pc.localDescription;
      if (description !== null && description.type === "offer") {
        this.#send({ kind: "offer", sdp: description.sdp });
        this.#waitForAnswer();
      }
    } catch (error) {
      warn("could not make an offer", error);
    } finally {
      this.#makingOffer = false;
    }
  }

  receive(signal: IncomingSignal): void {
    this.#receiving = this.#receiving.then(async () => {
      if (this.#closed) return;
      try {
        await this.#handle(signal);
      } catch (error) {
        warn(`could not take the ${signal.kind}`, error);
      }
    });
  }

  async #handle(signal: IncomingSignal): Promise<void> {
    const pc = this.#pc;
    switch (signal.kind) {
      case "offer":
      case "answer": {
        const description: RTCSessionDescriptionInit = { type: signal.kind, sdp: signal.sdp ?? "" };
        const readyForOffer = !this.#makingOffer && (pc.signalingState === "stable" || this.#settingRemoteAnswer);
        const offerCollision = description.type === "offer" && !readyForOffer;
        this.#ignoreOffer = !this.#polite && offerCollision;
        if (this.#ignoreOffer) return;
        this.#settingRemoteAnswer = description.type === "answer";
        try {
          await pc.setRemoteDescription(description);
        } finally {
          this.#settingRemoteAnswer = false;
        }
        this.#described = true;
        clearTimeout(this.#offerTimer);
        if (description.type === "answer") clearTimeout(this.#answerTimer);
        for (const candidate of this.#pendingCandidates.splice(0)) await this.#addCandidate(candidate);
        if (description.type === "offer") {
          // The first offer made the transceiver on this side: send on it while the session speaks, else only receive.
          if (this.#transceiver === null) {
            this.#transceiver = pc.getTransceivers().find((t) => t.receiver.track.kind === "audio") ?? null;
            if (this.#transceiver !== null) {
              this.#transceiver.direction = this.#direction();
              await this.#applyTrack();
            }
          }
          await pc.setLocalDescription();
          const answer = pc.localDescription;
          if (answer !== null) this.#send({ kind: "answer", sdp: answer.sdp });
        }
        return;
      }
      case "candidate": {
        const candidate = JSON.parse(signal.candidate ?? "{}") as RTCIceCandidateInit;
        if (pc.remoteDescription === null) this.#pendingCandidates.push(candidate);
        else await this.#addCandidate(candidate);
        return;
      }
      case "restart": {
        // The other side lost a signal: drop an offer it never answered and offer again with new ICE credentials.
        await this.#renegotiate();
        return;
      }
    }
  }

  /** Adds a candidate of the current remote description; one of another ICE generation, or one that fails, is dropped. */
  async #addCandidate(candidate: RTCIceCandidateInit): Promise<void> {
    const fragment = candidate.usernameFragment;
    if (
      fragment !== undefined &&
      fragment !== null &&
      !usernameFragments(this.#pc.remoteDescription?.sdp).has(fragment)
    )
      return;
    try {
      await this.#pc.addIceCandidate(candidate);
    } catch (error) {
      if (!this.#ignoreOffer) warn("could not add a candidate", error);
    }
  }

  close(): void {
    if (this.#closed) return;
    this.#closed = true;
    clearTimeout(this.#offerTimer);
    clearTimeout(this.#answerTimer);
    clearTimeout(this.#resendTimer);
    clearTimeout(this.#disconnectTimer);
    this.#pc.onnegotiationneeded = null;
    this.#pc.onicecandidate = null;
    this.#pc.ontrack = null;
    this.#pc.oniceconnectionstatechange = null;
    this.#pc.close();
    this.#audio.close();
    this.#talking.unwatch(this.remote);
  }
}

/** Whether two sessions connect (ADR-0101 §4): at least one of them speaks. */
function connects(selfSpeaker: boolean, member: Pick<RoomMember, "speaker">): boolean {
  return selfSpeaker || member.speaker;
}

/** Whether this session sends the first offer to `remote`: it has the greater ID. */
function offersFirst(session: string, remote: string): boolean {
  return session > remote;
}

class P2PSession implements RoomSession {
  readonly kind = "p2p";
  readonly session: string;
  readonly capabilities: RoomSessionCapabilities;
  readonly #core: SessionCore;
  readonly #signaling: SignalingPort;
  readonly #talking: TalkingDetector;
  readonly #microphone: Microphone;
  #configuration: RTCConfiguration;
  #roster: RoomRoster = { speaker: false, members: [] };
  readonly #peers = new Map<string, Peer>();
  /** The microphone the user wants: the changes apply one after another, the latest one last. */
  #wantMicrophone = false;
  #microphoneChanges: Promise<void> = Promise.resolve();
  readonly #unwatch: (() => void)[] = [];

  constructor(options: RoomSessionOptions, signaling: SignalingPort) {
    this.session = options.session;
    this.#core = new SessionCore(options.environment);
    this.capabilities = { outputSelection: options.environment.setOutputDevice !== undefined };
    this.#signaling = signaling;
    this.#configuration = rtcConfiguration(options.grant);
    this.#talking = new TalkingDetector(options.environment, (sessions) => {
      this.#core.setTalking(sessions);
    });
    this.#microphone = new Microphone(options.environment, (track, stream) => {
      this.#measureMicrophone(stream);
      for (const peer of this.#peers.values()) peer.setSending(this.#roster.speaker, track);
    });
    const environment = options.environment;
    const network = environment.watchNetwork?.(() => {
      for (const peer of this.#peers.values()) peer.restartIce();
    });
    if (network !== undefined) this.#unwatch.push(network);
    const devices = environment.watchDevices?.(() => {
      void this.#microphone.recover();
    });
    if (devices !== undefined) this.#unwatch.push(devices);
    this.#core.sampleQuality(
      async () => Promise.all([...this.#peers].map(async ([session, peer]) => [session, await peer.stats()] as const)),
      options.qualityInterval,
    );
    this.#reportConnection();
  }

  snapshot(): RoomSessionSnapshot {
    return this.#core.snapshot();
  }

  on<E extends keyof RoomSessionEvents>(event: E, listener: RoomSessionEvents[E]): () => void {
    return this.#core.on(event, listener);
  }

  #reportConnection(): void {
    const participants = [...this.#peers.values()].map((peer) => ({ session: peer.remote, state: peer.state }));
    const state = participants.some((p) => p.state === "reconnecting")
      ? "reconnecting"
      : participants.some((p) => p.state === "connecting")
        ? "connecting"
        : "connected";
    this.#core.setConnection({ state, participants });
  }

  #sends(): boolean {
    return this.#wantMicrophone && this.#roster.speaker && this.#microphone.track() !== null;
  }

  #measureMicrophone(stream: MediaStream | null = this.#microphone.stream): void {
    if (this.#sends() && stream !== null) this.#talking.watch(this.session, this.session, stream);
    else this.#talking.unwatch(this.session);
    this.#core.setMicrophone(this.#sends());
  }

  #stopMicrophone(): void {
    this.#microphone.stop();
    this.#wantMicrophone = false;
    this.#measureMicrophone();
  }

  update(roster: RoomRoster): void {
    if (this.#core.closed()) return;
    this.#roster = roster;
    if (!roster.speaker) this.#stopMicrophone();
    const members = new Map(roster.members.map((member) => [member.session, member]));
    for (const [session, peer] of this.#peers) {
      const member = members.get(session);
      if (member === undefined || !connects(roster.speaker, member)) {
        peer.close();
        this.#peers.delete(session);
      }
    }
    for (const member of roster.members) {
      if (member.session === this.session || !connects(roster.speaker, member)) continue;
      const peer = this.#peers.get(member.session);
      if (peer === undefined) {
        this.#peers.set(member.session, this.#peer(member));
      } else {
        peer.setAudible(member.speaker);
        peer.setSending(roster.speaker, this.#microphone.track());
      }
    }
    this.#reportConnection();
  }

  #peer(member: RoomMember): Peer {
    return new Peer({
      remote: member.session,
      polite: !offersFirst(this.session, member.session),
      configuration: this.#configuration,
      speaker: this.#roster.speaker,
      track: this.#microphone.track(),
      audible: member.speaker,
      core: this.#core,
      signaling: this.#signaling,
      talking: this.#talking,
      onState: () => {
        this.#reportConnection();
      },
    });
  }

  setMicrophone(on: boolean): Promise<void> {
    this.#wantMicrophone = on;
    const change = this.#microphoneChanges.then(() => this.#applyMicrophone());
    this.#microphoneChanges = change.catch(() => undefined);
    return change;
  }

  /** Applies the microphone the user wants now; only one request for the microphone runs at a time. */
  async #applyMicrophone(): Promise<void> {
    if (this.#core.closed()) return;
    if (this.#wantMicrophone && this.#roster.speaker) {
      // The user stopped speaking, or left, while the browser asked: the tracks stop.
      const stream = await this.#microphone.acquire(() => !this.#core.closed() && this.#roster.speaker);
      if (stream === null) return;
    }
    const track = this.#microphone.track();
    this.#microphone.setEnabled(this.#wantMicrophone && this.#roster.speaker);
    this.#measureMicrophone();
    for (const peer of this.#peers.values()) peer.setSending(this.#roster.speaker, track);
  }

  setOutputDevice(deviceId: string): Promise<void> {
    return this.#core.setOutputDevice(deviceId);
  }

  renew(grant: RoomGrant): void {
    if (this.#core.closed()) return;
    this.#configuration = rtcConfiguration(grant);
    for (const [session, peer] of this.#peers) {
      if (peer.renew(this.#configuration)) continue;
      // The browser cannot change this connection: connect the pair again with the new ICE servers.
      peer.close();
      const member = this.#roster.members.find((m) => m.session === session);
      if (member === undefined) this.#peers.delete(session);
      else this.#peers.set(session, this.#peer(member));
    }
    this.#reportConnection();
  }

  reconnect(grant: RoomGrant): void {
    if (this.#core.closed()) return;
    // New RTCPeerConnections for the same session: the other side meets a new DTLS identity in the new offer and
    // renegotiates by perfect negotiation.
    this.#configuration = rtcConfiguration(grant);
    for (const peer of this.#peers.values()) peer.close();
    this.#peers.clear();
    this.update(this.#roster);
  }

  receiveSignal(signal: IncomingSignal): void {
    if (this.#core.closed()) return;
    let peer = this.#peers.get(signal.from);
    if (peer === undefined) {
      if (signal.kind !== "offer") return;
      const member = this.#roster.members.find((m) => m.session === signal.from);
      // An offer from a participant whom the latest roster does not connect to (two listeners): a stale one.
      if (member !== undefined && !connects(this.#roster.speaker, member)) return;
      // An offer from a session that the latest roster does not list yet (the offer can come first): connect now.
      peer = this.#peer(member ?? { session: signal.from, speaker: false });
      this.#peers.set(signal.from, peer);
      this.#reportConnection();
    }
    peer.receive(signal);
  }

  async resumePlayback(): Promise<void> {
    await this.#core.resume(async () => {
      const results = await Promise.allSettled([...this.#peers.values()].map((peer) => peer.resume()));
      const failed = results.find((result) => result.status === "rejected");
      if (failed !== undefined) throw failed.reason;
    });
  }

  close(): void {
    if (this.#core.closed()) return;
    for (const unwatch of this.#unwatch.splice(0)) unwatch();
    for (const peer of this.#peers.values()) peer.close();
    this.#peers.clear();
    this.#stopMicrophone();
    this.#talking.close();
    this.#core.close();
  }
}

/** The P2P session of `options`, signaling through `signaling`. */
export function createP2PSession(options: RoomSessionOptions, signaling: SignalingPort): RoomSession {
  return new P2PSession(options, signaling);
}
