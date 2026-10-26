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
// New ICE servers (a renewed grant) go onto every connection with setConfiguration, and restartIce gathers new
// candidates with them. A connection whose ICE failed restarts ICE. A polite side that has not received its first offer
// after a while asks the other side for one with a `restart` signal (a signal that the receiver's stream did not take
// is lost after 30 seconds).
import { type IncomingVoiceSignal, rtcConfiguration, type VoiceConnection, type VoiceConnectionOptions, type VoiceGrant, type VoiceMember, type VoiceRoster, type VoiceSignal } from "./types";

/** How long a polite side waits for its first offer before it asks for one. */
export const offerWait = 10_000;

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
    ...(init.usernameFragment === undefined || init.usernameFragment === null ? {} : { usernameFragment: init.usernameFragment }),
  });
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
  onPlaybackBlocked: () => void;
};

/** The connection to one other session. */
class Peer {
  readonly remote: string;
  readonly #polite: boolean;
  readonly #pc: RTCPeerConnection;
  readonly #audio;
  readonly #options: VoiceConnectionOptions;
  #transceiver: RTCRtpTransceiver | null = null;
  #speaker: boolean;
  #track: MediaStreamTrack | null;
  #makingOffer = false;
  #ignoreOffer = false;
  #settingRemoteAnswer = false;
  #described = false;
  #closed = false;
  #offerTimer: ReturnType<typeof setTimeout> | undefined;
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
      this.#audio.play(stream).catch((error: unknown) => {
        if (error instanceof DOMException && error.name === "NotAllowedError") options.onPlaybackBlocked();
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
    this.#audio.setAudible(audible);
  }

  resume(): Promise<void> {
    return this.#audio.resume();
  }

  /** Takes new ICE servers and restarts ICE with them. */
  renew(configuration: RTCConfiguration): void {
    try {
      this.#pc.setConfiguration(configuration);
    } catch (error) {
      warn("could not set the new ICE servers", error);
    }
    this.#pc.restartIce();
  }

  #waitForOffer(): void {
    clearTimeout(this.#offerTimer);
    this.#offerTimer = setTimeout(() => {
      if (this.#closed || this.#described) return;
      this.#send({ kind: "restart" });
      this.#waitForOffer();
    }, offerWait);
  }

  #send(signal: VoiceSignal): void {
    const to = this.remote;
    this.#sending = this.#sending.then(async () => {
      if (this.#closed) return;
      try {
        await this.#options.events.sendSignal({ ...signal, to });
      } catch (error) {
        warn(`could not send the ${signal.kind}`, error);
      }
    });
  }

  async #offer(): Promise<void> {
    if (this.#closed) return;
    try {
      this.#makingOffer = true;
      await this.#pc.setLocalDescription();
      const description = this.#pc.localDescription;
      if (description !== null && description.type === "offer") this.#send({ kind: "offer", sdp: description.sdp });
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
        this.#described = true;
        clearTimeout(this.#offerTimer);
        this.#settingRemoteAnswer = description.type === "answer";
        await pc.setRemoteDescription(description);
        this.#settingRemoteAnswer = false;
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
        if (pc.signalingState === "have-local-offer") await pc.setLocalDescription({ type: "rollback" });
        pc.restartIce();
        return;
      }
    }
  }

  async #addCandidate(candidate: RTCIceCandidateInit): Promise<void> {
    try {
      await this.#pc.addIceCandidate(candidate);
    } catch (error) {
      if (!this.#ignoreOffer) throw error;
    }
  }

  close(): void {
    if (this.#closed) return;
    this.#closed = true;
    clearTimeout(this.#offerTimer);
    this.#pc.onnegotiationneeded = null;
    this.#pc.onicecandidate = null;
    this.#pc.ontrack = null;
    this.#pc.oniceconnectionstatechange = null;
    this.#pc.close();
    this.#audio.close();
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
  #closed = false;

  constructor(options: VoiceConnectionOptions) {
    this.#options = options;
    this.#configuration = rtcConfiguration(options.grant);
  }

  #track(): MediaStreamTrack | null {
    return this.#microphone?.getAudioTracks()[0] ?? null;
  }

  #stopMicrophone(): void {
    for (const track of this.#microphone?.getTracks() ?? []) track.stop();
    this.#microphone = null;
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
      onPlaybackBlocked: this.#options.events.onPlaybackBlocked,
    });
  }

  async setMicrophone(on: boolean): Promise<void> {
    if (this.#closed) return;
    if (on && !this.#roster.speaker) return;
    if (on && this.#microphone === null) {
      const stream = await this.#options.environment.getMicrophone();
      // The user stopped speaking, or left, while the browser asked.
      if (!this.#mayStillSpeak()) {
        for (const track of stream.getTracks()) track.stop();
        return;
      }
      this.#microphone = stream;
    }
    const track = this.#track();
    if (track !== null) track.enabled = on;
    for (const peer of this.#peers.values()) peer.setSending(this.#roster.speaker, track);
  }

  renew(grant: VoiceGrant): void {
    this.#configuration = rtcConfiguration(grant);
    for (const peer of this.#peers.values()) peer.renew(this.#configuration);
  }

  reconnect(grant: VoiceGrant): void {
    this.#configuration = rtcConfiguration(grant);
    for (const peer of this.#peers.values()) peer.close();
    this.#peers.clear();
    this.update(this.#roster);
  }

  receive(signal: IncomingVoiceSignal): void {
    if (this.#closed) return;
    // A signal from a session that the latest `state` does not connect to yet (the offer can come first): connect now.
    let peer = this.#peers.get(signal.from);
    if (peer === undefined) {
      if (signal.kind !== "offer") return;
      const member = this.#roster.members.find((m) => m.session === signal.from) ?? { session: signal.from, account: "", speaker: false };
      peer = this.#peer(member);
      this.#peers.set(signal.from, peer);
    }
    peer.receive(signal);
  }

  async resumePlayback(): Promise<void> {
    await Promise.all([...this.#peers.values()].map((peer) => peer.resume()));
  }

  close(): void {
    if (this.#closed) return;
    this.#closed = true;
    for (const peer of this.#peers.values()) peer.close();
    this.#peers.clear();
    this.#stopMicrophone();
  }
}

/** Connects the user's session to the audio of a P2P room. */
export function connectP2P(options: VoiceConnectionOptions): VoiceConnection {
  return new P2PConnection(options);
}
