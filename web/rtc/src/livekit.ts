// SPDX-License-Identifier: AGPL-3.0-only
// The LiveKit RoomSession (ADR-0101 §12, §15): one Room of livekit-client, joined with the grant's URL and access
// token; the identity of the session in the room is its session ID. The SDK is a chunk of its own, loaded with
// import() only once a grant names LiveKit.
//
// The server changes what the session may do on LiveKit itself: a speaker's token, or UpdateParticipant, lets it
// publish its microphone (canPublishSources: ["microphone"]); a listener's does not, and taking the permission away
// unpublishes the track. Turning the microphone on waits for that permission (ParticipantPermissionsChanged), and a
// session that the latest roster no longer lists as a speaker unpublishes and stops its microphone at once.
//
// The stability of the call is mostly the SDK's: it resumes or restarts its connection by itself (Reconnecting,
// Reconnected), renews the access token while connected, and selects the default device again when the one in use
// goes away. A renewed grant is used only to join again after the room went away without the user leaving
// (Disconnected, `lost`); a grant to reconnect with joins again at once. A Disconnected whose reason says the server
// removed the session (removed, the same identity joined elsewhere, the room deleted) is the session's failure.
//
// Who is talking is LiveKit's own: its active speakers (which list the local participant too), by identity, which is
// the session. Only a speaker of the latest roster talks, and the user only while the microphone is on.
import type * as LiveKit from "livekit-client";
import { SessionCore, warn } from "./core";
import { RemoteAudios, Waiters } from "./sfu";
import { sameSessions } from "./talking";
import {
  type IncomingSignal,
  type LiveKitSdk,
  type RoomGrant,
  type RoomRoster,
  type RoomSession,
  type RoomSessionCapabilities,
  type RoomSessionEvents,
  RoomSessionFailure,
  type RoomSessionOptions,
  type RoomSessionSnapshot,
} from "./types";

/** The microphone's processing (ADR-0101 §7), as LiveKit captures it. */
const audioCapture: LiveKit.AudioCaptureOptions = {
  echoCancellation: true,
  noiseSuppression: true,
  autoGainControl: true,
};

class LiveKitSession implements RoomSession {
  readonly kind = "livekit";
  readonly session: string;
  readonly capabilities: RoomSessionCapabilities;
  readonly #core: SessionCore;
  readonly #sdk: Promise<LiveKitSdk>;
  readonly #waiters = new Waiters();
  #roster: RoomRoster = { speaker: false, members: [] };
  #audios: RemoteAudios;
  /** The remote audio tracks of the room, by their SID, for the statistics. */
  readonly #tracks = new Map<string, LiveKit.RemoteTrack>();
  /** The room of the latest join, or null before it connected and after it went away. */
  #room: LiveKit.Room | null = null;
  /** The latest join: the room once it connected, null when it did not. */
  #joined: Promise<LiveKit.Room | null>;
  #attempt = 0;
  /** Whether the room went away without the user leaving: the next grant joins again. */
  #lost = false;
  /** Whether the user wants the microphone on: a join after the room went away publishes it again. */
  #micOn = false;
  #microphoneChanges: Promise<void> = Promise.resolve();
  /** The identities of LiveKit's latest active speakers. */
  #activeSpeakers: readonly string[] = [];
  #talking: ReadonlySet<string> = new Set();
  /** Whether the SDK reconnects by itself now. */
  #reconnecting = false;

