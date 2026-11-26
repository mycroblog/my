// SPDX-License-Identifier: AGPL-3.0-only
// The RoomSession of my's own SFU (`sfu`, ADR-0101 §11, §12, §14, §15): one RTCPeerConnection to the media node that
// holds the room, and a WebSocket to the same node for its signaling, opened with the grant's single-use ticket
// (`<url>?ticket=<token>`). Only the browser's WebRTC and WebSocket are used. The messages are those of the client
// API's OpenAPI (`VoiceSFUClientMessage`, `VoiceSFUNodeMessage`), each a JSON text frame of version 1.
//
// The node always makes the offer: one transceiver that receives the session's microphone (the m= section it offers
// `recvonly`), and one that sends each speaker, named by the offer's `tracks` (mid → participant). The session answers
// it, sending the microphone on the first transceiver while it speaks and wants it on (the track is replaced, never
// renegotiated), and plays the audio of each listed transceiver while the latest roster lists its participant as a
// speaker. Who is talking is what the node's `speaking` says of the speakers of the roster, and the session itself
// while its microphone sends and its level says so. The session's ICE candidates go to the node; the node's are in
// its offer.
//
// Recovery: when the network changes, or ICE failed or stayed disconnected for `disconnectWait`, the session sends
// `restart` and the node offers again with new ICE credentials. When the WebSocket closes by itself (the node moved
// the room with `migrate`, refused the ticket, said the room is unavailable, or the socket dropped), the session asks
// its caller for a new grant (`requestGrant`) and connects with it, for the same session: it keeps its
// RTCPeerConnection when it lives and the grant names the same node (the node reattaches the session within 30
// seconds), and makes a new one otherwise (another node, a room no longer on the node, ICE that failed with the socket
// gone). It asks once by itself; when that gives no grant, or the new socket closes again before it got an offer or
// ICE came back, it says `lost` and waits for the caller's `reconnect`. `unsupported_version` fails the session as
// `unavailable`. A renewed grant sets its ICE servers on the connection.
import { Microphone, SessionCore, warn } from "./core";
import { disconnectWait } from "./p2p";
import { RemoteAudios } from "./sfu";
import { TalkingDetector } from "./talking";
import {
  type IncomingSignal,
  rtcConfiguration,
  type RoomGrant,
  type RoomRoster,
  type RoomSession,
  RoomSessionFailure,
  type RoomSessionCapabilities,
  type RoomSessionEvents,
  type RoomSessionOptions,
  type RoomSessionSnapshot,
} from "./types";

/** The version of the media node's messages (`VoiceSFUVersion`). */
const sfuVersion = 1;

/** A transceiver of an offer that sends the audio of one speaker (`VoiceSFUTrack`). */
type SfuTrack = { readonly mid: string; readonly participant: string };

/** A message to the media node (`VoiceSFUClientMessage`). */
type SfuClientMessage =
  | { readonly v: 1; readonly type: "answer"; readonly sdp: string }
  | { readonly v: 1; readonly type: "candidate"; readonly candidate: string }
  | { readonly v: 1; readonly type: "restart" };

/** A message from the media node (`VoiceSFUNodeMessage`); a client ignores a type it does not know. */
type SfuNodeMessage =
  | { readonly v: 1; readonly type: "offer"; readonly sdp: string; readonly tracks: readonly SfuTrack[] }
  | { readonly v: 1; readonly type: "candidate"; readonly candidate: string }
  | { readonly v: 1; readonly type: "speaking"; readonly participants: readonly string[] }
  | { readonly v: 1; readonly type: "migrate" }
  | { readonly v: 1; readonly type: "error"; readonly code: string; readonly message: string };

/** The codes of `error` the session tells apart; any other is `room_unavailable`. */
type ErrorCode = "invalid_ticket" | "invalid_message" | "unsupported_version" | "room_unavailable";

/** WebSocket's readyState. */
const socketConnecting = 0;
const socketOpen = 1;

/** The key of the microphone among the measured streams. */
const microphoneKey = "microphone";

/** The URL of the node's WebSocket for `grant`: its `url` with the ticket as the query parameter `ticket`. */
function sfuSocketUrl(grant: RoomGrant): string | null {
  if (grant.url === undefined || grant.token === undefined) return null;
  let url: URL;
  try {
    url = new URL(grant.url);
  } catch {
    return null;
  }
  url.searchParams.set("ticket", grant.token);
  return url.toString();
}

