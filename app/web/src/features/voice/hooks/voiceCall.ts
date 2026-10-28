// SPDX-License-Identifier: AGPL-3.0-only
// The user's session in a room while the call lasts (ADR-0101 §2, §3, §5, §6; ADR-0100 §5, §6): an external system
// that the app-level call (VoiceCallProvider) keeps in sync with React through subscribe / getSnapshot
// (useSyncExternalStore). The call lasts across pages until the user leaves or it ends (#1345 answer 45).
//
// - It sends the session's heartbeat every 15 seconds. A 404 says the session is no longer the user's (it expired
//   while the device was away): it joins again with a new session.
// - It keeps the room stream open with the same session, opening it again after it ends or fails (with a growing
//   wait), and hands its events on: `state` to the Query cache and to the audio, `signal` to the audio, `mic` and
//   `reaction` to the snapshot, `muted` turns the microphone off, `grant` renews or reconnects, and `removed`,
//   `replaced`, and `ended` end the call.
// - It sends the signals of the audio one at a time under the server's rate (20 a second), waiting out a 429 and
//   trying again after a network failure.
// - 10 minutes before the grant expires it asks for a new one and gives it to the audio (setConfiguration and
//   restartIce on every connection, for P2P).
// - The microphone follows the latest wish of the user, one change after another, and is on only while the user
//   speaks in the room as the call knows it; the room that a write answers (speaking, listening) comes in by applyRoom.
// - It keeps who is talking (`talking`, the sessions that the audio hears talking, the user's own among them while the
//   microphone sends), as the audio reports it: the snapshot changes only when that set does, never on each sample of
//   the levels, and the set is empty once the call ends or joins again with a new session.
// - It tells the provider what changed for the user (a role, a mute, a raised hand for the hosts) as notices, which
//   the provider announces.
// The requests and the audio are given to it, so that the tests give fakes.
import { connectVoice, UnsupportedVoiceMethodError } from "../rtc/connect";
import type {
  OutgoingVoiceSignal,
  VoiceConnection,
  VoiceConnectionOptions,
  VoiceEnvironment,
  VoiceGrant,
  VoiceMember,
} from "../rtc/types";
import {
  isSessionGone,
  signalRetryDelay,
  type VoiceGrantData,
  type VoiceSignalData,
  type VoiceStreamEvent,
  type VoiceStreamOptions,
} from "../api/call";
import type { VoiceRoom, VoiceRoomParticipant } from "../api/voice";

/** How often the session sends its heartbeat (ADR-0100 §6). */
export const heartbeatInterval = 15_000;

/** How long before a grant expires the session asks for a new one (ADR-0101 §5). */
export const grantRenewalLead = 10 * 60_000;

/** How long a reaction shows next to its participant. */
export const reactionDuration = 5_000;

/** The least time between two signals: 15 a second, under the server's 20. */
const signalSpacing = 1_000 / 15;

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
  /** Leaves the room without a choice of the next host (a session that the call no longer wants). */
  abandon: () => Promise<void>;
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

/** Why the browser did not give the microphone: the user or the browser refused, there is none, or another app holds it. */
export type VoiceMicError = "denied" | "missing" | "busy" | "other";

export type VoiceCallSnapshot = {
  /** `connecting` until the room stream first opens, `reconnecting` while it is down. */
  phase: "connecting" | "live" | "reconnecting" | "ended";
  end: VoiceCallEnd | undefined;
  /** The room as the call knows it (the latest `state`, or the answer of a write). */
  room: VoiceRoom;
  /** Whether the user's microphone sends. */
  micOn: boolean;
  /** A change of the microphone is under way (the browser may be asking the user). */
  micPending: boolean;
  /** Why the browser did not give the microphone the last time. */
  micError: VoiceMicError | undefined;
  /** The browser did not let the audio play: a button starts it. */
  playbackBlocked: boolean;
  /** The microphones of the participants (account ID → on), as the `mic` events said. */
  mics: ReadonlyMap<string, boolean>;
  reactions: readonly VoiceReactionShown[];
  /**
   * The sessions talking now (a participant's `session`; the user's own while the microphone sends), as the audio
   * hears them: a new set only when who talks changes.
   */
  talking: ReadonlySet<string>;
};

/** A change for the user that the page announces. */
export type VoiceCallNotice =
  | {
      kind:
        | "speaker"
        | "listener"
        | "muted"
        | "hand-down"
        | "cohost"
        | "cohost-removed"
        | "host"
        | "reconnecting"
        | "reconnected";
    }
  | { kind: "hand-raised"; participant: VoiceRoomParticipant };

