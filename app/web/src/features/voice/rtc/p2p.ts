// SPDX-License-Identifier: AGPL-3.0-only
// The P2P method of the audio of a call (ADR-0101 §4, §5): one RTCPeerConnection to each other live session of the
// room when at least one of the two speaks (P1; two listeners never connect). Of the two, the session with the greater
// ID sends the first offer; after that, either renegotiates by the perfect negotiation pattern of WebRTC, with the
// session of the smaller ID polite (MDN, "Establishing a connection: The WebRTC perfect negotiation pattern").
// Signals go through the room stream and POST /voice/rooms/{id}/signals, one at a time and in order for each pair.
//
// Each pair carries one audio transceiver. A speaker sends and receives; anyone else only receives (`recvonly`), which
// is all the server relays from a participant who may not speak. A speaker who goes back to listening stops the
// microphone and turns its transceivers to `recvonly`, which renegotiates. The audio of a participant whom the latest
// `state` does not list as a speaker is not played (the receiver's side of the enforcement).
//
// Signals get lost (a receiver's stream that does not take one within 30 seconds, a refused or failed request), so no
// pair waits for one forever: an offer that gets no answer is rolled back and made again, a description that could not
// be sent is made again a little later, and a polite side that has had no offer asks the other side for one with a
// `restart` signal. A candidate that cannot be added (one of an ICE generation that a restart replaced) is dropped.
// A connection whose ICE failed restarts ICE.
//
// New ICE servers (a renewed grant) go onto every connection with setConfiguration, and restartIce gathers new
// candidates with them. A browser without setConfiguration (Firefox before 99, MDN browser-compat-data), or one that
// refuses the new configuration, gets a new connection for that pair instead.
//
// Who is talking comes from the level of each remote stream that is heard, and of the microphone while it sends
// (TalkingDetector), each under its session.
import { TalkingDetector } from "./talking";
import {
  type IncomingVoiceSignal,
  rtcConfiguration,
  type VoiceConnection,
  type VoiceConnectionOptions,
  type VoiceGrant,
  type VoiceMember,
  type VoiceRoster,
  type VoiceSignal,
} from "./types";

/** How long a polite side waits for its first offer, and an offer for its answer, before asking again. */
export const offerWait = 10_000;

/** How long after a description could not be sent the pair negotiates again. */
export const resendWait = 3_000;

/** The microphone's constraints (ADR-0101 §7): audio only. */
export const microphoneConstraints: MediaStreamConstraints = {
  audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true },
  video: false,
};

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

function warn(message: string, error: unknown): void {
  console.warn(`voice: ${message}`, error);
}

type PeerOptions = {
  remote: string;
  polite: boolean;
  configuration: RTCConfiguration;
  speaker: boolean;
  track: MediaStreamTrack | null;
  audible: boolean;
  connection: VoiceConnectionOptions;
  talking: TalkingDetector;
};

/** The connection to one other session. */
class Peer {
  readonly remote: string;
  readonly #polite: boolean;
  readonly #pc: RTCPeerConnection;
  readonly #audio;
  readonly #options: VoiceConnectionOptions;
  readonly #talking: TalkingDetector;
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
  /** The wait of a polite side for its first offer. */
  #offerTimer: ReturnType<typeof setTimeout> | undefined;
  /** The wait of an offer for its answer. */
  #answerTimer: ReturnType<typeof setTimeout> | undefined;
  #resendTimer: ReturnType<typeof setTimeout> | undefined;
  /** The candidates that came before the remote description they belong to. */
  #pendingCandidates: RTCIceCandidateInit[] = [];
  /** The signals to send, one after another. */
  #sending: Promise<void> = Promise.resolve();
  /** The signals received, handled one after another. */
  #receiving: Promise<void> = Promise.resolve();

