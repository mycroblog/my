// SPDX-License-Identifier: AGPL-3.0-only
// The Sora method of the audio of a call (ADR-0102 §3, §6): one multistream connection of sora-js-sdk to the channel
// of the room, with the client_id of the session and the grant's signed join value (and, on Sora Cloud, its access
// token) in the metadata that Sora's auth webhook checks. The SDK is a chunk of its own, loaded with import() only
// when a grant names Sora.
//
// The role of a Sora connection does not change while it is open (ADR-0102 §3): a session joins `recvonly`, and joins
// again `sendrecv` with the same client_id to speak. It does so when the user turns the microphone on with a
// `sendrecv` grant, so a speaker who never speaks is never asked for the microphone; a `grant` event with
// `reconnect: true` joins again with the role of the new grant (back to `recvonly` when the user no longer speaks).
// The new connection replaces the previous one once it is up, so the audio pauses only while it connects.
//
// Each remote stream is the connection ID of its sender; the `notify` events of Sora name the client_id (the session)
// of each connection, which decides whose audio is heard. A renewed grant is kept for the next join, and joins again
// at once when the connection went away without the user leaving.
import type Sora from "sora-js-sdk";
import type { ConnectionOptions, ConnectionPublisher, ConnectionSubscriber, SignalingNotifyMessage } from "sora-js-sdk";
import { RemoteAudios, Waiters, warn } from "./sfu";
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
  onLost: (link: Link) => void;
};

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
    this.audios = new RemoteAudios(options.connection.environment, options.connection.events.onPlaybackBlocked, options.roster);
    const { grant } = options;
    const channel = grant.options.channel_id ?? "";
    const connectionOptions: ConnectionOptions = { audio: true, video: false, audioCodecType: "OPUS", clientId: options.connection.session };
    const signaling = options.sdk.connection(signalingUrls(grant));
    this.#sora =
      options.role === "sendrecv" ? signaling.sendrecv(channel, metadata(grant), connectionOptions) : signaling.recvonly(channel, metadata(grant), connectionOptions);
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
  #grant: VoiceGrant;
  #roster: VoiceRoster = { speaker: false, members: [] };
  /** The live connection, or null before the first one is up and after it went away. */
  #link: Link | null = null;
  /** The joins, one after another. */
  #joining: Promise<void> = Promise.resolve();
  #microphone: MediaStream | null = null;
  /** Whether the connection went away without the user leaving: the next grant joins again. */
  #lost = false;
  #closed = false;

  constructor(options: VoiceConnectionOptions) {
    this.#options = options;
    this.#grant = options.grant;
    this.#sdk = import("sora-js-sdk").then((module) => module.default);
    void this.#join("recvonly").catch((error: unknown) => {
      warn("could not connect to Sora", error);
    });
  }

  /** Joins with `role` and replaces the live connection once the new one is up; a failure keeps the previous one. */
  #join(role: Role): Promise<void> {
    const joined = this.#joining.then(async () => {
      if (this.#closed) return;
      this.#lost = false;
      const sdk = await this.#sdk;
      if (this.#isClosed()) return;
      const link = new Link({
        sdk,
        grant: this.#grant,
        role,
        connection: this.#options,
        roster: this.#roster,
        onLost: (lost) => {
          this.#linkLost(lost);
        },
      });
      try {
        await link.connect(role === "sendrecv" ? this.#microphone : null);
      } catch (error) {
        link.close();
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
    });
    this.#joining = joined.catch(() => undefined);
    return joined;
  }

  /** Whether the user left; a method, so that it is read again after an await. */
  #isClosed(): boolean {
    return this.#closed;
  }

  #linkLost(link: Link): void {
    if (link !== this.#link) return;
    link.close();
    this.#link = null;
    this.#lost = !this.#closed;
  }

  /** The role to join with: `sendrecv` only while the user speaks with the microphone and the grant allows it. */
  #role(): Role {
    return this.#roster.speaker && this.#microphone !== null && grantRole(this.#grant) === "sendrecv" ? "sendrecv" : "recvonly";
  }

  #stopMicrophone(): void {
    for (const track of this.#microphone?.getTracks() ?? []) track.stop();
    this.#microphone = null;
  }

  #mayStillSpeak(): boolean {
    return !this.#closed && this.#roster.speaker;
  }

  update(roster: VoiceRoster): void {
    if (this.#closed) return;
    this.#roster = roster;
    // The server blocks the audio of a session that no longer speaks and sends it a recvonly grant to join again.
    if (!roster.speaker) this.#stopMicrophone();
    this.#link?.audios.update(roster);
  }

  async setMicrophone(on: boolean): Promise<void> {
    if (this.#closed) return;
    if (on && !this.#roster.speaker) return;
    if (!on) {
      for (const track of this.#microphone?.getAudioTracks() ?? []) track.enabled = false;
      return;
    }
    if (this.#microphone !== null && this.#link?.role === "sendrecv") {
      for (const track of this.#microphone.getAudioTracks()) track.enabled = true;
      return;
    }
    // The `sendrecv` grant can come on the room stream after the `state` that made the user a speaker.
    await this.#waiters.wait(() => this.#closed || grantRole(this.#grant) === "sendrecv", "Sora did not give a sendrecv grant");
    if (!this.#mayStillSpeak()) return;
    const stream = this.#microphone ?? (await this.#options.environment.getMicrophone());
    if (!this.#mayStillSpeak()) {
      for (const track of stream.getTracks()) track.stop();
      return;
    }
    this.#microphone = stream;
    for (const track of stream.getAudioTracks()) track.enabled = true;
    try {
      await this.#join("sendrecv");
    } catch (error) {
      this.#stopMicrophone();
      throw error;
    }
  }

  renew(grant: VoiceGrant): void {
    if (this.#closed) return;
    this.#grant = grant;
    this.#waiters.changed();
    if (this.#lost) this.#rejoin(this.#role());
  }

  reconnect(grant: VoiceGrant): void {
    if (this.#closed) return;
    this.#grant = grant;
    this.#waiters.changed();
    if (grantRole(grant) === "recvonly") this.#stopMicrophone();
    const role = this.#role();
    // A speaker whose microphone is off keeps listening on the connection it has: it joins sendrecv to speak.
    if (role === "recvonly" && grantRole(grant) === "sendrecv" && this.#link?.role === "recvonly") return;
    this.#rejoin(role);
  }

  #rejoin(role: Role): void {
    this.#join(role).catch((error: unknown) => {
      warn("could not join Sora again", error);
    });
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
    this.#waiters.changed();
  }
}

/** Connects the user's session to the audio of a Sora channel. */
export function connectSora(options: VoiceConnectionOptions): VoiceConnection {
  return new SoraConnection(options);
}
