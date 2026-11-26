// SPDX-License-Identifier: AGPL-3.0-only
// The audio of the remote participants of `sfu` (ADR-0101 §15): each in a RemoteAudio of the environment and heard
// only while the latest roster lists its session as a speaker (the receiver's side of the enforcement, as in P2P).
import type { SessionCore } from "./core";
import type { RemoteAudio, RoomRoster } from "./types";

type Entry = { audio: RemoteAudio; session: string | undefined; stream: MediaStream };

/** The audio of the remote participants of one connection to a media node, keyed by the mid of each stream. */
export class RemoteAudios {
  readonly #core: SessionCore;
  readonly #entries = new Map<string, Entry>();
  #speakers: ReadonlySet<string>;

  constructor(core: SessionCore, roster: RoomRoster) {
    this.#core = core;
    this.#speakers = speakers(roster);
  }

  /** The sessions whose audio plays now, heard or not. */
  sessions(): string[] {
    return [...new Set([...this.#entries.values()].flatMap((entry) => entry.session ?? []))].sort();
  }

  #audible(entry: Entry): boolean {
    return entry.session !== undefined && this.#speakers.has(entry.session);
  }

  /** Plays `stream` of the participant `session` (undefined while the node has not said whose it is). */
  play(key: string, session: string | undefined, stream: MediaStream): void {
    this.remove(key);
    const entry: Entry = { audio: this.#core.environment.createRemoteAudio(), session, stream };
    entry.audio.setAudible(this.#audible(entry));
    this.#entries.set(key, entry);
    this.#core.play(entry.audio, stream);
  }

  /** Names the participant of a stream that played before the node said whose it is. */
  setSession(key: string, session: string): void {
    const entry = this.#entries.get(key);
    if (entry === undefined) return;
    entry.session = session;
    entry.audio.setAudible(this.#audible(entry));
  }

  remove(key: string): void {
    this.#entries.get(key)?.audio.close();
    this.#entries.delete(key);
  }

  /** Hears only the speakers of `roster`. */
  update(roster: RoomRoster): void {
    this.#speakers = speakers(roster);
    for (const entry of this.#entries.values()) entry.audio.setAudible(this.#audible(entry));
  }

  async resume(): Promise<void> {
    await Promise.all([...this.#entries.values()].map((entry) => entry.audio.resume()));
  }

  close(): void {
    for (const entry of this.#entries.values()) entry.audio.close();
    this.#entries.clear();
  }
}

function speakers(roster: RoomRoster): ReadonlySet<string> {
  return new Set(roster.members.filter((member) => member.speaker).map((member) => member.session));
}
