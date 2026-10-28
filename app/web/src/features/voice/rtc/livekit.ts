// SPDX-License-Identifier: AGPL-3.0-only
// The LiveKit method of the audio of a call (ADR-0102 §3, §6): one Room of livekit-client, joined with the grant's URL
// and access token; the identity of the session in the room is its session ID. The SDK is a chunk of its own, loaded
// with import() only when a grant names LiveKit (the other screens never fetch it).
//
// The server changes what the session may do on LiveKit itself: a speaker's token, or UpdateParticipant, lets it
// publish its microphone (canPublishSources: ["microphone"]); a listener's does not, and taking the permission away
// unpublishes the track. Turning the microphone on waits for that permission (ParticipantPermissionsChanged), and a
// session that the latest `state` no longer lists as a speaker unpublishes and stops its microphone at once.
//
// While connected, LiveKit renews the access token itself, so a renewed grant is used only to join again after the
// room went away without the user leaving (Disconnected); a grant to reconnect with joins again at once.
//
// Who is talking is LiveKit's own: its active speakers (RoomEvent.ActiveSpeakersChanged, which lists the local
// participant too), by identity, which is the session. Only a speaker of the latest `state` talks, and the user only
// while the microphone is on; the set is empty while the room is down.
import type * as LiveKit from "livekit-client";
import { RemoteAudios, Waiters, warn } from "./sfu";
import { sameSessions } from "./talking";
import type { VoiceConnection, VoiceConnectionOptions, VoiceGrant, VoiceRoster } from "./types";

type Sdk = typeof LiveKit;

/** The microphone's processing (ADR-0101 §7), as LiveKit captures it. */
const audioCapture: LiveKit.AudioCaptureOptions = {
  echoCancellation: true,
  noiseSuppression: true,
  autoGainControl: true,
};

class LiveKitConnection implements VoiceConnection {
  readonly #options: VoiceConnectionOptions;
  readonly #sdk: Promise<Sdk>;
  readonly #waiters = new Waiters();
  #roster: VoiceRoster = { speaker: false, members: [] };
  #audios: RemoteAudios;
  /** The room of the latest join, or null before it connected and after it went away. */
  #room: LiveKit.Room | null = null;
  /** The latest join: the room once it connected, null when it did not. */
  #joined: Promise<LiveKit.Room | null>;
  #attempt = 0;
  /** Whether the room went away without the user leaving: the next grant joins again. */
  #lost = false;
  /** Whether the user wants the microphone on: a join after the room went away publishes it again. */
  #micOn = false;
  /** The changes of the microphone, one after another. */
  #microphoneChanges: Promise<void> = Promise.resolve();
  /** The identities of LiveKit's latest active speakers, and the talking sessions reported from them. */
  #activeSpeakers: readonly string[] = [];
  #talking: ReadonlySet<string> = new Set();
  #closed = false;

  constructor(options: VoiceConnectionOptions) {
    this.#options = options;
    this.#audios = this.#newAudios();
    this.#sdk = import("livekit-client");
    this.#joined = this.#join(options.grant);
  }

  #newAudios(): RemoteAudios {
    return new RemoteAudios(this.#options.environment, this.#options.events.onPlaybackBlocked, this.#roster);
  }