  constructor(options: RoomSessionOptions) {
    this.session = options.session;
    this.#core = new SessionCore(options.environment);
    this.capabilities = { outputSelection: options.environment.setOutputDevice !== undefined };
    this.#audios = this.#newAudios();
    const load = options.environment.loadLiveKit ?? (() => import("livekit-client"));
    this.#sdk = load();
    this.#sdk.catch((error: unknown) => {
      this.#core.fail(new RoomSessionFailure("unavailable", error));
      this.close();
    });
    this.#joined = this.#join(options.grant);
    this.#core.sampleQuality(() => this.#stats(), options.qualityInterval);
  }

  snapshot(): RoomSessionSnapshot {
    return this.#core.snapshot();
  }

  on<E extends keyof RoomSessionEvents>(event: E, listener: RoomSessionEvents[E]): () => void {
    return this.#core.on(event, listener);
  }

  async #stats(): Promise<(readonly [string, RTCStatsReport])[]> {
    const room = this.#room;
    const tracks: [string, { getRTCStatsReport: () => Promise<RTCStatsReport | undefined> }][] = [...this.#tracks];
    const local = room?.localParticipant.getTrackPublication((await this.#sdk).Track.Source.Microphone)?.track;
    if (local !== undefined) tracks.push(["microphone", local]);
    const reports = await Promise.all(
      tracks.map(async ([key, track]) => [key, await track.getRTCStatsReport()] as const),
    );
    return reports.flatMap(([key, report]) => (report === undefined ? [] : [[key, report] as const]));
  }

  #newAudios(): RemoteAudios {
    return new RemoteAudios(this.#core, this.#roster);
  }

  #reportConnection(): void {
    const room = this.#room;
    const participants =
      room === null
        ? []
        : [...room.remoteParticipants.values()]
            .map((participant) => ({ session: participant.identity, state: "connected" as const }))
            .sort((a, b) => (a.session < b.session ? -1 : a.session > b.session ? 1 : 0));
    const state = this.#lost
      ? "lost"
      : room === null
        ? "connecting"
        : this.#reconnecting
          ? "reconnecting"
          : "connected";
    this.#core.setConnection({
      state,
      participants: state === "connected" || state === "reconnecting" ? participants : [],
    });
  }

  async #join(grant: RoomGrant): Promise<LiveKit.Room | null> {
    const attempt = ++this.#attempt;
    this.#lost = false;
    this.#reconnecting = false;
    this.#reportConnection();
    let sdk: LiveKitSdk;
    try {
      sdk = await this.#sdk;
    } catch {
      return null;
    }
    if (this.#core.closed() || attempt !== this.#attempt) return null;
    const room = new sdk.Room({
      adaptiveStream: false,
      dynacast: false,
      audioCaptureDefaults: audioCapture,
      stopLocalTrackOnUnpublish: true,
    });
    let connected = false;
    room
      .on(sdk.RoomEvent.TrackSubscribed, (track, publication, participant) => {
        if (room !== this.#room || track.kind !== sdk.Track.Kind.Audio) return;
        this.#tracks.set(publication.trackSid, track);
        this.#audios.play(
          publication.trackSid,
          participant.identity,
          this.#core.environment.createStream([track.mediaStreamTrack]),
        );
      })
      .on(sdk.RoomEvent.TrackUnsubscribed, (_track, publication) => {
        if (room !== this.#room) return;
        this.#tracks.delete(publication.trackSid);
        this.#audios.remove(publication.trackSid);
      })
      .on(sdk.RoomEvent.ParticipantConnected, () => {
        if (room === this.#room && connected) this.#reportConnection();
      })
      .on(sdk.RoomEvent.ParticipantDisconnected, () => {
        if (room === this.#room && connected) this.#reportConnection();
      })
      .on(sdk.RoomEvent.ParticipantPermissionsChanged, (_previous, participant) => {
        if (participant === room.localParticipant) this.#waiters.changed();
      })
      .on(sdk.RoomEvent.ActiveSpeakersChanged, (speakers) => {
        if (room !== this.#room) return;
        this.#activeSpeakers = speakers.map((speaker) => speaker.identity);
        this.#reportTalking();
      })
      .on(sdk.RoomEvent.Reconnecting, () => {
        if (room !== this.#room) return;
        this.#reconnecting = true;
        this.#reportConnection();
      })
      .on(sdk.RoomEvent.Reconnected, () => {
        if (room !== this.#room) return;
        this.#reconnecting = false;
        this.#reportConnection();
      })
      .on(sdk.RoomEvent.Disconnected, (reason) => {
        if (room !== this.#room) return;
        this.#room = null;
        this.#resetAudios();
        if (this.#core.closed()) return;
        if (removedBy(sdk, reason)) {
          this.#core.fail(new RoomSessionFailure("removed", reason));
          this.close();
          return;
        }
        this.#lost = true;
        this.#reportConnection();
      });
    this.#room = room;
    try {
      await room.connect(grant.url ?? "", grant.token ?? "", { autoSubscribe: true });
    } catch (error) {
      warn("could not join the LiveKit room", error);
      if (room === this.#room) {
        this.#room = null;
        this.#lost = !this.#core.closed();
        this.#reportConnection();
      }
      return null;
    }
    if (room !== this.#room) {
      void room.disconnect();
      return null;
    }
    connected = true;
    this.#reportConnection();
    await this.#applyRole(sdk, room);
    return room;
  }

  #resetAudios(): void {
    this.#audios.close();
    this.#audios = this.#newAudios();
    this.#tracks.clear();
    this.#activeSpeakers = [];
    this.#reportTalking();
  }

  /** Reports the active speakers who talk: the speakers of the latest roster, and the user while the microphone is on. */
  #reportTalking(): void {
    const speakers = new Set(this.#roster.members.filter((member) => member.speaker).map((member) => member.session));
    const self = this.session;
    const talking = new Set(
      this.#activeSpeakers.filter((identity) =>
        identity === self ? this.#micOn && this.#roster.speaker : speakers.has(identity),
      ),
    );
    this.#core.setMicrophone(this.#micOn && this.#roster.speaker && this.#room !== null);
    if (sameSessions(talking, this.#talking)) return;
    this.#talking = talking;
    this.#core.setTalking(talking);
  }

  /** After a join: stops the microphone of a session that does not speak, and publishes it again for one whose microphone was on. */
  async #applyRole(sdk: LiveKitSdk, room: LiveKit.Room): Promise<void> {
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

  async #unpublish(sdk: LiveKitSdk, room: LiveKit.Room): Promise<void> {
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
      () => this.#core.closed() || room.localParticipant.permissions?.canPublish === true,
      "LiveKit did not let the session publish",
    );
    if (!this.#mayStillSpeak(room)) return;
    await room.localParticipant.setMicrophoneEnabled(true);
  }

  #mayStillSpeak(room: LiveKit.Room): boolean {
    return !this.#core.closed() && this.#roster.speaker && room === this.#room;
  }

  update(roster: RoomRoster): void {
    if (this.#core.closed()) return;
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
    const change = this.#microphoneChanges
      .then(() => this.#applyMicrophone())
      .catch((error: unknown) => {
        // Refused (by the browser or LiveKit): the microphone is off, unless a later call asked again.
        if (on && this.#micOn) {
          this.#micOn = false;
          this.#reportTalking();
        }
        throw error;
      });
    this.#microphoneChanges = change.catch(() => undefined);
    return change;
  }

  /** Applies the microphone the user wants now (the latest `setMicrophone`); one change runs at a time. */
  async #applyMicrophone(): Promise<void> {
    if (this.#core.closed()) return;
    const room = await this.#joined;
    if (this.#core.closed()) return;
    const on = this.#micOn && this.#roster.speaker;
    if (room === null || room !== this.#room) {
      if (on) throw new Error("rtc: not connected to LiveKit");
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

  setOutputDevice(deviceId: string): Promise<void> {
    return this.#core.setOutputDevice(deviceId);
  }

  renew(grant: RoomGrant): void {
    if (this.#core.closed() || !this.#lost) return;
    this.#joined = this.#join(grant);
  }

  reconnect(grant: RoomGrant): void {
    if (this.#core.closed()) return;
    const room = this.#room;
    this.#room = null;
    this.#resetAudios();
    if (room !== null) void room.disconnect();
    this.#joined = this.#join(grant);
  }

  receiveSignal(_signal: IncomingSignal): void {
    // LiveKit signals on its own connection; the room stream carries no signals for it.
  }

  async resumePlayback(): Promise<void> {
    await this.#core.resume(() => this.#audios.resume());
  }

  close(): void {
    if (this.#core.closed()) return;
    const room = this.#room;
    this.#room = null;
    this.#audios.close();
    this.#tracks.clear();
    this.#activeSpeakers = [];
    this.#core.close();
    this.#waiters.changed();
    if (room !== null) void room.disconnect();
  }
}

/** Whether LiveKit disconnected because the server removed the session. */
function removedBy(sdk: LiveKitSdk, reason: LiveKit.DisconnectReason | undefined): boolean {
  return (
    reason === sdk.DisconnectReason.PARTICIPANT_REMOVED ||
    reason === sdk.DisconnectReason.DUPLICATE_IDENTITY ||
    reason === sdk.DisconnectReason.ROOM_DELETED
  );
}

/** The LiveKit session of `options`. */
export function createLiveKitSession(options: RoomSessionOptions): RoomSession {
  return new LiveKitSession(options);
}