export type VoiceCallOptions = {
  /** The user's account. */
  accountId: string;
  join: VoiceJoin;
  server: VoiceCallServer;
  environment: VoiceEnvironment;
  onNotice: (notice: VoiceCallNotice) => void;
  /** Connects the audio; connectVoice (by the grant's method) unless a test gives another. */
  connect?: (options: VoiceConnectionOptions) => VoiceConnection;
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

/** Why getUserMedia failed, by the name of its DOMException (Media Capture and Streams §10). */
function micError(error: unknown): VoiceMicError {
  const name = error instanceof DOMException || error instanceof Error ? error.name : "";
  if (name === "NotAllowedError" || name === "SecurityError") return "denied";
  if (name === "NotFoundError" || name === "OverconstrainedError") return "missing";
  if (name === "NotReadableError" || name === "AbortError") return "busy";
  return "other";
}

function wait(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise<void>((resolve) => {
    const timer = setTimeout(resolve, ms);
    signal.addEventListener(
      "abort",
      () => {
        clearTimeout(timer);
        resolve();
      },
      { once: true },
    );
  });
}

/** Nobody talks. */
const noneTalking: ReadonlySet<string> = new Set();

export class VoiceCall {
  readonly #options: VoiceCallOptions;
  #session: string;
  #grant: VoiceGrant;
  #connection: VoiceConnection | null = null;
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
  /** The signals waiting to be sent, one after another. */
  #signals: Promise<void> = Promise.resolve();
  #lastSignalAt = 0;
  /** The microphone the user wants, and the changes that apply it, one after another. */
  #wantMic = false;
  #micChanges: Promise<void> = Promise.resolve();

  constructor(options: VoiceCallOptions) {
    this.#options = options;
    this.#session = options.join.session;
    this.#grant = voiceGrant(options.join.grant);
    this.#snapshot = {
      phase: "connecting",
      end: undefined,
      room: options.join.room,
      micOn: false,
      micPending: false,
      micError: undefined,
      playbackBlocked: false,
      mics: new Map(),
      reactions: [],
      talking: noneTalking,
    };
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

  /** The room of the call. */
  get roomId(): string {
    return this.#snapshot.room.id;
  }

  /** Connects the audio and starts the heartbeats, the room stream, and the renewal of the grant; once. */
  start(): void {
    if (this.#started || this.#isEnded()) return;
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
    const session = this.#session;
    try {
      this.#connection = (this.#options.connect ?? connectVoice)({
        session: this.#session,
        grant: this.#grant,
        environment: this.#options.environment,
        events: {
          sendSignal: (signal) => this.#sendSignal(this.#session, signal),
          onPlaybackBlocked: () => {
            this.#set({ playbackBlocked: true });
          },
          onTalking: (talking) => {
            // The connection of a session that the call left behind (it joined again) no longer says who talks.
            if (session !== this.#session || this.#isEnded()) return;
            this.#set({ talking });
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

  /** Sends a signal after the ones before it, under the server's rate, trying again after a 429 or a network failure. */
  #sendSignal(session: string, { to, kind, sdp, candidate }: OutgoingVoiceSignal): Promise<void> {
    const body: VoiceSignalData = {
      to,
      kind,
      ...(sdp === undefined ? {} : { sdp }),
      ...(candidate === undefined ? {} : { candidate }),
    };
    const send = this.#signals.then(async () => {
      for (let attempt = 1; ; attempt++) {
        if (this.#isStopped() || session !== this.#session) return;
        const spacing = this.#lastSignalAt + signalSpacing - Date.now();
        if (spacing > 0) await wait(spacing, this.#stream.signal);
        this.#lastSignalAt = Date.now();
        try {
          await this.#options.server.sendSignal(session, body);
          return;
        } catch (error) {
          const delay = signalRetryDelay(error, attempt, Date.now());
          if (delay === null) throw error;
          await wait(delay, this.#stream.signal);
        }
      }
    });
    this.#signals = send.catch(() => undefined);
    return send;
  }

  #self(room: VoiceRoom = this.#snapshot.room): VoiceRoomParticipant | undefined {
    return (
      room.participants.find((p) => p.session === this.#session) ??
      room.participants.find((p) => p.account.id === this.#options.accountId)
    );
  }

  /** Whether the user speaks in the room as the call knows it. */
  #speaks(): boolean {
    return this.#self()?.role === "speaker";
  }

  #applyRoster(): void {
    const room = this.#snapshot.room;
    const members: VoiceMember[] = room.participants.flatMap((p) =>
      p.session === undefined || p.session === this.#session
        ? []
        : [{ session: p.session, account: p.account.id, speaker: p.role === "speaker" }],
    );
    this.#connection?.update({ speaker: this.#speaks(), members });
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
    if (this.#rejoining || this.#isStopped()) return;
    this.#rejoining = true;
    try {
      const join = await this.#options.server.join();
      if (this.#isStopped()) {
        // The user left while the request ran: the new session must not stay in the room.
        void this.#options.server.abandon().catch(() => undefined);
        return;
      }
      this.#connection?.close();
      this.#session = join.session;
      this.#grant = voiceGrant(join.grant);
      this.#wantMic = false;
      this.#set({ room: join.room, micOn: false, talking: noneTalking });
      this.#options.server.onRoom(join.room);
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
            const wasDown = this.#phase() === "reconnecting";
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
      const delay = streamDelays[Math.min(Math.max(this.#streamFailures - 1, 0), streamDelays.length - 1)] ?? 30_000;
      await wait(delay, this.#stream.signal);
    }
  }

  #handle(event: VoiceStreamEvent): void {
    switch (event.type) {
      case "state":
        this.applyRoom(event.room);
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
        // The server turned the microphone off for everyone already: nothing to tell it.
        this.#changeMicrophone(false, { tell: false }).catch(() => undefined);
        this.#options.onNotice({ kind: "muted" });
        return;
      case "grant":
        this.#grant = voiceGrant(event.grant.grant);
        if (event.grant.reconnect) this.#connection?.reconnect(this.#grant);
        else this.#connection?.renew(this.#grant, { restartIce: false });
        this.#scheduleRenewal();
        return;
      case "removed":
      case "replaced":
      case "ended":
        this.#finish({ kind: event.type, reason: event.reason });
        return;
    }
  }

  /** Takes the room as the stream or a write (speaking, listening, a raised hand) has it. */
  applyRoom(room: VoiceRoom): void {
    if (this.#isEnded() || room.id !== this.#snapshot.room.id) return;
    const previous = this.#snapshot.room;
    const before = this.#self(previous);
    const wasManager = manages(before);
    this.#set({ room });
    this.#options.server.onRoom(room);
    const after = this.#self(room);
    if (before !== undefined && after !== undefined) {
      if (before.role !== after.role) {
        this.#options.onNotice({ kind: after.role === "speaker" ? "speaker" : "listener" });
        if (after.role !== "speaker") {
          this.#wantMic = false;
          this.#set({ micOn: false });
        }
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
        if (
          participant.hand_raised &&
          !raised.has(participant.account.id) &&
          participant.account.id !== this.#options.accountId
        ) {
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
    if (this.#isStopped()) return;
    // Only the ICE servers of P2P (their TURN credentials) are renewed on this schedule (ADR-0101 §5). An SFU's grant is
    // short-lived and only connects: the SFU's own client keeps its connection, and a new grant comes by a `grant` event.
    if (this.#grant.kind !== "p2p") return;
    // A timer of more than 2^31 - 1 ms fires at once: a grant that far away is looked at again later.
    const delay = Math.min(Math.max(this.#grant.expiresAt - grantRenewalLead - Date.now(), 5_000), longestTimer);
    if (delay === longestTimer) {
      this.#renewal = setTimeout(() => {
        this.#scheduleRenewal();
      }, delay);
      return;
    }
    this.#renewal = setTimeout(() => {
      void this.#renew();
    }, delay);
  }

  async #renew(): Promise<void> {
    const session = this.#session;
    try {
      const grant = await this.#options.server.refreshGrant(session);
      if (session !== this.#session || this.#isStopped()) return;
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

  /**
   * Turns the user's microphone on or off and tells the participants; turning it on asks the browser for it, and works
   * only while the user speaks. Changes apply one after another, so the latest one wins; it rejects when the server
   * does not take it (the microphone is off then).
   */
  setMicrophone(on: boolean): Promise<void> {
    return this.#changeMicrophone(on, { tell: true });
  }

  #changeMicrophone(on: boolean, { tell }: { tell: boolean }): Promise<void> {
    this.#wantMic = on;
    this.#set({ micPending: true });
    const change = this.#micChanges.then(() => this.#applyMicrophone(tell));
    this.#micChanges = change.catch(() => undefined);
    return change;
  }

  async #applyMicrophone(tell: boolean): Promise<void> {
    const connection = this.#connection;
    const want = this.#wantMic && this.#speaks();
    if (connection === null || this.#isEnded()) {
      this.#set({ micPending: false, micOn: false });
      return;
    }
    try {
      await connection.setMicrophone(want);
    } catch (error) {
      this.#wantMic = false;
      this.#set({ micOn: false, micPending: false, micError: micError(error) });
      return;
    }
    const changed = want !== this.#snapshot.micOn;
    this.#set({ micOn: want, micPending: false, ...(want ? { micError: undefined } : {}) });
    if (!tell || !changed) return;
    try {
      await this.#options.server.setMic(this.#session, want);
    } catch (error) {
      // The server did not take it (the user no longer speaks): the microphone stays off.
      if (want) {
        this.#wantMic = false;
        await connection.setMicrophone(false).catch(() => undefined);
        this.#set({ micOn: false });
      }
      throw error;
    }
  }

  /** Plays the audio that the browser blocked, from the user's gesture; the button stays while it stays blocked. */
  async resumePlayback(): Promise<void> {
    try {
      await this.#connection?.resumePlayback();
      this.#set({ playbackBlocked: false });
    } catch {
      this.#set({ playbackBlocked: true });
    }
  }

  #finish(end: VoiceCallEnd): void {
    if (this.#isEnded()) return;
    this.#stop();
    this.#set({ phase: "ended", end, micOn: false, micPending: false, reactions: [], talking: noneTalking });
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

  /** Leaves the call: closes the audio, the stream, and the timers (the request to leave is the provider's); once. */
  close(): void {
    if (this.#isEnded()) return;
    this.#stop();
    this.#set({ phase: "ended", end: undefined, micOn: false, micPending: false, reactions: [], talking: noneTalking });
  }
}