  async #join(grant: VoiceGrant): Promise<LiveKit.Room | null> {
    const attempt = ++this.#attempt;
    this.#lost = false;
    let sdk: Sdk;
    try {
      sdk = await this.#sdk;
    } catch (error) {
      warn("could not load LiveKit", error);
      if (attempt === this.#attempt) this.#lost = true;
      return null;
    }
    if (this.#closed || attempt !== this.#attempt) return null;
    const room = new sdk.Room({
      adaptiveStream: false,
      dynacast: false,
      audioCaptureDefaults: audioCapture,
      stopLocalTrackOnUnpublish: true,
    });
    room
      .on(sdk.RoomEvent.TrackSubscribed, (track, publication, participant) => {
        if (room !== this.#room || track.kind !== sdk.Track.Kind.Audio) return;
        this.#audios.play(publication.trackSid, participant.identity, new MediaStream([track.mediaStreamTrack]));
      })
      .on(sdk.RoomEvent.TrackUnsubscribed, (_track, publication) => {
        if (room === this.#room) this.#audios.remove(publication.trackSid);
      })
      .on(sdk.RoomEvent.ParticipantPermissionsChanged, (_previous, participant) => {
        if (participant === room.localParticipant) this.#waiters.changed();
      })
      .on(sdk.RoomEvent.ActiveSpeakersChanged, (speakers) => {
        if (room !== this.#room) return;
        this.#activeSpeakers = speakers.map((speaker) => speaker.identity);
        this.#reportTalking();
      })
      .on(sdk.RoomEvent.Disconnected, () => {
        if (room !== this.#room) return;
        this.#room = null;
        this.#lost = !this.#closed;
        this.#resetAudios();
      });
    this.#room = room;
    try {
      await room.connect(grant.url ?? "", grant.token ?? "", { autoSubscribe: true });
    } catch (error) {
      warn("could not join the LiveKit room", error);
      if (room === this.#room) {
        this.#room = null;
        this.#lost = !this.#closed;
      }
      return null;
    }
    if (room !== this.#room) {
      void room.disconnect();
      return null;
    }
    await this.#applyRole(sdk, room);
    return room;
  }

  #resetAudios(): void {
    this.#audios.close();
    this.#audios = this.#newAudios();
    this.#activeSpeakers = [];
    this.#reportTalking();
  }

  /** Reports the active speakers who talk: the speakers of the latest `state`, and the user while the microphone is on. */
  #reportTalking(): void {
    const speakers = new Set(this.#roster.members.filter((member) => member.speaker).map((member) => member.session));
    const self = this.#options.session;
    const talking = new Set(
      this.#activeSpeakers.filter((identity) =>
        identity === self ? this.#micOn && this.#roster.speaker : speakers.has(identity),
      ),
    );
    if (sameSessions(talking, this.#talking)) return;
    this.#talking = talking;
    this.#options.events.onTalking?.(talking);
  }

  /** After a join: stops the microphone of a session that does not speak, and publishes it again for one whose microphone was on. */
  async #applyRole(sdk: Sdk, room: LiveKit.Room): Promise<void> {
    if (!this.#roster.speaker) {
      await this.#unpublish(sdk, room);
      return;
    }
    // Without waiting for it: the microphone goes on again once LiveKit lets the session publish.
    if (this.#micOn) {
      this.#publish(room).catch((error: unknown) => {
        warn("could not publish the microphone again", error);
      });
    }
  }

  async #unpublish(sdk: Sdk, room: LiveKit.Room): Promise<void> {
    this.#micOn = false;
    this.#reportTalking();
    const track = room.localParticipant.getTrackPublication(sdk.Track.Source.Microphone)?.track;
    if (track === undefined) return;
    try {
      await room.localParticipant.unpublishTrack(track, true);
    } catch (error) {
      warn("could not stop the microphone", error);
    }
  }

  /** Publishes the microphone once LiveKit lets the session publish it. */
  async #publish(room: LiveKit.Room): Promise<void> {
    await this.#waiters.wait(
      () => this.#closed || room.localParticipant.permissions?.canPublish === true,
      "LiveKit did not let the session publish",
    );
    if (!this.#mayStillSpeak(room)) return;
    await room.localParticipant.setMicrophoneEnabled(true);
  }

  /** Whether the user left; a method, so that it is read again after an await. */
  #isClosed(): boolean {
    return this.#closed;
  }

  #mayStillSpeak(room: LiveKit.Room): boolean {
    return !this.#closed && this.#roster.speaker && room === this.#room;
  }

  update(roster: VoiceRoster): void {
    if (this.#closed) return;
    this.#roster = roster;
    this.#audios.update(roster);
    this.#reportTalking();
    const room = this.#room;
    if (!roster.speaker && room !== null) {
      void this.#sdk.then((sdk) => this.#unpublish(sdk, room));
    }
  }

  setMicrophone(on: boolean): Promise<void> {
    this.#micOn = on;
    this.#reportTalking();
    const change = this.#microphoneChanges.then(() => this.#applyMicrophone());
    this.#microphoneChanges = change.catch(() => undefined);
    return change;
  }

  /** Applies the microphone the user wants now (the latest `setMicrophone`); one change runs at a time. */
  async #applyMicrophone(): Promise<void> {
    if (this.#closed) return;
    const room = await this.#joined;
    if (this.#isClosed()) return;
    const on = this.#micOn && this.#roster.speaker;
    if (room === null || room !== this.#room) {
      if (on) throw new Error("voice: not connected to LiveKit");
      return;
    }
    if (!on) {
      await room.localParticipant.setMicrophoneEnabled(false);
      return;
    }
    await this.#publish(room);
    if (!this.#mayStillSpeak(room)) {
      // The user stopped speaking, or left, while LiveKit or the browser was asked.
      const sdk = await this.#sdk;
      await this.#unpublish(sdk, room);
    }
  }

  renew(grant: VoiceGrant): void {
    if (this.#closed || !this.#lost) return;
    this.#joined = this.#join(grant);
  }

  reconnect(grant: VoiceGrant): void {
    if (this.#closed) return;
    const room = this.#room;
    this.#room = null;
    this.#resetAudios();
    if (room !== null) void room.disconnect();
    this.#joined = this.#join(grant);
  }

  receive(): void {
    // LiveKit signals on its own connection; the room stream carries no signals for it.
  }

  resumePlayback(): Promise<void> {
    return this.#audios.resume();
  }

  close(): void {
    if (this.#closed) return;
    this.#closed = true;
    const room = this.#room;
    this.#room = null;
    this.#audios.close();
    this.#activeSpeakers = [];
    this.#reportTalking();
    this.#waiters.changed();
    if (room !== null) void room.disconnect();
  }
}

/** Connects the user's session to the audio of a LiveKit room. */
export function connectLiveKit(options: VoiceConnectionOptions): VoiceConnection {
  return new LiveKitConnection(options);
}
