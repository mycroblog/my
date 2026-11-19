// SPDX-License-Identifier: AGPL-3.0-only
// The Sora RoomSession (ADR-0101 §12, §15): one multistream connection of sora-js-sdk to the channel of the room,
// with the client_id of the session and the grant's signed join value (and, on Sora Cloud, its access token) in the
// metadata that Sora's auth webhook checks. The SDK is a chunk of its own, loaded with import() only once a grant
// names Sora.
//
// The role of a Sora connection does not change while it is open (ADR-0101 §12): a session joins in the role of its
// grant, `recvonly` for a listener, and `reconnect` joins again in the role of the new grant with the same client_id
// (`sendrecv` when the user may speak, `recvonly` when no longer). The grant is short-lived and comes only with the
// join and those reconnections, so a `sendrecv` join happens then: it takes the microphone, which stays off until the
// user turns it on. The new connection replaces the previous one once it is up, so the audio pauses only while it
// connects. When the browser does not give the microphone, the session keeps listening, and turning the microphone
// on asks again while the grant is valid.
//
// Each remote stream is the connection ID of its sender; the `notify` events of Sora name the client_id (the session)
// of each connection, which decides whose audio is heard. A connection that went away without the user leaving is
// `lost`: a renewed grant, or the network coming back while the grant is valid, joins again. When the device of the
// microphone goes away, the microphone is taken again and its new track replaces the old on the connection.
//
// Who is talking comes from the level of each remote stream that is heard, and of the microphone while a sendrecv
// connection sends it; the detector outlives the connections that replace one another.
import type { ConnectionOptions, ConnectionPublisher, ConnectionSubscriber, SignalingNotifyMessage } from "sora-js-sdk";
import { Microphone, SessionCore, warn } from "./core";
import { RemoteAudios, Waiters } from "./sfu";
import { TalkingDetector } from "./talking";
import type {
  IncomingSignal,
  RoomGrant,
  RoomRoster,
  RoomSession,
  RoomSessionCapabilities,
  RoomSessionEvents,
  RoomSessionOptions,
  RoomSessionSnapshot,
  SoraSdk,
} from "./types";
import { RoomSessionFailure } from "./types";

type Role = "recvonly" | "sendrecv";

function grantRole(grant: RoomGrant): Role {
  return grant.options.role === "sendrecv" ? "sendrecv" : "recvonly";
}

/** The signaling URLs of a grant: all of its options' `signaling_urls`, or its URL. */
function signalingUrls(grant: RoomGrant): string[] {
  const urls = (grant.options.signaling_urls ?? "").split(",").filter((url) => url !== "");
  return urls.length > 0 ? urls : [grant.url ?? ""];
}

/** The metadata of the connect message that Sora's auth webhook checks. */
function metadata(grant: RoomGrant): Record<string, string> {
  const accessToken = grant.options.access_token;
  return { join: grant.token ?? "", ...(accessToken === undefined ? {} : { access_token: accessToken }) };
}

type LinkOptions = {
  sdk: SoraSdk;
  grant: RoomGrant;
  role: Role;
  session: string;
  core: SessionCore;
  roster: RoomRoster;
  talking: TalkingDetector;
  onLost: (link: Link) => void;
  onParticipants: (link: Link) => void;
};

/** The key of the user's microphone in the TalkingDetector (the remote audios' keys have a colon). */
const microphoneKey = "microphone";

/** One connection to Sora with one role. */
class Link {
  readonly role: Role;
  readonly audios: RemoteAudios;
  readonly #sora: ConnectionPublisher | ConnectionSubscriber;
  readonly #session: string;
  /** The client_id (session) of each connection ID that Sora named. */
  readonly #clients = new Map<string, string>();
  /** The stream a sendrecv connection sends. */
  #sent: MediaStream | null = null;
  #closed = false;

