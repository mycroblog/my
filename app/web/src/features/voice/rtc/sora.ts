// SPDX-License-Identifier: AGPL-3.0-only
// The Sora method of the audio of a call (ADR-0102 §3, §6): one multistream connection of sora-js-sdk to the channel
// of the room, with the client_id of the session and the grant's signed join value (and, on Sora Cloud, its access
// token) in the metadata that Sora's auth webhook checks. The SDK is a chunk of its own, loaded with import() only
// when a grant names Sora.
//
// The role of a Sora connection does not change while it is open (ADR-0102 §3): a session joins in the role of its
// grant, `recvonly` for a listener, and a `grant` event with `reconnect: true` joins again in the role of the new grant
// with the same client_id (`sendrecv` when the user may speak, `recvonly` when no longer). The grant is short-lived and
// comes only with the join and those events, so a `sendrecv` join happens then: it takes the microphone, which stays
// off until the user turns it on. The new connection replaces the previous one once it is up, so the audio pauses only
// while it connects. When the browser does not give the microphone, the session keeps listening, and turning the
// microphone on asks again while the grant is valid.
//
// Each remote stream is the connection ID of its sender; the `notify` events of Sora name the client_id (the session)
// of each connection, which decides whose audio is heard. A renewed grant is kept, and joins again at once when the
// connection went away without the user leaving.
//
// Who is talking comes from the level of each remote stream that is heard, and of the microphone while a sendrecv
// connection sends it (TalkingDetector); the detector outlives the connections that replace one another.
import type Sora from "sora-js-sdk";
import type { ConnectionOptions, ConnectionPublisher, ConnectionSubscriber, SignalingNotifyMessage } from "sora-js-sdk";
import { RemoteAudios, Waiters, warn } from "./sfu";
import { TalkingDetector } from "./talking";
import type { VoiceConnection, VoiceConnectionOptions, VoiceGrant, VoiceRoster } from "./types";

type Sdk = typeof Sora;

type Role = "recvonly" | "sendrecv";

function grantRole(grant: VoiceGrant): Role {
  return grant.options.role === "sendrecv" ? "sendrecv" : "recvonly";
}

/** The signaling URLs of a grant: all of its options' `signaling_urls`, or its URL. */
function signalingUrls(grant: VoiceGrant): string[] {
  const urls = (grant.options.signaling_urls ?? "").split(",").filter((url) => url !== "");
  return urls.length > 0 ? urls : [grant.url ?? ""];
}

/** The metadata of the connect message that Sora's auth webhook checks. */
function metadata(grant: VoiceGrant): Record<string, string> {
  const accessToken = grant.options.access_token;
  return { join: grant.token ?? "", ...(accessToken === undefined ? {} : { access_token: accessToken }) };
}

type LinkOptions = {
  sdk: Sdk;
  grant: VoiceGrant;
  role: Role;
  connection: VoiceConnectionOptions;
  roster: VoiceRoster;
  talking: TalkingDetector;
  onLost: (link: Link) => void;
};

/** The key of the user's microphone in the TalkingDetector (the remote audios' keys have a colon). */
const microphoneKey = "microphone";

/** One connection to Sora with one role. */
class Link {
  readonly role: Role;
  readonly audios: RemoteAudios;
  readonly #sora: ConnectionPublisher | ConnectionSubscriber;
  /** The client_id (session) of each connection ID that Sora named. */
  readonly #clients = new Map<string, string>();
  #closed = false;

