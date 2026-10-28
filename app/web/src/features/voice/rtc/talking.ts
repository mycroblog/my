// SPDX-License-Identifier: AGPL-3.0-only
// Who is talking in a call, for the methods that measure it themselves (P2P and Sora; LiveKit reports its active
// speakers): the level of each remote stream and of the user's microphone, from a LevelMeter of the environment (an
// AnalyserNode on the call's one AudioContext), sampled every `talkingSample` ms while anything is measured. A stream
// starts talking when its level reaches `talkingThreshold`, and stops only after it stayed below it for `talkingHold`
// ms, so that the state does not flicker between words. The set of talking sessions goes to `onChange` only when it
// changes. Without a LevelMeter (no Web Audio) nothing is measured and nobody talks.
import type { LevelMeter, VoiceEnvironment } from "./types";

/** How often the levels are read. */
export const talkingSample = 100;

/** The level (RMS, 0–1) from which a stream talks; speech through the microphone's gain control is well above it. */
export const talkingThreshold = 0.02;

/** How long a stream keeps talking after its level fell below the threshold. */
export const talkingHold = 400;

/** How many quiet samples in a row end talking. */
const holdSamples = Math.ceil(talkingHold / talkingSample);

type Entry = { stream: MediaStream; meter: LevelMeter; session: string | undefined; talking: boolean; quiet: number };

/** Whether two sets have the same members. */
export function sameSessions(a: ReadonlySet<string>, b: ReadonlySet<string>): boolean {
  return a.size === b.size && [...a].every((session) => b.has(session));
}

/** Measures the streams of one connection, each under a key of the connection's choosing, and reports who talks. */
export class TalkingDetector {
  readonly #createMeter: VoiceEnvironment["createLevelMeter"];
  readonly #onChange: (sessions: ReadonlySet<string>) => void;
  readonly #entries = new Map<string, Entry>();
  #timer: ReturnType<typeof setInterval> | undefined;
  #reported: ReadonlySet<string> = new Set();
  #closed = false;

  constructor(environment: VoiceEnvironment, onChange: ((sessions: ReadonlySet<string>) => void) | undefined) {
    this.#createMeter = environment.createLevelMeter;
    this.#onChange = onChange ?? (() => undefined);
  }

  /**
   * Measures `stream` of `session` (undefined while the SDK has not said whose it is) under `key`, in place of what
   * the key measured before; the same stream again only names its session.
   */
  watch(key: string, session: string | undefined, stream: MediaStream): void {
    if (this.#closed) return;
    const entry = this.#entries.get(key);
    if (entry?.stream === stream) {
      this.name(key, session);
      return;
    }
    this.unwatch(key);
    const meter = this.#createMeter?.(stream) ?? null;
    if (meter === null) return;
    this.#entries.set(key, { stream, meter, session, talking: false, quiet: 0 });
    this.#timer ??= setInterval(() => {
      this.#sample();
    }, talkingSample);
  }

  /** Names the session of the stream under `key`. */
  name(key: string, session: string | undefined): void {
    const entry = this.#entries.get(key);
    if (entry === undefined || entry.session === session) return;
    entry.session = session;
    this.#report();
  }

  /** Stops measuring the stream under `key` (the track went away, or it is not heard). */
  unwatch(key: string): void {
    const entry = this.#entries.get(key);
    if (entry === undefined) return;
    entry.meter.close();
    this.#entries.delete(key);
    if (this.#entries.size === 0) this.#stopTimer();
    this.#report();
  }

  /** Releases every meter; nobody talks after it. */
  close(): void {
    if (this.#closed) return;
    for (const key of [...this.#entries.keys()]) this.unwatch(key);
    this.#closed = true;
    this.#stopTimer();
  }

  #stopTimer(): void {
    clearInterval(this.#timer);
    this.#timer = undefined;
  }

  #sample(): void {
    for (const entry of this.#entries.values()) {
      if (entry.meter.level() >= talkingThreshold) {
        entry.talking = true;
        entry.quiet = 0;
      } else if (entry.talking && ++entry.quiet >= holdSamples) {
        entry.talking = false;
        entry.quiet = 0;
      }
    }
    this.#report();
  }

  #report(): void {
    const sessions = new Set<string>();
    for (const entry of this.#entries.values())
      if (entry.talking && entry.session !== undefined) sessions.add(entry.session);
    if (sameSessions(sessions, this.#reported)) return;
    this.#reported = sessions;
    this.#onChange(sessions);
  }
}
