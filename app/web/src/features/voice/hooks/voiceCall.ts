// SPDX-License-Identifier: AGPL-3.0-only
// The user's session in a room while the call lasts (ADR-0101 §2, §3, §5, §6; ADR-0100 §5, §6): an external system
// that the call's hook keeps in sync with React through subscribe / getSnapshot (useSyncExternalStore).
//
// - It sends the session's heartbeat every 15 seconds. A 404 says the session is no longer the user's (it expired
//   while the device was away): it joins again with a new session.
// - It keeps the room stream open with the same session, opening it again after it ends or fails (with a growing
//   wait), and hands its events on: `state` to the Query cache and to the audio, `signal` to the audio, `mic` and
//   `reaction` to the snapshot, `muted` turns the microphone off, `grant` renews or reconnects, and `removed`,
//   `replaced`, and `ended` end the call.
// - 10 minutes before the grant expires it asks for a new one and gives it to the audio (setConfiguration and
//   restartIce on every connection, for P2P).
// - It tells the hook what changed for the user (a role, a mute, a raised hand for the hosts) as notices, which the
//   hook announces.
// The requests and the audio are given to it, so that the tests give fakes.
import { connectVoice, UnsupportedVoiceMethodError } from "../rtc/connect";
import type { VoiceConnection, VoiceEnvironment, VoiceGrant, VoiceMember } from "../rtc/types";
import { isSessionGone, type VoiceGrantData, type VoiceSignalData, type VoiceStreamEvent, type VoiceStreamOptions } from "../api/call";
import type { VoiceRoom, VoiceRoomParticipant } from "../api/voice";

/** How often the session sends its heartbeat (ADR-0100 §6). */
export const heartbeatInterval = 15_000;

/** How long before a grant expires the session asks for a new one (ADR-0101 §5). */
export const grantRenewalLead = 10 * 60_000;

/** How long a reaction shows next to its participant. */
export const reactionDuration = 5_000;

/** The waits before opening the room stream again after failures in a row. */
const streamDelays = [1_000, 2_000, 5_000, 10_000, 30_000] as const;

/** The longest wait that setTimeout takes. */
const longestTimer = 2 ** 31 - 1;

/** The wait before asking again for a grant that the server could not give. */
const grantRetry = 30_000;

/** A session of the user in the room: what POST /voice/rooms/{id}/participation answered. */
export type VoiceJoin = { session: string; grant: VoiceGrantData; room: VoiceRoom };

/** What the call asks of the server, and where the room goes. */
export type VoiceCallServer = {
  /** Joins the room again with a new session. */
  join: () => Promise<VoiceJoin>;
  heartbeat: (session: string) => Promise<void>;
  refreshGrant: (session: string) => Promise<VoiceGrantData>;
  sendSignal: (session: string, signal: VoiceSignalData) => Promise<void>;
  setMic: (session: string, on: boolean) => Promise<void>;
  /** Reads the room stream of `session` until it ends (readVoiceStream). */
  openStream: (session: string, options: VoiceStreamOptions) => Promise<void>;
  /** The room as the stream has it: into the Query cache. */
  onRoom: (room: VoiceRoom) => void;
  /** The call ended: the page reads the room again. */
  onEnd: () => void;
};

/** Why the call ended: the stream's `removed`, `replaced`, or `ended` with its reason; `lost` when joining again failed; `unsupported` for a method this client does not have. */
export type VoiceCallEnd = { kind: "removed" | "replaced" | "ended" | "lost" | "unsupported"; reason: string };

/** A reaction that shows for a while next to the participant. */
export type VoiceReactionShown = { id: number; participant: string; emoji: string };

export type VoiceCallSnapshot = {
  /** `connecting` until the room stream first opens, `reconnecting` while it is down. */
  phase: "connecting" | "live" | "reconnecting" | "ended";
  end: VoiceCallEnd | undefined;
  /** Whether the user's microphone is on. */
  micOn: boolean;
  /** The browser did not give the microphone. */
  micFailed: boolean;
  /** The browser did not let the audio play: a button starts it. */
  playbackBlocked: boolean;
  /** The microphones of the participants (account ID → on), as the `mic` events said. */
  mics: ReadonlyMap<string, boolean>;
  reactions: readonly VoiceReactionShown[];
};