/** The mid and the direction of each m= section of a session description. */
function sections(sdp: string): { mid: string | undefined; direction: string | undefined }[] {
  return sdp
    .split(/^m=/mu)
    .slice(1)
    .map((section) => ({
      mid: /^a=mid:(\S+)/mu.exec(section)?.[1],
      direction: /^a=(sendrecv|sendonly|recvonly|inactive)$/mu.exec(section)?.[1],
    }));
}

/** The mid of the transceiver that receives the session's microphone: the section the node offers `recvonly`. */
function microphoneMid(sdp: string, tracks: readonly SfuTrack[]): string | undefined {
  const all = sections(sdp);
  const listed = new Set(tracks.map((track) => track.mid));
  return (
    all.find((section) => section.direction === "recvonly")?.mid ??
    all.find((section) => section.mid !== undefined && !listed.has(section.mid))?.mid
  );
}

/** The JSON of a candidate, with only the members the node takes. */
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

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function strings(value: unknown): string[] | null {
  return Array.isArray(value) && value.every((item) => typeof item === "string") ? value : null;
}

/** A message of the node, or null for one of another version, of a type the session does not know, or malformed. */
function parseSfuMessage(data: unknown): SfuNodeMessage | null {
  if (typeof data !== "string") return null;
  let value: unknown;
  try {
    value = JSON.parse(data);
  } catch {
    return null;
  }
  if (!isRecord(value) || value["v"] !== sfuVersion) return null;
  switch (value["type"]) {
    case "offer": {
      const { sdp, tracks } = value;
      if (typeof sdp !== "string" || !Array.isArray(tracks)) return null;
      const named = tracks.flatMap((track: unknown) =>
        isRecord(track) && typeof track["mid"] === "string" && typeof track["participant"] === "string"
          ? [{ mid: track["mid"], participant: track["participant"] }]
          : [],
      );
      return { v: 1, type: "offer", sdp, tracks: named };
    }
    case "candidate":
      return typeof value["candidate"] === "string" ? { v: 1, type: "candidate", candidate: value["candidate"] } : null;
    case "speaking": {
      const participants = strings(value["participants"]);
      return participants === null ? null : { v: 1, type: "speaking", participants };
    }
    case "migrate":
      return { v: 1, type: "migrate" };
    case "error":
      return {
        v: 1,
        type: "error",
        code: typeof value["code"] === "string" ? value["code"] : "room_unavailable",
        message: typeof value["message"] === "string" ? value["message"] : "",
      };
    default:
      return null;
  }
}

function errorCode(code: string): ErrorCode {
  return code === "invalid_ticket" || code === "invalid_message" || code === "unsupported_version"
    ? code
    : "room_unavailable";
}

/** One WebSocket to a node, and what it said before it closed. */
type Link = {
  readonly socket: WebSocket;
  /** The messages waiting for the socket to open. */
  readonly queue: string[];
  migrating: boolean;
  error: ErrorCode | null;
};

/** What the session plays of one transceiver of the node. */
type Heard = { readonly participant: string; readonly track: MediaStreamTrack };

class SfuSession implements RoomSession {
  readonly kind = "sfu";
  readonly session: string;
  readonly capabilities: RoomSessionCapabilities;
  readonly #core: SessionCore;
  readonly #talking: TalkingDetector;
  readonly #microphone: Microphone;
  readonly #audios: RemoteAudios;
  readonly #requestGrant: (() => Promise<RoomGrant>) | undefined;
  readonly #openSocket: (url: string) => WebSocket;
  #grant: RoomGrant;
  #roster: RoomRoster = { speaker: false, members: [] };
  #link: Link | null = null;
  #pc: RTCPeerConnection | null = null;
  /** The node's URL of the connection `#pc`, which a grant of another node does not keep. */
  #pcUrl: string | undefined;
  /** The sender of the transceiver that receives the microphone, once an offer named it. */
  #sender: RTCRtpSender | null = null;
  /** The audio played of each transceiver of the node, by its mid. */
  readonly #heard = new Map<string, Heard>();
  /** The participants the node says make sound now. */
  #speaking: ReadonlySet<string> = new Set();
  /** Whether the session itself talks, from the level of the microphone it sends. */
  #selfTalking = false;
  /** The node's candidates that came before the offer they belong to. */
  #pendingCandidates: RTCIceCandidateInit[] = [];
  /** The messages of the node, handled one after another. */
  #receiving: Promise<void> = Promise.resolve();
  /** Counts the connections made, so that a grant asked for before a newer one connected is dropped. */
  #attempt = 0;
  /** Whether the session asked for a grant by itself since it last got an offer or ICE came back. */
  #asked = false;
  /** The connection (`#attempt`) whose grant the session asked for and waits for, if any. */
  #asking: number | null = null;
  /** Whether the session gave up recovering by itself: the caller's `reconnect` connects it again. */
  #lost = false;
  /** Whether ICE was ever up: before, the session connects; after, it reconnects. */
  #wasConnected = false;
  #disconnectTimer: ReturnType<typeof setTimeout> | undefined;
  /** The microphone the user wants: the changes apply one after another, the latest one last. */
  #wantMicrophone = false;
  #microphoneChanges: Promise<void> = Promise.resolve();
  readonly #unwatch: (() => void)[] = [];