  constructor(options: LinkOptions) {
    this.role = options.role;
    this.#session = options.session;
    this.audios = new RemoteAudios(options.core, options.roster, options.talking);
    const { grant } = options;
    const channel = grant.options.channel_id ?? "";
    const connectionOptions: ConnectionOptions = {
      audio: true,
      video: false,
      audioCodecType: "OPUS",
      clientId: options.session,
    };
    const signaling = options.sdk.connection(signalingUrls(grant));
    this.#sora =
      options.role === "sendrecv"
        ? signaling.sendrecv(channel, metadata(grant), connectionOptions)
        : signaling.recvonly(channel, metadata(grant), connectionOptions);
    this.#sora.on("track", (event) => {
      const stream = event.streams[0];
      if (stream === undefined || event.track.kind !== "audio") return;
      this.audios.play(stream.id, this.#clients.get(stream.id), stream);
    });
    this.#sora.on("removetrack", (event) => {
      // The target is the stream of the sender's connection, whose ID is the connection ID.
      const stream = event.target as MediaStream | null;
      if (stream !== null) this.audios.remove(stream.id);
    });
    this.#sora.on("notify", (event) => {
      this.#notify(event);
      options.onParticipants(this);
    });
    this.#sora.on("disconnect", () => {
      if (!this.#closed) options.onLost(this);
    });
  }

  /** The other sessions Sora named on this connection. */
  participants(): string[] {
    return [...new Set(this.#clients.values())].filter((session) => session !== this.#session).sort();
  }

  #notify(event: SignalingNotifyMessage): void {
    switch (event.event_type) {
      case "connection.created":
        for (const named of [event, ...(event.data ?? [])]) {
          if (named.connection_id === undefined || named.client_id === undefined) continue;
          this.#clients.set(named.connection_id, named.client_id);
          this.audios.setSession(named.connection_id, named.client_id);
        }
        return;
      case "connection.destroyed":
        if (event.connection_id === undefined) return;
        this.#clients.delete(event.connection_id);
        this.audios.remove(event.connection_id);
        return;
      default:
        return;
    }
  }

  async connect(stream: MediaStream | null): Promise<void> {
    if (this.role === "sendrecv") {
      if (stream === null) throw new Error("rtc: a sendrecv connection needs the microphone");
      this.#sent = stream;
      await (this.#sora as ConnectionPublisher).connect(stream);
    } else {
      await (this.#sora as ConnectionSubscriber).connect();
    }
  }

  /** Sends `track` in place of the microphone's previous track. */
  async replaceMicrophone(track: MediaStreamTrack): Promise<void> {
    if (this.#sent === null || this.#closed) return;
    await this.#sora.replaceAudioTrack(this.#sent, track);
  }

  stats(): Promise<RTCStatsReport> | null {
    return this.#sora.pc?.getStats() ?? null;
  }

  close(): void {
    if (this.#closed) return;
    this.#closed = true;
    this.audios.close();
    this.#sora.disconnect().catch((error: unknown) => {
      warn("could not disconnect from Sora", error);
    });
  }
}

class SoraSession implements RoomSession {
  readonly kind = "sora";
  readonly session: string;
  readonly capabilities: RoomSessionCapabilities;
  readonly #core: SessionCore;
  readonly #sdk: Promise<SoraSdk>;
  readonly #waiters = new Waiters();
  readonly #talking: TalkingDetector;
  readonly #microphone: Microphone;
  #grant: RoomGrant;
  #roster: RoomRoster = { speaker: false, members: [] };
  /** The live connection, or null before the first one is up and after it went away. */
  #link: Link | null = null;
  /** The joins, one after another. */
  #joining: Promise<void> = Promise.resolve();
  /** How many sendrecv joins wait or run. */
  #sendrecvJoins = 0;
  /** The microphone the user wants: the latest `setMicrophone`. */
  #wantMicrophone = false;
  #microphoneChanges: Promise<void> = Promise.resolve();
  /** Whether the connection went away without the user leaving: the next grant joins again. */
  #lost = false;
  readonly #unwatch: (() => void)[] = [];

  constructor(options: RoomSessionOptions) {
    this.session = options.session;
    const environment = options.environment;
    this.#core = new SessionCore(environment);
    this.capabilities = { outputSelection: environment.setOutputDevice !== undefined };
    this.#grant = options.grant;
    this.#talking = new TalkingDetector(environment, (sessions) => {
      this.#core.setTalking(sessions);
    });
    this.#microphone = new Microphone(environment, async (track) => {
      this.#measureMicrophone();
      await this.#link?.replaceMicrophone(track);
    });
    const load = environment.loadSora ?? (() => import("sora-js-sdk").then((module) => module.default));
    this.#sdk = load();
    this.#sdk.catch((error: unknown) => {
      this.#core.fail(new RoomSessionFailure("unavailable", error));
      this.close();
    });
    const network = environment.watchNetwork?.(() => {
      if (this.#lost && this.#grant.expiresAt > Date.now()) this.#rejoin(grantRole(this.#grant));
    });
    if (network !== undefined) this.#unwatch.push(network);
    const devices = environment.watchDevices?.(() => {
      void this.#microphone.recover();
    });
    if (devices !== undefined) this.#unwatch.push(devices);
    this.#core.sampleQuality(async () => {
      const stats = this.#link?.stats();
      return stats === undefined || stats === null ? [] : [["sora", await stats] as const];
    }, options.qualityInterval);
    this.#rejoin(grantRole(options.grant));
  }

  snapshot(): RoomSessionSnapshot {
    return this.#core.snapshot();
  }

  on<E extends keyof RoomSessionEvents>(event: E, listener: RoomSessionEvents[E]): () => void {
    return this.#core.on(event, listener);
  }

  #reportConnection(): void {
    const link = this.#link;
    const state = this.#lost ? "lost" : link === null ? "connecting" : "connected";
    const participants = (link?.participants() ?? []).map((session) => ({ session, state: "connected" as const }));
    this.#core.setConnection({ state, participants: state === "connected" ? participants : [] });
  }

  /**
   * Joins with `role` and replaces the live connection once the new one is up; a failure keeps the previous one. A
   * `sendrecv` join takes the microphone first (off until the user turns it on). Without it the join rejects with the
   * browser's error, and a session that has no connection yet joins `recvonly` instead.
   */
  #join(role: Role): Promise<void> {
    if (role === "sendrecv") this.#sendrecvJoins++;
    const joined = this.#joining.then(async () => {
      if (this.#core.closed()) return;
      // The browser's error (a DOMException) as it is, so that the caller tells a refusal from a missing microphone.
      let microphoneError: Error | null = null;
      let joinRole = role;
      if (role === "sendrecv" && this.#microphone.stream === null) {
        try {
          const stream = await this.#microphone.acquire(() => !this.#core.closed());
          if (stream === null) return;
        } catch (error) {
          if (this.#link !== null) throw error;
          microphoneError = error instanceof Error ? error : new Error(String(error));
          joinRole = "recvonly";
        }
      }
      this.#applyMicrophoneTracks();
      await this.#connect(joinRole);
      if (microphoneError !== null) throw microphoneError;
    });
    const settled = joined.catch(() => undefined);
    this.#joining = settled.then(() => {
      if (role === "sendrecv") this.#sendrecvJoins--;
    });
    return joined;
  }

  async #connect(role: Role): Promise<void> {
    this.#lost = false;
    this.#reportConnection();
    const sdk = await this.#sdk;
    if (this.#core.closed()) return;
    const link = new Link({
      sdk,
      grant: this.#grant,
      role,
      session: this.session,
      core: this.#core,
      roster: this.#roster,
      talking: this.#talking,
      onLost: (lost) => {
        this.#linkLost(lost);
      },
      onParticipants: (named) => {
        if (named === this.#link) this.#reportConnection();
      },
    });
    try {
      await link.connect(role === "sendrecv" ? this.#microphone.stream : null);
    } catch (error) {
      link.close();
      if (role === "sendrecv" && this.#link?.role !== "sendrecv") this.#stopMicrophone();
      if (this.#link === null) this.#lost = !this.#core.closed();
      this.#reportConnection();
      throw error;
    }
    if (this.#core.closed()) {
      link.close();
      return;
    }
    link.audios.update(this.#roster);
    this.#link?.close();
    this.#link = link;
    if (role === "recvonly") this.#stopMicrophone();
    this.#measureMicrophone();
    this.#reportConnection();
    this.#waiters.changed();
  }

  #rejoin(role: Role): void {
    if (role === "recvonly") this.#stopMicrophone();
    this.#join(role).catch((error: unknown) => {
      warn(`could not join Sora ${role}`, error);
    });
  }

  #linkLost(link: Link): void {
    if (link !== this.#link) return;
    link.close();
    this.#link = null;
    this.#lost = !this.#core.closed();
    this.#measureMicrophone();
    this.#reportConnection();
  }

  #stopMicrophone(): void {
    this.#microphone.stop();
    this.#talking.unwatch(microphoneKey);
    this.#core.setMicrophone(false);
  }

  #sends(): boolean {
    return (
      this.#microphone.stream !== null &&
      this.#wantMicrophone &&
      this.#roster.speaker &&
      this.#link?.role === "sendrecv"
    );
  }

  /** Sends the microphone only while the user wants it and speaks. */
  #applyMicrophoneTracks(): void {
    this.#microphone.setEnabled(this.#wantMicrophone && this.#roster.speaker);
    this.#measureMicrophone();
  }

  /** Measures the microphone while a sendrecv connection sends it: the user talks only then. */
  #measureMicrophone(): void {
    const stream = this.#microphone.stream;
    if (stream !== null && this.#sends()) this.#talking.watch(microphoneKey, this.session, stream);
    else this.#talking.unwatch(microphoneKey);
    this.#core.setMicrophone(this.#sends());
  }

  update(roster: RoomRoster): void {
    if (this.#core.closed()) return;
    this.#roster = roster;
    // The server blocks the audio of a session that no longer speaks and gives it a recvonly grant to join again.
    this.#applyMicrophoneTracks();
    this.#link?.audios.update(roster);
  }

  setMicrophone(on: boolean): Promise<void> {
    this.#wantMicrophone = on;
    const change = this.#microphoneChanges.then(() => this.#applyMicrophone());
    this.#microphoneChanges = change.catch(() => undefined);
    return change;
  }

  /** Applies the microphone the user wants now, joining sendrecv first when the session does not send yet. */
  async #applyMicrophone(): Promise<void> {
    if (this.#core.closed()) return;
    if (this.#wantMicrophone && this.#roster.speaker && this.#link?.role !== "sendrecv") {
      // A sendrecv grant in hand that is still valid (after the browser refused the microphone, say): join with it now.
      if (this.#sendrecvJoins === 0 && grantRole(this.#grant) === "sendrecv" && this.#grant.expiresAt > Date.now()) {
        await this.#join("sendrecv");
      }
      // Otherwise the sendrecv grant comes with `reconnect`, after the room state that made the user a speaker.
      await this.#waiters.wait(
        () => this.#core.closed() || this.#link?.role === "sendrecv",
        "Sora did not let the session send",
      );
    }
    this.#applyMicrophoneTracks();
  }

  setOutputDevice(deviceId: string): Promise<void> {
    return this.#core.setOutputDevice(deviceId);
  }

  renew(grant: RoomGrant): void {
    if (this.#core.closed()) return;
    this.#grant = grant;
    if (this.#lost) this.#rejoin(grantRole(grant));
  }

  reconnect(grant: RoomGrant): void {
    if (this.#core.closed()) return;
    this.#grant = grant;
    this.#rejoin(grantRole(grant));
  }

  receiveSignal(_signal: IncomingSignal): void {
    // Sora signals on its own connection; the room stream carries no signals for it.
  }

  async resumePlayback(): Promise<void> {
    await this.#core.resume(async () => {
      await this.#link?.audios.resume();
    });
  }

  close(): void {
    if (this.#core.closed()) return;
    for (const unwatch of this.#unwatch.splice(0)) unwatch();
    this.#link?.close();
    this.#link = null;
    this.#stopMicrophone();
    this.#talking.close();
    this.#core.close();
    this.#waiters.changed();
  }
}

/** The Sora session of `options`. */
export function createSoraSession(options: RoomSessionOptions): RoomSession {
  return new SoraSession(options);
}