/** A change for the user that the page announces. */
export type VoiceCallNotice =
  | { kind: "speaker" | "listener" | "muted" | "hand-down" | "cohost" | "cohost-removed" | "host" | "reconnecting" | "reconnected" }
  | { kind: "hand-raised"; participant: VoiceRoomParticipant };

export type VoiceCallOptions = {
  /** The user's account. */
  accountId: string;
  join: VoiceJoin;
  server: VoiceCallServer;
  environment: VoiceEnvironment;
  onNotice: (notice: VoiceCallNotice) => void;
};

/** The grant as the audio takes it. */
function voiceGrant(grant: VoiceGrantData): VoiceGrant {
  return {
    kind: grant.kind,
    url: grant.url,
    token: grant.token,
    options: grant.options,
    iceServers: grant.ice_servers,
    relayOnly: grant.relay_only,
    expiresAt: Date.parse(grant.expires_at),
  };
}

/** The name of a participant as the page shows it. */
export function participantName(participant: VoiceRoomParticipant): string {
  return participant.account.display_name || participant.account.username;
}

/** Whether the participant hosts or co-hosts, who handles raised hands. */
function manages(participant: VoiceRoomParticipant | undefined): boolean {
  return participant !== undefined && (participant.host || participant.cohost);
}

export class VoiceCall {
  readonly #options: VoiceCallOptions;
  #session: string;
  #grant: VoiceGrant;
  #connection: VoiceConnection | null = null;
  #room: VoiceRoom;
  #snapshot: VoiceCallSnapshot;
  readonly #listeners = new Set<() => void>();
  readonly #stream = new AbortController();
  #streamFailures = 0;
  #heartbeat: ReturnType<typeof setInterval> | undefined;
  #renewal: ReturnType<typeof setTimeout> | undefined;
  readonly #reactionTimers = new Set<ReturnType<typeof setTimeout>>();
  #reactionId = 0;
  #rejoining = false;
  #started = false;

  constructor(options: VoiceCallOptions) {
    this.#options = options;
    this.#session = options.join.session;
    this.#grant = voiceGrant(options.join.grant);
    this.#room = options.join.room;
    this.#snapshot = { phase: "connecting", end: undefined, micOn: false, micFailed: false, playbackBlocked: false, mics: new Map(), reactions: [] };
  }

  subscribe = (listener: () => void): (() => void) => {
    this.#listeners.add(listener);
    return () => {
      this.#listeners.delete(listener);
    };
  };

  getSnapshot = (): VoiceCallSnapshot => this.#snapshot;

  #set(change: Partial<VoiceCallSnapshot>): void {
    this.#snapshot = { ...this.#snapshot, ...change };
    for (const listener of this.#listeners) listener();
  }

