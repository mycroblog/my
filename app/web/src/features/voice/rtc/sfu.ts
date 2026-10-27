// SPDX-License-Identifier: AGPL-3.0-only
// What the SFU methods (livekit, sora; ADR-0102 §6) share behind the VoiceConnection of ADR-0101 §7: the audio of the
// remote participants, each in a RemoteAudio of the environment and heard only while the latest `state` lists its
// session as a speaker (the receiver's side of the enforcement, as in P2P), and the wait for a change that the server
// makes on its own path (a permission, a grant) before the microphone can go on. A TalkingDetector given to the remote
// audio measures each stream while it is heard (Sora; LiveKit reports its active speakers instead).
import type { TalkingDetector } from "./talking";
import type { RemoteAudio, VoiceEnvironment, VoiceRoster } from "./types";

/** How long turning the microphone on waits for the server to let the session send (a permission or a grant). */
export const sendWait = 10_000;

export function warn(message: string, error: unknown): void {
  console.warn(`voice: ${message}`, error);
}

type Entry = { audio: RemoteAudio; session: string | undefined; stream: MediaStream };

/** The remote audios made so far: each measures under keys of its own, so that two connections never share one. */
let audiosMade = 0;

/** The audio of the remote participants of one connection to an SFU, keyed by the SDK's ID of each stream. */
export class RemoteAudios {
  readonly #environment: VoiceEnvironment;
  readonly #onPlaybackBlocked: () => void;
  readonly #entries = new Map<string, Entry>();
  #speakers: ReadonlySet<string>;
  readonly #talking: TalkingDetector | undefined;
  readonly #prefix = `${String(++audiosMade)}:`;

  constructor(environment: VoiceEnvironment, onPlaybackBlocked: () => void, roster: VoiceRoster, talking?: TalkingDetector) {
    this.#environment = environment;
    this.#onPlaybackBlocked = onPlaybackBlocked;
    this.#speakers = speakers(roster);
    this.#talking = talking;
  }

  /** Measures the stream of `key` while it is heard: a participant who does not speak never talks. */
  #measure(key: string, entry: Entry): void {
    if (this.#audible(entry)) this.#talking?.watch(this.#prefix + key, entry.session, entry.stream);
    else this.#talking?.unwatch(this.#prefix + key);
  }

  #audible(entry: Entry): boolean {
    return entry.session !== undefined && this.#speakers.has(entry.session);
  }

  /** Plays `stream` of the participant `session` (undefined while the SDK has not said whose it is). */
  play(key: string, session: string | undefined, stream: MediaStream): void {
    this.remove(key);
    const entry: Entry = { audio: this.#environment.createRemoteAudio(), session, stream };
    entry.audio.setAudible(this.#audible(entry));
    this.#entries.set(key, entry);
    this.#measure(key, entry);
    entry.audio.play(stream).catch((error: unknown) => {
      if (error instanceof DOMException && error.name === "NotAllowedError") this.#onPlaybackBlocked();
      else warn("could not play the audio", error);
    });
  }

  /** Names the participant of a stream that played before the SDK said whose it is. */
  setSession(key: string, session: string): void {
    const entry = this.#entries.get(key);
    if (entry === undefined) return;
    entry.session = session;
    entry.audio.setAudible(this.#audible(entry));
    this.#measure(key, entry);
  }

  remove(key: string): void {
    this.#entries.get(key)?.audio.close();
    this.#entries.delete(key);
    this.#talking?.unwatch(this.#prefix + key);
  }

  /** Hears only the speakers of `roster`. */
  update(roster: VoiceRoster): void {
    this.#speakers = speakers(roster);
    for (const [key, entry] of this.#entries) {
      entry.audio.setAudible(this.#audible(entry));
      this.#measure(key, entry);
    }
  }

  async resume(): Promise<void> {
    await Promise.all([...this.#entries.values()].map((entry) => entry.audio.resume()));
  }

  close(): void {
    for (const [key, entry] of this.#entries) {
      entry.audio.close();
      this.#talking?.unwatch(this.#prefix + key);
    }
    this.#entries.clear();
  }
}

function speakers(roster: VoiceRoster): ReadonlySet<string> {
  return new Set(roster.members.filter((member) => member.speaker).map((member) => member.session));
}

/** Changes that a connection waits for: `wait` resolves once `ready()` is true after a `changed()`, or rejects after `sendWait`. */
export class Waiters {
  readonly #waiting = new Set<() => void>();

  wait(ready: () => boolean, failure: string): Promise<void> {
    if (ready()) return Promise.resolve();
    return new Promise((resolve, reject) => {
      const check = () => {
        if (!ready()) return;
        clearTimeout(timer);
        this.#waiting.delete(check);
        resolve();
      };
      const timer = setTimeout(() => {
        this.#waiting.delete(check);
        reject(new Error(`voice: ${failure}`));
      }, sendWait);
      this.#waiting.add(check);
    });
  }

  changed(): void {
    for (const check of [...this.#waiting]) check();
  }
}
