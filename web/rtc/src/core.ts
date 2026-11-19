// SPDX-License-Identifier: AGPL-3.0-only
// What every kind of RoomSession shares: its listeners and the snapshot of what they heard, the periodic sample of
// the quality, the output device, and the microphone, which the session takes again when its device goes away (the
// track ends, or the device is no longer among the inputs after a `devicechange`).
import { qualityInterval as defaultQualityInterval, type LossCounters, qualityOf } from "./quality";
import { sameSessions } from "./talking";
import type {
  ConnectionQuality,
  ParticipantLink,
  RoomConnection,
  RoomEnvironment,
  RoomSessionEvents,
  RoomSessionFailure,
  RoomSessionSnapshot,
} from "./types";

export function warn(message: string, error: unknown): void {
  console.warn(`rtc: ${message}`, error);
}

/** Whether `error` is the browser's refusal to play audio without a gesture. */
function isAutoplayBlock(error: unknown): boolean {
  return error instanceof DOMException && error.name === "NotAllowedError";
}

type Listeners = { [E in keyof RoomSessionEvents]: Set<RoomSessionEvents[E]> };

function sameLinks(a: readonly ParticipantLink[], b: readonly ParticipantLink[]): boolean {
  return (
    a.length === b.length && a.every((link, i) => link.session === b.at(i)?.session && link.state === b.at(i)?.state)
  );
}