  constructor(options: RoomSessionOptions) {
    this.session = options.session;
    const environment = options.environment;
    this.#core = new SessionCore(environment);
    this.capabilities = { outputSelection: environment.setOutputDevice !== undefined };
    this.#grant = options.grant;
    this.#requestGrant = options.requestGrant;
    this.#openSocket = environment.openSocket ?? ((url) => new WebSocket(url));
    this.#audios = new RemoteAudios(this.#core, this.#roster);
    this.#talking = new TalkingDetector(environment, (sessions) => {
      this.#selfTalking = sessions.has(this.session);
      this.#reportTalking();
    });
    this.#microphone = new Microphone(environment, async () => {
      this.#measureMicrophone();
      await this.#applyTrack();
    });
    const network = environment.watchNetwork?.(() => {
      this.#restartIce();
    });
    if (network !== undefined) this.#unwatch.push(network);
    const devices = environment.watchDevices?.(() => {
      void this.#microphone.recover();
    });
    if (devices !== undefined) this.#unwatch.push(devices);
    this.#core.sampleQuality(async () => {
      const pc = this.#pc;
      return pc === null ? [] : [["sfu", await pc.getStats()] as const];
    }, options.qualityInterval);
    this.#connect(options.grant);
  }

  snapshot(): RoomSessionSnapshot {
    return this.#core.snapshot();
  }

  on<E extends keyof RoomSessionEvents>(event: E, listener: RoomSessionEvents[E]): () => void {
    return this.#core.on(event, listener);
  }

  // ---- The connection ----------------------------------------------------------------------------------------

  #iceUp(): boolean {
    const ice = this.#pc?.iceConnectionState;
    return ice === "connected" || ice === "completed";
  }

  /**
   * Whether the connection cannot go on with a new WebSocket: it closed, or its ICE failed while the socket was gone
   * (the node is likely gone, and a node whose own side failed has dropped the session).
   */
  #pcDead(): boolean {
    const pc = this.#pc;
    if (pc === null) return false;
    return pc.iceConnectionState === "closed" || pc.iceConnectionState === "failed" || pc.connectionState === "closed";
  }

  #reportConnection(): void {
    if (this.#core.closed()) return;
    if (this.#lost) {
      this.#core.setConnection({ state: "lost", participants: [] });
      return;
    }
    const state = this.#iceUp() ? "connected" : this.#wasConnected ? "reconnecting" : "connecting";
    this.#core.setConnection({
      state,
      participants: this.#audios.sessions().map((session) => ({ session, state })),
    });
  }

  /** Opens a WebSocket to the node of `grant`, replacing the one before, and drops a connection that cannot go on. */
  #connect(grant: RoomGrant): void {
    if (this.#core.closed()) return;
    this.#attempt++;
    this.#grant = grant;
    this.#lost = false;
    this.#dropLink();
    if (this.#pc !== null && (this.#pcDead() || grant.url !== this.#pcUrl)) this.#dropPeer();
    const url = sfuSocketUrl(grant);
    if (url === null) {
      warn("the grant names no media node", grant.url);
      this.#giveUp();
      return;
    }
    let socket: WebSocket;
    try {
      socket = this.#openSocket(url);
    } catch (error) {
      warn("could not open the media node's WebSocket", error);
      this.#giveUp();
      return;
    }
    const link: Link = { socket, queue: [], migrating: false, error: null };
    this.#link = link;
    socket.onopen = () => {
      if (this.#link !== link) return;
      for (const data of link.queue.splice(0)) socket.send(data);
    };
    socket.onmessage = (event: MessageEvent) => {
      if (this.#link === link) this.#receive(link, event.data);
    };
    socket.onclose = () => {
      if (this.#link === link) this.#linkClosed(link);
    };
    socket.onerror = () => undefined;
    this.#reportConnection();
  }

  /** Forgets the WebSocket, closing it; its events say nothing after it. */
  #dropLink(): void {
    const link = this.#link;
    this.#link = null;
    if (link === null) return;
    link.socket.onopen = null;
    link.socket.onmessage = null;
    link.socket.onclose = null;
    link.socket.onerror = null;
    if (link.socket.readyState === socketConnecting || link.socket.readyState === socketOpen) link.socket.close(1000);
  }

  /** Closes the connection and the audio it played: the next offer makes a new one. */
  #dropPeer(): void {
    const pc = this.#pc;
    this.#pc = null;
    this.#pcUrl = undefined;
    this.#sender = null;
    this.#pendingCandidates = [];
    clearTimeout(this.#disconnectTimer);
    for (const mid of this.#heard.keys()) this.#audios.remove(mid);
    this.#heard.clear();
    if (pc === null) return;
    pc.onicecandidate = null;
    pc.oniceconnectionstatechange = null;
    pc.close();
  }

  #newPeer(): RTCPeerConnection {
    const pc = this.#core.environment.createPeerConnection(rtcConfiguration(this.#grant));
    this.#pc = pc;
    this.#pcUrl = this.#grant.url;
    pc.onicecandidate = ({ candidate }) => {
      if (candidate !== null && this.#pc === pc)
        this.#send({ v: 1, type: "candidate", candidate: candidateJson(candidate) });
    };
    pc.oniceconnectionstatechange = () => {
      if (this.#pc === pc) this.#iceChanged(pc);
    };
    return pc;
  }

  #iceChanged(pc: RTCPeerConnection): void {
    const ice = pc.iceConnectionState;
    clearTimeout(this.#disconnectTimer);
    if (ice === "connected" || ice === "completed") {
      this.#wasConnected = true;
      this.#asked = false;
    } else if (ice === "failed") {
      this.#restartIce();
    } else if (ice === "disconnected") {
      this.#disconnectTimer = setTimeout(() => {
        if (this.#pc === pc && pc.iceConnectionState === "disconnected") this.#restartIce();
      }, disconnectWait);
    }
    this.#reportConnection();
  }

  /**
   * Asks the node for an ICE restart (a network change, ICE that failed or stayed disconnected); without a socket to
   * ask on, connects again, with a new connection when ICE failed (the node is likely gone).
   */
  #restartIce(): void {
    if (this.#core.closed() || this.#lost) return;
    if (this.#link !== null) {
      this.#send({ v: 1, type: "restart" });
      return;
    }
    this.#recover(this.#pc?.iceConnectionState === "failed");
  }

  #send(message: SfuClientMessage): void {
    const link = this.#link;
    if (link === null) return;
    const data = JSON.stringify(message);
    if (link.socket.readyState === socketOpen) link.socket.send(data);
    else if (link.socket.readyState === socketConnecting) link.queue.push(data);
  }

  #linkClosed(link: Link): void {
    this.#link = null;
    if (this.#core.closed()) return;
    if (link.error === "unsupported_version") {
      this.#core.fail(new RoomSessionFailure("unavailable", new Error("rtc: the media node speaks another version")));
      this.close();
      return;
    }
    // Another node (a move), or a session or room the node no longer holds: the connection cannot go on.
    const fresh =
      link.migrating ||
      link.error === "room_unavailable" ||
      link.error === "invalid_message" ||
      this.#pc?.iceConnectionState === "failed";
    this.#recover(fresh);
  }

  /** Asks the caller for a grant and connects with it, once until the next offer or ICE connection; else `lost`. */
  #recover(fresh: boolean): void {
    if (this.#core.closed() || this.#lost || this.#asking === this.#attempt) return;
    if (fresh) this.#dropPeer();
    const requestGrant = this.#requestGrant;
    if (requestGrant === undefined || this.#asked) {
      this.#giveUp();
      return;
    }
    this.#asked = true;
    const attempt = this.#attempt;
    this.#asking = attempt;
    this.#reportConnection();
    requestGrant().then(
      (grant) => {
        if (this.#asking === attempt) this.#asking = null;
        if (this.#core.closed() || attempt !== this.#attempt) return;
        this.#connect(grant);
      },
      (error: unknown) => {
        if (this.#asking === attempt) this.#asking = null;
        if (this.#core.closed() || attempt !== this.#attempt) return;
        warn("could not get a new grant", error);
        this.#giveUp();
      },
    );
  }

  /** Says `lost`: the caller's `reconnect` connects the session again. */
  #giveUp(): void {
    this.#dropLink();
    this.#lost = true;
    this.#reportConnection();
  }

  // ---- The node's messages -----------------------------------------------------------------------------------

  #receive(link: Link, data: unknown): void {
    const message = parseSfuMessage(data);
    if (message === null) return;
    switch (message.type) {
      case "offer":
        this.#handle(link, "offer", () => this.#takeOffer(message.sdp, message.tracks));
        return;
      case "candidate":
        this.#handle(link, "candidate", () => this.#addCandidate(JSON.parse(message.candidate) as RTCIceCandidateInit));
        return;
      case "speaking":
        this.#speaking = new Set(message.participants);
        this.#reportTalking();
        return;
      case "migrate":
        link.migrating = true;
        return;
      case "error":
        link.error = errorCode(message.code);
        warn(`the media node refused: ${message.code}`, message.message);
        return;
    }
  }

  /** Runs `handle` after the messages before it, while `link` is the session's WebSocket. */
  #handle(link: Link, kind: string, handle: () => Promise<void>): void {
    this.#receiving = this.#receiving.then(async () => {
      if (this.#core.closed() || this.#link !== link) return;
      try {
        await handle();
      } catch (error) {
        warn(`could not take the media node's ${kind}`, error);
      }
    });
  }

  async #takeOffer(sdp: string, tracks: readonly SfuTrack[]): Promise<void> {
    const offer: RTCSessionDescriptionInit = { type: "offer", sdp };
    const kept = this.#pc;
    let pc = kept ?? this.#newPeer();
    try {
      await pc.setRemoteDescription(offer);
    } catch (error) {
      // An offer the kept connection cannot take (another node's, or a session the node made anew): a new connection.
      if (kept === null) throw error;
      this.#dropPeer();
      pc = this.#newPeer();
      await pc.setRemoteDescription(offer);
    }
    if (this.#pc !== pc) return;
    this.#asked = false;
    const mid = microphoneMid(sdp, tracks);
    const transceivers = pc.getTransceivers();
    const microphone = transceivers.find((transceiver) => mid !== undefined && transceiver.mid === mid);
    if (microphone !== undefined) {
      if (microphone.direction !== "sendonly") microphone.direction = "sendonly";
      this.#sender = microphone.sender;
      await this.#applyTrack();
    }
    this.#hear(transceivers, tracks);
    for (const candidate of this.#pendingCandidates.splice(0)) await this.#addCandidate(candidate);
    await pc.setLocalDescription();
    const answer = pc.localDescription;
    if (answer !== null && this.#pc === pc) this.#send({ v: 1, type: "answer", sdp: answer.sdp });
    this.#reportConnection();
  }

  async #addCandidate(candidate: RTCIceCandidateInit): Promise<void> {
    const pc = this.#pc;
    if (pc === null || pc.remoteDescription === null) {
      this.#pendingCandidates.push(candidate);
      return;
    }
    try {
      await pc.addIceCandidate(candidate);
    } catch (error) {
      warn("could not add a candidate of the media node", error);
    }
  }

  /** Plays the transceiver of each speaker the offer lists, and no longer those it does not. */
  #hear(transceivers: readonly RTCRtpTransceiver[], tracks: readonly SfuTrack[]): void {
    const listed = new Map(tracks.map((track) => [track.mid, track.participant]));
    for (const mid of [...this.#heard.keys()]) {
      if (listed.has(mid)) continue;
      this.#audios.remove(mid);
      this.#heard.delete(mid);
    }
    for (const [mid, participant] of listed) {
      const track = transceivers.find((transceiver) => transceiver.mid === mid)?.receiver.track;
      if (track === undefined) continue;
      const before = this.#heard.get(mid);
      if (before?.participant === participant && before.track === track) continue;
      this.#heard.set(mid, { participant, track });
      this.#audios.play(mid, this.#sessionOf(participant), this.#core.environment.createStream([track]));
    }
  }

  /** The session of the participant the node names, as the latest roster has it. */
  #sessionOf(participant: string): string | undefined {
    return this.#roster.members.find((member) => member.participant === participant)?.session;
  }

  #reportTalking(): void {
    if (this.#core.closed()) return;
    const talking = new Set<string>();
    for (const member of this.#roster.members)
      if (member.speaker && member.participant !== undefined && this.#speaking.has(member.participant))
        talking.add(member.session);
    if (this.#selfTalking && this.#sends()) talking.add(this.session);
    this.#core.setTalking(talking);
  }

  // ---- The roster and the microphone -------------------------------------------------------------------------

  update(roster: RoomRoster): void {
    if (this.#core.closed()) return;
    this.#roster = roster;
    if (!roster.speaker) this.#stopMicrophone();
    for (const [mid, { participant }] of this.#heard) {
      const session = this.#sessionOf(participant);
      if (session !== undefined) this.#audios.setSession(mid, session);
    }
    this.#audios.update(roster);
    void this.#applyTrack();
    this.#reportTalking();
    this.#reportConnection();
  }

  #sends(): boolean {
    return this.#wantMicrophone && this.#roster.speaker && this.#microphone.track() !== null;
  }

  /** Sends the microphone on the node's transceiver while it sends, and nothing otherwise. */
  async #applyTrack(): Promise<void> {
    const sender = this.#sender;
    const track = this.#roster.speaker ? this.#microphone.track() : null;
    if (sender === null || sender.track === track) return;
    try {
      await sender.replaceTrack(track);
    } catch (error) {
      warn("could not change the sent track", error);
    }
  }

  #measureMicrophone(): void {
    const stream = this.#microphone.stream;
    if (this.#sends() && stream !== null) this.#talking.watch(microphoneKey, this.session, stream);
    else this.#talking.unwatch(microphoneKey);
    this.#core.setMicrophone(this.#sends());
  }

  #stopMicrophone(): void {
    this.#microphone.stop();
    this.#wantMicrophone = false;
    this.#measureMicrophone();
  }

  setMicrophone(on: boolean): Promise<void> {
    this.#wantMicrophone = on;
    const change = this.#microphoneChanges.then(() => this.#applyMicrophone());
    this.#microphoneChanges = change.catch(() => undefined);
    return change;
  }

  async #applyMicrophone(): Promise<void> {
    if (this.#core.closed()) return;
    if (this.#wantMicrophone && this.#roster.speaker) {
      const stream = await this.#microphone.acquire(() => !this.#core.closed() && this.#roster.speaker);
      if (stream === null) return;
    }
    this.#microphone.setEnabled(this.#wantMicrophone && this.#roster.speaker);
    this.#measureMicrophone();
    await this.#applyTrack();
  }

  setOutputDevice(deviceId: string): Promise<void> {
    return this.#core.setOutputDevice(deviceId);
  }

  renew(grant: RoomGrant): void {
    if (this.#core.closed()) return;
    this.#grant = grant;
    const pc = this.#pc;
    // Firefox before 99 has no setConfiguration (MDN browser-compat-data): the next connection takes them.
    if (pc !== null && typeof pc.setConfiguration === "function") {
      try {
        pc.setConfiguration(rtcConfiguration(grant));
      } catch (error) {
        warn("could not set the new ICE servers", error);
      }
    }
    if (this.#lost) this.#connect(grant);
  }

  reconnect(grant: RoomGrant): void {
    this.#connect(grant);
  }

  receiveSignal(_signal: IncomingSignal): void {
    // The node signals on its own WebSocket; the room stream carries no signals for it.
  }

  async resumePlayback(): Promise<void> {
    await this.#core.resume(() => this.#audios.resume());
  }

  close(): void {
    if (this.#core.closed()) return;
    this.#attempt++;
    for (const unwatch of this.#unwatch.splice(0)) unwatch();
    this.#dropLink();
    this.#dropPeer();
    this.#audios.close();
    this.#stopMicrophone();
    this.#talking.close();
    this.#core.close();
  }
}

/** The session of `options` on my's SFU. */
export function createSfuSession(options: RoomSessionOptions): RoomSession {
  return new SfuSession(options);
}