  constructor(options: PeerOptions) {
    this.remote = options.remote;
    this.#polite = options.polite;
    this.#options = options.connection;
    this.#talking = options.talking;
    this.#audible = options.audible;
    this.#speaker = options.speaker;
    this.#track = options.track;
    this.#audio = options.connection.environment.createRemoteAudio();
    this.#audio.setAudible(options.audible);
    const pc = options.connection.environment.createPeerConnection(options.configuration);
    this.#pc = pc;
    pc.onnegotiationneeded = () => {
      void this.#offer();
    };
    pc.onicecandidate = ({ candidate }) => {
      if (candidate !== null) this.#send({ kind: "candidate", candidate: candidateJson(candidate) });
    };
    pc.ontrack = ({ track, streams }) => {
      const stream = streams[0] ?? new MediaStream([track]);
      this.#stream = stream;
      this.#measure();
      this.#audio.play(stream).catch((error: unknown) => {
        if (error instanceof DOMException && error.name === "NotAllowedError")
          options.connection.events.onPlaybackBlocked();
        else warn("could not play the audio", error);
      });
    };
    pc.oniceconnectionstatechange = () => {
      if (pc.iceConnectionState === "failed") pc.restartIce();
    };
    if (this.#polite) {
      this.#waitForOffer();
    } else {
      // The greater ID sends the first offer: the transceiver fires negotiationneeded.
      this.#transceiver = pc.addTransceiver("audio", { direction: this.#direction() });
      void this.#applyTrack();
    }
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

  /** Applies the user's role and microphone: a change of direction renegotiates. */
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

  /**
   * Takes new ICE servers, and restarts ICE with them when `restart`. False when the browser cannot change the
   * configuration of this connection: the caller connects the pair again.
   */
  renew(configuration: RTCConfiguration, restart: boolean): boolean {
    // Firefox before 99 has no setConfiguration (MDN browser-compat-data).
    if (typeof this.#pc.setConfiguration !== "function") return false;
    try {
      this.#pc.setConfiguration(configuration);
    } catch (error) {
      warn("could not set the new ICE servers", error);
      return false;
    }
    if (restart) this.#pc.restartIce();
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

  #send(signal: VoiceSignal): void {
    const to = this.remote;
    this.#sending = this.#sending.then(async () => {
      if (this.#closed) return;
      try {
        await this.#options.events.sendSignal({ ...signal, to });
      } catch (error) {
        warn(`could not send the ${signal.kind}`, error);
        // A description that the other side never gets leaves the pair waiting: negotiate again a little later, with
        // the role that the latest `state` gives (the server refuses a description that its role does not allow).
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

  receive(signal: IncomingVoiceSignal): void {
    this.#receiving = this.#receiving.then(async () => {
      if (this.#closed) return;
      try {
        await this.#handle(signal);
      } catch (error) {
        warn(`could not take the ${signal.kind}`, error);
      }
    });
  }

  async #handle(signal: IncomingVoiceSignal): Promise<void> {
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
          // The first offer made the transceiver on this side: send on it while the user speaks, and only receive otherwise.
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
    this.#pc.onnegotiationneeded = null;
    this.#pc.onicecandidate = null;
    this.#pc.ontrack = null;
    this.#pc.oniceconnectionstatechange = null;
    this.#pc.close();
    this.#audio.close();
    this.#talking.unwatch(this.remote);
  }
}

/** Whether two sessions connect (P1): at least one of them speaks. */
function connects(selfSpeaker: boolean, member: Pick<VoiceMember, "speaker">): boolean {
  return selfSpeaker || member.speaker;
}

/** Whether the user's session sends the first offer to `remote`: it has the greater ID. */
function offersFirst(session: string, remote: string): boolean {
  return session > remote;
}

class P2PConnection implements VoiceConnection {
  readonly #options: VoiceConnectionOptions;
  #configuration: RTCConfiguration;
  #roster: VoiceRoster = { speaker: false, members: [] };
  readonly #peers = new Map<string, Peer>();
  #microphone: MediaStream | null = null;
  /** The microphone the user wants: the changes apply one after another, the latest one last. */
  #wantMicrophone = false;
  #microphoneChanges: Promise<void> = Promise.resolve();
  readonly #talking: TalkingDetector;
  #closed = false;

  constructor(options: VoiceConnectionOptions) {
    this.#options = options;
    this.#configuration = rtcConfiguration(options.grant);
    this.#talking = new TalkingDetector(options.environment, options.events.onTalking);
  }

  #track(): MediaStreamTrack | null {
    return this.#microphone?.getAudioTracks()[0] ?? null;
  }

  #stopMicrophone(): void {
    for (const track of this.#microphone?.getTracks() ?? []) track.stop();
    this.#microphone = null;
    this.#wantMicrophone = false;
    this.#talking.unwatch(this.#options.session);
  }

  /** Whether the connection is open and the user speaks; read again after an await. */
  #mayStillSpeak(): boolean {
    return !this.#closed && this.#roster.speaker;
  }

  update(roster: VoiceRoster): void {
    if (this.#closed) return;
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
      if (member.session === this.#options.session || !connects(roster.speaker, member)) continue;
      const peer = this.#peers.get(member.session);
      if (peer === undefined) {
        this.#peers.set(member.session, this.#peer(member));
      } else {
        peer.setAudible(member.speaker);
        peer.setSending(roster.speaker, this.#track());
      }
    }
  }

  #peer(member: VoiceMember): Peer {
    return new Peer({
      remote: member.session,
      polite: !offersFirst(this.#options.session, member.session),
      configuration: this.#configuration,
      speaker: this.#roster.speaker,
      track: this.#track(),
      audible: member.speaker,
      connection: this.#options,
      talking: this.#talking,
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
    if (this.#closed) return;
    if (this.#wantMicrophone && this.#roster.speaker && this.#microphone === null) {
      const stream = await this.#options.environment.getMicrophone();
      // The user stopped speaking, or left, while the browser asked.
      if (!this.#mayStillSpeak()) {
        for (const track of stream.getTracks()) track.stop();
        return;
      }
      this.#microphone = stream;
    }
    const track = this.#track();
    const sends = this.#wantMicrophone && this.#roster.speaker;
    if (track !== null) track.enabled = sends;
    // The user talks only while the microphone sends.
    const microphone = this.#microphone;
    if (sends && microphone !== null) this.#talking.watch(this.#options.session, this.#options.session, microphone);
    else this.#talking.unwatch(this.#options.session);
    for (const peer of this.#peers.values()) peer.setSending(this.#roster.speaker, track);
  }

  renew(grant: VoiceGrant, options: { restartIce?: boolean } = {}): void {
    this.#configuration = rtcConfiguration(grant);
    const restart = options.restartIce ?? true;
    for (const [session, peer] of this.#peers) {
      if (peer.renew(this.#configuration, restart)) continue;
      // The browser cannot change this connection: connect the pair again with the new ICE servers.
      peer.close();
      const member = this.#roster.members.find((m) => m.session === session);
      if (member === undefined) this.#peers.delete(session);
      else this.#peers.set(session, this.#peer(member));
    }
  }

  reconnect(grant: VoiceGrant): void {
    // New RTCPeerConnections for the same session: the other side meets a new DTLS identity in the new offer and
    // renegotiates by perfect negotiation. P2P servers never ask for it (SetSpeaking never reconnects, ADR-0101 §2).
    this.#configuration = rtcConfiguration(grant);
    for (const peer of this.#peers.values()) peer.close();
    this.#peers.clear();
    this.update(this.#roster);
  }

  receive(signal: IncomingVoiceSignal): void {
    if (this.#closed) return;
    let peer = this.#peers.get(signal.from);
    if (peer === undefined) {
      if (signal.kind !== "offer") return;
      const member = this.#roster.members.find((m) => m.session === signal.from);
      // An offer from a participant whom the latest `state` does not connect to (two listeners): a stale one.
      if (member !== undefined && !connects(this.#roster.speaker, member)) return;
      // An offer from a session that the latest `state` does not list yet (the offer can come first): connect now.
      peer = this.#peer(member ?? { session: signal.from, account: "", speaker: false });
      this.#peers.set(signal.from, peer);
    }
    peer.receive(signal);
  }

  async resumePlayback(): Promise<void> {
    const results = await Promise.allSettled([...this.#peers.values()].map((peer) => peer.resume()));
    const failed = results.find((result) => result.status === "rejected");
    if (failed !== undefined) throw failed.reason;
  }

  close(): void {
    if (this.#closed) return;
    this.#closed = true;
    for (const peer of this.#peers.values()) peer.close();
    this.#peers.clear();
    this.#stopMicrophone();
    this.#talking.close();
  }
}

/** Connects the user's session to the audio of a P2P room. */
export function connectP2P(options: VoiceConnectionOptions): VoiceConnection {
  return new P2PConnection(options);
}