/** The listeners and the snapshot of one session. */
export class SessionCore {
  readonly environment: RoomEnvironment;
  readonly #listeners: Listeners = {
    connection: new Set(),
    talking: new Set(),
    playbackBlocked: new Set(),
    quality: new Set(),
    failed: new Set(),
  };
  #snapshot: RoomSessionSnapshot = {
    connection: { state: "connecting", participants: [] },
    talking: new Set(),
    playbackBlocked: false,
    microphone: false,
    quality: null,
    failure: null,
  };
  #qualityTimer: ReturnType<typeof setInterval> | undefined;
  readonly #lossCounters: LossCounters = new Map();
  #closed = false;

  constructor(environment: RoomEnvironment) {
    this.environment = environment;
  }

  /** Whether the session closed; a method, so that it is read again after an await. */
  closed(): boolean {
    return this.#closed;
  }

  snapshot(): RoomSessionSnapshot {
    return this.#snapshot;
  }

  on<E extends keyof RoomSessionEvents>(event: E, listener: RoomSessionEvents[E]): () => void {
    const listeners = this.#listeners[event] as Set<RoomSessionEvents[E]>;
    listeners.add(listener);
    return () => {
      listeners.delete(listener);
    };
  }

  #emit<E extends keyof RoomSessionEvents>(event: E, ...args: Parameters<RoomSessionEvents[E]>): void {
    for (const listener of [...(this.#listeners[event] as Set<(...a: Parameters<RoomSessionEvents[E]>) => void>)]) {
      try {
        listener(...args);
      } catch (error) {
        warn(`a listener of ${event} threw`, error);
      }
    }
  }

  setConnection(connection: RoomConnection): void {
    const now = this.#snapshot.connection;
    if (now.state === "closed") return;
    if (now.state === connection.state && sameLinks(now.participants, connection.participants)) return;
    this.#snapshot = { ...this.#snapshot, connection };
    this.#emit("connection", connection);
  }

  setTalking(sessions: ReadonlySet<string>): void {
    if (sameSessions(sessions, this.#snapshot.talking)) return;
    this.#snapshot = { ...this.#snapshot, talking: sessions };
    this.#emit("talking", sessions);
  }

  setPlaybackBlocked(blocked: boolean): void {
    if (this.#closed || this.#snapshot.playbackBlocked === blocked) return;
    this.#snapshot = { ...this.#snapshot, playbackBlocked: blocked };
    this.#emit("playbackBlocked", blocked);
  }

  /** Whether the microphone sends now; the snapshot alone says it. */
  setMicrophone(sending: boolean): void {
    if (this.#snapshot.microphone === sending) return;
    this.#snapshot = { ...this.#snapshot, microphone: sending };
  }

  #setQuality(quality: ConnectionQuality): void {
    if (this.#closed) return;
    this.#snapshot = { ...this.#snapshot, quality };
    this.#emit("quality", quality);
  }

  /** Plays `stream` on `audio`, saying when the browser blocks it. */
  play(audio: { play: (stream: MediaStream) => Promise<void> }, stream: MediaStream): void {
    audio.play(stream).catch((error: unknown) => {
      if (isAutoplayBlock(error)) this.setPlaybackBlocked(true);
      else warn("could not play the audio", error);
    });
  }

  /** Resumes what `resume` plays again, and says the audio plays once it did. */
  async resume(resume: () => Promise<void>): Promise<void> {
    await resume();
    this.setPlaybackBlocked(false);
  }

  /** Samples the quality of what `reports` gives every `interval` ms, until the session closes. */
  sampleQuality(
    reports: () => Promise<readonly (readonly [string, RTCStatsReport])[]>,
    interval: number = defaultQualityInterval,
  ): void {
    this.#qualityTimer = setInterval(() => {
      reports().then(
        (taken) => {
          if (taken.length > 0) this.#setQuality(qualityOf(taken, this.#lossCounters));
        },
        (error: unknown) => {
          warn("could not read the statistics", error);
        },
      );
    }, interval);
  }

  async setOutputDevice(deviceId: string): Promise<void> {
    const set = this.environment.setOutputDevice;
    if (set === undefined) throw new Error("rtc: the output device cannot be chosen here");
    await set(deviceId);
  }

  /** Says the terminal failure; the session closes itself after it. */
  fail(failure: RoomSessionFailure): void {
    if (this.#closed) return;
    this.#snapshot = { ...this.#snapshot, failure };
    this.#emit("failed", failure);
  }

  /** Ends the snapshot as closed, stops the sampling and releases the environment. Idempotent. */
  close(): void {
    if (this.#closed) return;
    this.#closed = true;
    clearInterval(this.#qualityTimer);
    this.setTalking(new Set());
    this.setMicrophone(false);
    this.setConnection({ state: "closed", participants: [] });
    try {
      this.environment.close?.();
    } catch (error) {
      warn("could not release the environment", error);
    }
  }
}

/** The deviceId of `track`, where the browser says it. */
function deviceOf(track: MediaStreamTrack): string | undefined {
  return typeof track.getSettings === "function" ? track.getSettings().deviceId : undefined;
}

/**
 * The microphone of a session: taken from the environment once, and taken again when its track ends or its device
 * is no longer among the inputs. `onReplaced` puts the new track where the old one was sent.
 */
export class Microphone {
  readonly #environment: RoomEnvironment;
  readonly #onReplaced: (track: MediaStreamTrack, stream: MediaStream) => Promise<void> | void;
  #stream: MediaStream | null = null;
  #recovering: Promise<void> | null = null;

  constructor(
    environment: RoomEnvironment,
    onReplaced: (track: MediaStreamTrack, stream: MediaStream) => Promise<void> | void,
  ) {
    this.#environment = environment;
    this.#onReplaced = onReplaced;
  }

  /** The stream held now; read again after an await. */
  get stream(): MediaStream | null {
    return this.#stream;
  }

  track(): MediaStreamTrack | null {
    return this.#stream?.getAudioTracks()[0] ?? null;
  }

  /** Takes the microphone; null when `keep()` said no more by the time the browser gave it (its tracks stopped). */
  async acquire(keep: () => boolean): Promise<MediaStream | null> {
    if (this.#stream !== null) return this.#stream;
    const stream = await this.#environment.getMicrophone();
    // Another request took it while the browser asked, or the session no longer wants it.
    const held = this.stream;
    if (!keep() || held !== null) {
      for (const track of stream.getTracks()) track.stop();
      return held;
    }
    this.#hold(stream);
    return stream;
  }

  #hold(stream: MediaStream): void {
    this.#stream = stream;
    const track = stream.getAudioTracks()[0];
    if (track !== undefined)
      track.onended = () => {
        if (this.#stream === stream) void this.recover();
      };
  }

  setEnabled(enabled: boolean): void {
    for (const track of this.#stream?.getAudioTracks() ?? []) track.enabled = enabled;
  }

  /** Takes the microphone again when its track ended or its device went away (`devicechange`). */
  recover(): Promise<void> {
    this.#recovering ??= this.#recover().finally(() => {
      this.#recovering = null;
    });
    return this.#recovering;
  }

  async #recover(): Promise<void> {
    const held = this.#stream;
    const track = held?.getAudioTracks()[0];
    if (held === null || track === undefined) return;
    if (track.readyState !== "ended") {
      const device = deviceOf(track);
      const inputs = await this.#environment.audioInputs?.();
      if (device === undefined || inputs === undefined || inputs.includes(device)) return;
    }
    let stream: MediaStream;
    try {
      stream = await this.#environment.getMicrophone();
    } catch (error) {
      warn("could not take the microphone again", error);
      return;
    }
    if (this.#stream !== held) {
      for (const t of stream.getTracks()) t.stop();
      return;
    }
    const next = stream.getAudioTracks()[0];
    if (next === undefined) return;
    next.enabled = track.enabled;
    this.#hold(stream);
    try {
      await this.#onReplaced(next, stream);
    } catch (error) {
      warn("could not send the new microphone", error);
    }
    for (const t of held.getTracks()) t.stop();
  }

  stop(): void {
    for (const track of this.#stream?.getTracks() ?? []) {
      track.onended = null;
      track.stop();
    }
    this.#stream = null;
  }
}