  constructor(options: LinkOptions) {
    this.role = options.role;
    this.audios = new RemoteAudios(
      options.connection.environment,
      options.connection.events.onPlaybackBlocked,
      options.roster,
      options.talking,
    );
    const { grant } = options;
    const channel = grant.options.channel_id ?? "";
    const connectionOptions: ConnectionOptions = {
      audio: true,
      video: false,
      audioCodecType: "OPUS",
      clientId: options.connection.session,
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
    });
    this.#sora.on("disconnect", () => {
      if (!this.#closed) options.onLost(this);
    });
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
      if (stream === null) throw new Error("voice: a sendrecv connection needs the microphone");
      await (this.#sora as ConnectionPublisher).connect(stream);
    } else {
      await (this.#sora as ConnectionSubscriber).connect();
    }
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

class SoraConnection implements VoiceConnection {
  readonly #options: VoiceConnectionOptions;
  readonly #sdk: Promise<Sdk>;
  readonly #waiters = new Waiters();
  readonly #talking: TalkingDetector;
  #grant: VoiceGrant;
  #roster: VoiceRoster = { speaker: false, members: [] };
  /** The live connection, or null before the first one is up and after it went away. */
  #link: Link | null = null;
  /** The joins, one after another. */
  #joining: Promise<void> = Promise.resolve();
  /** How many sendrecv joins wait or run. */
  #sendrecvJoins = 0;
  #microphone: MediaStream | null = null;
  /** The microphone the user wants: the latest `setMicrophone`. */
  #wantMicrophone = false;
  /** The changes of the microphone, one after another. */
  #microphoneChanges: Promise<void> = Promise.resolve();
  /** Whether the connection went away without the user leaving: the next grant joins again. */
  #lost = false;
  #closed = false;

  constructor(options: VoiceConnectionOptions) {
    this.#options = options;
    this.#grant = options.grant;
    this.#talking = new TalkingDetector(options.environment, options.events.onTalking);
    this.#sdk = import("sora-js-sdk").then((module) => module.default);
    this.#rejoin(grantRole(options.grant));
  }

  /** Whether the user left; a method, so that it is read again after an await. */
  #isClosed(): boolean {
    return this.#closed;
  }

  /**
   * Joins with `role` and replaces the live connection once the new one is up; a failure keeps the previous one. A
   * `sendrecv` join takes the microphone first (off until the user turns it on). Without it the join rejects with the
   * browser's error, and a session that has no connection yet joins `recvonly` instead.
   */
  #join(role: Role): Promise<void> {
    if (role === "sendrecv") this.#sendrecvJoins++;
    const joined = this.#joining.then(async () => {
      if (this.#closed) return;
      // The browser's error (a DOMException) as it is, so that the hooks tell a refusal from a missing microphone.
      let microphoneError: Error | null = null;
      let joinRole = role;
      if (role === "sendrecv" && this.#microphone === null) {
        try {
          const stream = await this.#options.environment.getMicrophone();
          if (this.#isClosed()) {
            for (const track of stream.getTracks()) track.stop();
            return;
          }
          this.#microphone = stream;
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
    const sdk = await this.#sdk;
    if (this.#isClosed()) return;
    const link = new Link({
      sdk,
      grant: this.#grant,
      role,
      connection: this.#options,
      roster: this.#roster,
      talking: this.#talking,
      onLost: (lost) => {
        this.#linkLost(lost);
      },
    });
    try {
      await link.connect(role === "sendrecv" ? this.#microphone : null);
    } catch (error) {
      link.close();
      if (role === "sendrecv" && this.#link?.role !== "sendrecv") this.#stopMicrophone();
      if (this.#link === null) this.#lost = !this.#closed;
      throw error;
    }
    if (this.#isClosed()) {
      link.close();
      return;
    }
    link.audios.update(this.#roster);
    this.#link?.close();
    this.#link = link;
    if (role === "recvonly") this.#stopMicrophone();
    this.#measureMicrophone();
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
    this.#lost = !this.#closed;
    this.#measureMicrophone();
  }

  #stopMicrophone(): void {
    for (const track of this.#microphone?.getTracks() ?? []) track.stop();
    this.#microphone = null;
    this.#talking.unwatch(microphoneKey);
  }

  /** Sends the microphone only while the user wants it and speaks. */
  #applyMicrophoneTracks(): void {
    for (const track of this.#microphone?.getAudioTracks() ?? [])
      track.enabled = this.#wantMicrophone && this.#roster.speaker;
    this.#measureMicrophone();
  }

  /** Measures the microphone while a sendrecv connection sends it: the user talks only then. */
  #measureMicrophone(): void {
    const microphone = this.#microphone;
    if (microphone !== null && this.#wantMicrophone && this.#roster.speaker && this.#link?.role === "sendrecv")
      this.#talking.watch(microphoneKey, this.#options.session, microphone);
    else this.#talking.unwatch(microphoneKey);
  }

  update(roster: VoiceRoster): void {
    if (this.#closed) return;
    this.#roster = roster;
    // The server blocks the audio of a session that no longer speaks and sends it a recvonly grant to join again.
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
    if (this.#closed) return;
    if (this.#wantMicrophone && this.#roster.speaker && this.#link?.role !== "sendrecv") {
      // A sendrecv grant in hand that is still valid (after the browser refused the microphone, say): join with it now.
      if (this.#sendrecvJoins === 0 && grantRole(this.#grant) === "sendrecv" && this.#grant.expiresAt > Date.now()) {
        await this.#join("sendrecv");
      }
      // Otherwise the sendrecv grant comes on the room stream, after the `state` that made the user a speaker.
      await this.#waiters.wait(
        () => this.#closed || this.#link?.role === "sendrecv",
        "Sora did not let the session send",
      );
    }
    this.#applyMicrophoneTracks();
  }

  renew(grant: VoiceGrant): void {
    if (this.#closed) return;
    this.#grant = grant;
    if (this.#lost) this.#rejoin(grantRole(grant));
  }

  reconnect(grant: VoiceGrant): void {
    if (this.#closed) return;
    this.#grant = grant;
    this.#rejoin(grantRole(grant));
  }

  receive(): void {
    // Sora signals on its own connection; the room stream carries no signals for it.
  }

  async resumePlayback(): Promise<void> {
    await this.#link?.audios.resume();
  }

  close(): void {
    if (this.#closed) return;
    this.#closed = true;
    this.#link?.close();
    this.#link = null;
    this.#stopMicrophone();
    this.#talking.close();
    this.#waiters.changed();
  }
}

/** Connects the user's session to the audio of a Sora channel. */
export function connectSora(options: VoiceConnectionOptions): VoiceConnection {
  return new SoraConnection(options);
}