  /** The phase now; a method, so that it is read again after an await. */
  #phase(): VoiceCallSnapshot["phase"] {
    return this.#snapshot.phase;
  }

  /** Whether the call stopped reading its stream: it was closed or it ended. */
  #isStopped(): boolean {
    return this.#stream.signal.aborted || this.#isEnded();
  }

  #isEnded(): boolean {
    return this.#phase() === "ended";
  }

  get session(): string {
    return this.#session;
  }

  /** Connects the audio and starts the heartbeats, the room stream, and the renewal of the grant. */
  start(): void {
    if (this.#started) return;
    this.#started = true;
    if (!this.#connect()) return;
    this.#heartbeat = setInterval(() => {
      void this.#beat();
    }, heartbeatInterval);
    void this.#beat();
    this.#scheduleRenewal();
    void this.#readStream();
  }

  #connect(): boolean {
    try {
      this.#connection = connectVoice({
        session: this.#session,
        grant: this.#grant,
        environment: this.#options.environment,
        events: {
          sendSignal: ({ to, kind, sdp, candidate }) =>
            this.#options.server.sendSignal(this.#session, { to, kind, ...(sdp === undefined ? {} : { sdp }), ...(candidate === undefined ? {} : { candidate }) }),
          onPlaybackBlocked: () => {
            this.#set({ playbackBlocked: true });
          },
        },
      });
    } catch (error) {
      if (error instanceof UnsupportedVoiceMethodError) {
        this.#finish({ kind: "unsupported", reason: error.kind });
        return false;
      }
      throw error;
    }
    this.#applyRoster();
    return true;
  }

  #self(room: VoiceRoom = this.#room): VoiceRoomParticipant | undefined {
    return room.participants.find((p) => p.session === this.#session) ?? room.participants.find((p) => p.account.id === this.#options.accountId);
  }

  #applyRoster(): void {
    const self = this.#self();
    const members: VoiceMember[] = this.#room.participants.flatMap((p) =>
      p.session === undefined || p.session === this.#session ? [] : [{ session: p.session, account: p.account.id, speaker: p.role === "speaker" }],
    );
    this.#connection?.update({ speaker: self?.role === "speaker", members });
  }

  async #beat(): Promise<void> {
    if (this.#isEnded()) return;
    const session = this.#session;
    try {
      await this.#options.server.heartbeat(session);
    } catch (error) {
      if (isSessionGone(error) && session === this.#session) await this.#rejoin();
      // Any other failure (offline, a server that went away) is retried with the next heartbeat.
    }
  }

  /** Joins again with a new session, after the server took the previous one away. */
  async #rejoin(): Promise<void> {
    if (this.#rejoining || this.#isEnded()) return;
    this.#rejoining = true;
    try {
      const join = await this.#options.server.join();
      if (this.#isEnded()) return;
      this.#connection?.close();
      this.#session = join.session;
      this.#grant = voiceGrant(join.grant);
      this.#room = join.room;
      this.#options.server.onRoom(join.room);
      this.#set({ micOn: false });
      this.#scheduleRenewal();
      this.#connect();
    } catch {
      this.#finish({ kind: "lost", reason: "" });
    } finally {
      this.#rejoining = false;
    }
  }

  async #readStream(): Promise<void> {
    while (!this.#isStopped()) {
      const session = this.#session;
      let failed = false;
      try {
        await this.#options.server.openStream(session, {
          signal: this.#stream.signal,
          onOpen: () => {
            this.#streamFailures = 0;
            const wasDown = this.#snapshot.phase === "reconnecting";
            this.#set({ phase: "live" });
            if (wasDown) this.#options.onNotice({ kind: "reconnected" });
          },
          onEvent: (event) => {
            if (session === this.#session) this.#handle(event);
          },
        });
      } catch (error) {
        if (isSessionGone(error)) await this.#rejoin();
        failed = true;
      }
      if (this.#isStopped()) return;
      const phase = this.#phase();
      if (phase === "live") this.#options.onNotice({ kind: "reconnecting" });
      this.#set({ phase: phase === "connecting" ? "connecting" : "reconnecting" });
      if (failed) this.#streamFailures++;
      const wait = streamDelays[Math.min(Math.max(this.#streamFailures - 1, 0), streamDelays.length - 1)] ?? 30_000;
      await new Promise<void>((resolve) => {
        const timer = setTimeout(resolve, wait);
        this.#stream.signal.addEventListener(
          "abort",
          () => {
            clearTimeout(timer);
            resolve();
          },
          { once: true },
        );
      });
    }
  }

  #handle(event: VoiceStreamEvent): void {
    switch (event.type) {
      case "state":
        this.#state(event.room);
        return;
      case "mic":
        this.#set({ mics: new Map([...this.#snapshot.mics, [event.mic.participant, event.mic.on]]) });
        return;
      case "reaction":
        this.#react(event.reaction.participant, event.reaction.emoji);
        return;
      case "signal": {
        const { from, kind, sdp, candidate } = event.signal;
        this.#connection?.receive({ from, kind, sdp, candidate });
        return;
      }
      case "muted":
        if (this.#snapshot.micOn) void this.setMicrophone(false);
        this.#options.onNotice({ kind: "muted" });
        return;
      case "grant":
        this.#grant = voiceGrant(event.grant.grant);
        if (event.grant.reconnect) this.#connection?.reconnect(this.#grant);
        this.#scheduleRenewal();
        return;
      case "removed":
      case "replaced":
      case "ended":
        this.#finish({ kind: event.type, reason: event.reason });
        return;
    }
  }

  #state(room: VoiceRoom): void {
    const before = this.#self();
    const wasManager = manages(before);
    const previous = this.#room;
    this.#room = room;
    this.#options.server.onRoom(room);
    const after = this.#self(room);
    if (before !== undefined && after !== undefined) {
      if (before.role !== after.role) {
        this.#options.onNotice({ kind: after.role === "speaker" ? "speaker" : "listener" });
        if (after.role !== "speaker") this.#set({ micOn: false });
      } else if (before.hand_raised && !after.hand_raised) {
        this.#options.onNotice({ kind: "hand-down" });
      }
      if (!before.host && after.host) this.#options.onNotice({ kind: "host" });
      else if (!before.cohost && after.cohost) this.#options.onNotice({ kind: "cohost" });
      else if (before.cohost && !after.cohost && !after.host) this.#options.onNotice({ kind: "cohost-removed" });
    }
    if (wasManager && manages(after)) {
      const raised = new Set(previous.participants.filter((p) => p.hand_raised).map((p) => p.account.id));
      for (const participant of room.participants) {
        if (participant.hand_raised && !raised.has(participant.account.id) && participant.account.id !== this.#options.accountId) {
          this.#options.onNotice({ kind: "hand-raised", participant });
        }
      }
    }
    this.#applyRoster();
  }

  #react(participant: string, emoji: string): void {
    const id = ++this.#reactionId;
    this.#set({ reactions: [...this.#snapshot.reactions, { id, participant, emoji }] });
    const timer = setTimeout(() => {
      this.#reactionTimers.delete(timer);
      this.#set({ reactions: this.#snapshot.reactions.filter((r) => r.id !== id) });
    }, reactionDuration);
    this.#reactionTimers.add(timer);
  }

  #scheduleRenewal(): void {
    clearTimeout(this.#renewal);
    if (this.#isEnded()) return;
    // A timer of more than 2^31 - 1 ms fires at once: a grant that far away is looked at again later.
    const wait = Math.min(Math.max(this.#grant.expiresAt - grantRenewalLead - Date.now(), 5_000), longestTimer);
    if (wait === longestTimer) {
      this.#renewal = setTimeout(() => {
        this.#scheduleRenewal();
      }, wait);
      return;
    }
    this.#renewal = setTimeout(() => {
      void this.#renew();
    }, wait);
  }

  async #renew(): Promise<void> {
    const session = this.#session;
    try {
      const grant = await this.#options.server.refreshGrant(session);
      if (session !== this.#session || this.#isEnded()) return;
      this.#grant = voiceGrant(grant);
      this.#connection?.renew(this.#grant);
      this.#scheduleRenewal();
    } catch (error) {
      if (isSessionGone(error)) {
        await this.#rejoin();
        return;
      }
      clearTimeout(this.#renewal);
      this.#renewal = setTimeout(() => {
        void this.#renew();
      }, grantRetry);
    }
  }

  /** Turns the user's microphone on or off and tells the participants; turning it on asks the browser for it. */
  async setMicrophone(on: boolean): Promise<void> {
    const connection = this.#connection;
    if (connection === null || this.#isEnded()) return;
    if (on) {
      try {
        await connection.setMicrophone(true);
      } catch {
        this.#set({ micFailed: true, micOn: false });
        return;
      }
      this.#set({ micOn: true, micFailed: false });
    } else {
      await connection.setMicrophone(false);
      this.#set({ micOn: false });
    }
    try {
      await this.#options.server.setMic(this.#session, on);
    } catch (error) {
      // The server did not take it (the user no longer speaks): the microphone stays off.
      if (on) {
        await connection.setMicrophone(false);
        this.#set({ micOn: false });
      }
      throw error;
    }
  }

  /** Plays the audio that the browser blocked, from the user's gesture. */
  async resumePlayback(): Promise<void> {
    await this.#connection?.resumePlayback();
    this.#set({ playbackBlocked: false });
  }

  #finish(end: VoiceCallEnd): void {
    if (this.#isEnded()) return;
    this.#stop();
    this.#set({ phase: "ended", end, micOn: false, reactions: [] });
    this.#options.server.onEnd();
  }

  #stop(): void {
    this.#stream.abort();
    clearInterval(this.#heartbeat);
    clearTimeout(this.#renewal);
    for (const timer of this.#reactionTimers) clearTimeout(timer);
    this.#reactionTimers.clear();
    this.#connection?.close();
    this.#connection = null;
  }

  /** Leaves the call: closes the audio, the stream, and the timers (the request to leave is the hook's). */
  close(): void {
    if (this.#isEnded()) return;
    this.#stop();
    this.#set({ phase: "ended", end: undefined, micOn: false, reactions: [] });
  }
}
