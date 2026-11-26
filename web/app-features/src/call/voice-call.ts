// SPDX-License-Identifier: AGPL-3.0-only
// One joined session of the reader in a voice room, while the call lasts (ADR-0101 §3, §5, §6, §7; ADR-0100 §5, §6):
// the RoomSession of @my/rtc that carries its audio, the room's stream, the heartbeat and the renewal of the grant,
// and the room as the call knows it. It holds no React and is held by the call controller (calls.ts), never by a
// screen, so that it outlives every one.
//
// - The heartbeat goes every 15 seconds. A heartbeat the server answers with 404 (it dropped the session), or none that
//   went through for 45 seconds (the server's TTL, past which it dropped the session), ends the call as `lost`.
// - The room's stream runs on the same session for the whole call (app-data's runVoiceStream opens it again whenever it
//   drops), and its events go on: `state` to the cache and the audio's roster, `signal` to the audio, `mic` and
//   `reaction` to the state, `muted` turns the microphone off, and `removed`, `replaced` and `ended` end the call. A stream the server refuses (a 404: it holds the session no more) ends it as
//   `lost`.
// - 10 minutes before a P2P grant expires (halfway through a shorter lifetime, counted from the server's time of the
//   answer, and never within 30 seconds of the last) the call asks for a new one and gives it to the audio (`renew`:
//   new TURN credentials and an ICE restart); the same credentials again wait and ask later. An SFU's grant only
//   connects and lives a minute; the SFU keeps the connection. The audio of my's SFU asks the call for a new grant by
//   itself when its node moves the room or its socket drops (`requestGrant`), and while the audio says it is `lost`
//   the call asks for a new grant and connects again with it (`reconnect`), after a wait that grows with the tries, or
//   as the network comes back.
// - The microphone follows the reader's latest wish, one change after another, and sends only while the reader speaks;
//   the server hears of it whenever what applies differs from what it last heard. The microphones of the participants
//   are those of the speakers in the latest `state`.
// - Once the stream gave a `state`, only the stream says how the room stands: the answer of a write may be older.
// - Every end happens once: whatever ends the call first stops the stream, the timers and the audio, and says why.
import {
  endVoiceRoom,
  isVoiceLeaveRefused,
  isVoiceSessionGone,
  leaveVoiceRoom,
  leaveVoiceRoomOnUnload,
  onDataClosed,
  reactInVoiceRoom,
  receiveVoiceRoom,
  renewVoiceGrant,
  runVoiceStream,
  sendVoiceHeartbeat,
  sendVoiceSignal,
  setVoiceMic,
  voiceHostOperation,
  voiceRoomLeft,
  voiceSelfOperation,
  type AppData,
  type ServerTime,
  type VoiceGrant,
  type VoiceHostOperation,
  type VoiceJoin,
  type VoiceLeaveChoice,
  type VoiceRoom,
  type VoiceRoomParticipant,
  type VoiceSelfOperation,
  type VoiceStreamEvent,
} from "@my/app-data";
import type { ConnectionState, RoomEnvironment, RoomGrant, RoomMember, RoomSession, RoomSessionOptions } from "@my/rtc";

/** How often the session sends its heartbeat (ADR-0100 §6). */
export const heartbeatInterval = 15_000;

/** How long the server keeps a session without a heartbeat (ADR-0100 §6): past it, the call is lost. */
export const heartbeatTtl = 45_000;

/** How long before a grant expires the call asks for a new one (ADR-0101 §5). */
export const grantRenewalLead = 10 * 60_000;

/** How long a reaction shows beside its participant. */
export const reactionDuration = 5_000;

/** The wait before asking again for a grant the server could not give. */
const grantRetry = 30_000;

/** The least wait after a grant before it is taken again, whatever its lifetime says. */
const renewalFloor = 30_000;

/** The waits before asking again for a grant for audio that is lost, by the tries before (the last for every later one). */
const regrantDelays: readonly number[] = [1_000, 2_000, 5_000, 10_000, 30_000];

/** The longest wait a timer takes. */
const longestTimer = 2 ** 31 - 1;

/** The timers and the time of a call: the system's, or a test's. */
export type Clock = {
  readonly now: () => number;
  readonly setTimeout: (run: () => void, ms: number) => unknown;
  readonly clearTimeout: (timer: unknown) => void;
  readonly setInterval: (run: () => void, ms: number) => unknown;
  readonly clearInterval: (timer: unknown) => void;
};

/** The system's clock. */
export const systemClock: Clock = {
  now: () => Date.now(),
  setTimeout: (run, ms) => setTimeout(run, ms),
  clearTimeout: (timer) => {
    clearTimeout(timer as ReturnType<typeof setTimeout>);
  },
  setInterval: (run, ms) => setInterval(run, ms),
  clearInterval: (timer) => {
    clearInterval(timer as ReturnType<typeof setInterval>);
  },
};

/**
 * Why a call ended: the reader left (`left`); the stream said the session was `removed` (its reason: `left`, `moved`,
 * `removed`, `blocked`…), `replaced` by another device, or the room `ended` (its reason: `host`, `empty`…); the server
 * dropped the session (`lost`: `heartbeat` when none went through for 45 seconds, `session` when it answered 404,
 * `stream` when it refused the stream); the reader joined another room (`switched`); the session of the app ended
 * (`signed_out`); the page went away (`unloaded`); or the audio could not go on (`failed`, with the audio's reason).
 */
export type CallEnd = {
  readonly kind: "left" | "removed" | "replaced" | "ended" | "lost" | "switched" | "signed_out" | "unloaded" | "failed";
  readonly reason: string;
};

/** `connecting` until the room's stream first opens, `live` while it is open, `reconnecting` while it is down. */
export type CallPhase = "connecting" | "live" | "reconnecting" | "ended";

/**
 * Why the microphone is not on: the browser or the reader refused it, there is none, another app holds it, or the
 * server did not take it (the reader does not speak).
 */
export type MicFailure = "denied" | "missing" | "busy" | "refused" | "other";

/** A reaction that shows a while beside its participant (an account ID). */
export type ShownReaction = { readonly id: number; readonly participant: string; readonly emoji: string };

/** The call as it stands; a new value whenever something changed. */
export type VoiceCallState = {
  readonly roomId: string;
  readonly session: string;
  readonly phase: CallPhase;
  /** Why it ended, once it did. */
  readonly end: CallEnd | null;
  /** The room as the call knows it: the latest `state`, or the answer of a write. */
  readonly room: VoiceRoom;
  /** The reader's own participant, once the room names it. */
  readonly self: VoiceRoomParticipant | undefined;
  /** Whether the reader's microphone sends. */
  readonly microphone: boolean;
  /** Whether the reader wants it on: their latest wish, until the browser, the server or the room turns it off. */
  readonly micWanted: boolean;
  /** A change of the microphone is under way (the browser may be asking the reader). */
  readonly micPending: boolean;
  readonly micFailure: MicFailure | null;
  /** The microphones of the participants (account ID → on), as the `mic` events said. */
  readonly mics: ReadonlyMap<string, boolean>;
  readonly reactions: readonly ShownReaction[];
  /** The sessions talking now, the reader's own among them while the microphone sends. */
  readonly talking: ReadonlySet<string>;
  /** The browser did not let the audio play: a gesture resumes it (`resumePlayback`). */
  readonly playbackBlocked: boolean;
  /** How the audio's connection stands. */
  readonly connection: ConnectionState;
  /** The account ID of the host or co-host who muted the reader last, until the microphone is turned on again. */
  readonly mutedBy: string | null;
};

/** What a call runs on: how its audio is made (the web's RoomSession of @my/rtc, a native one later) and its clock. */
export type VoiceCallRuntime = {
  readonly createSession: (options: RoomSessionOptions) => RoomSession;
  readonly clock: Clock;
  /** Calls `onOnline` when the network comes back, until the function it returns is called; none by default. */
  readonly watchNetwork?: ((onOnline: () => void) => () => void) | undefined;
};

/** The grant as @my/rtc takes it. */
export function roomGrant(grant: VoiceGrant): RoomGrant {
  return {
    kind: grant.kind,
    url: grant.url,
    token: grant.token,
    iceServers: grant.ice_servers,
    relayOnly: grant.relay_only,
    expiresAt: Date.parse(grant.expires_at),
  };
}

/** Why the browser did not give the microphone, by the name of its DOMException. */
function micFailure(error: unknown): MicFailure {
  const name = error instanceof Error ? error.name : "";
  if (name === "NotAllowedError" || name === "SecurityError") return "denied";
  if (name === "NotFoundError" || name === "OverconstrainedError") return "missing";
  if (name === "NotReadableError" || name === "AbortError") return "busy";
  return "other";
}

const nobody: ReadonlySet<string> = new Set();

/** One joined session of the reader in a room, from its join to its end. */
export class VoiceCall {
  readonly #data: AppData;
  readonly #runtime: VoiceCallRuntime;
  readonly #audio: RoomSession;
  readonly #stream = new AbortController();
  readonly #listeners = new Set<() => void>();
  readonly #ends = new Set<(end: CallEnd) => void>();
  readonly #unhook: (() => void)[] = [];
  readonly #reactionTimers = new Set<unknown>();
  #state: VoiceCallState;
  #grant: RoomGrant;
  #accountId: string | undefined;
  #members: readonly RoomMember[] = [];
  #heartbeat: unknown;
  #beating = false;
  #alive: number;
  #renewal: unknown;
  /** When the grant came, by the call's clock, and how long it lives from then (NaN when its expiry is unreadable). */
  #grantAt = 0;
  #grantLife = Number.NaN;
  #regranting = false;
  #regrantTimer: unknown;
  /** The tries for a new grant since the audio was last connected. */
  #regrants = 0;
  /** Whether the server last heard that the microphone is on. */
  #micHeard = false;
  /** Whether a `state` of the stream came: from then on only the stream says how the room stands. */
  #heardState = false;
  /** What `#leaving` was before a join of another room took its place. */
  #beforeSwitch: CallEnd | null = null;
  #reactionId = 0;
  #micChanges: Promise<void> = Promise.resolve();
  #started = false;
  /**
   * Why the call is ending while the reader's own request is on its way (leaving, or joining another room): the
   * server's word that the session went (the stream's `removed`, a 404) is that request's doing, and ends it so.
   */
  #leaving: CallEnd | null = null;

  /**
   * Takes the session `join` answered, on `data`'s session of the app, and makes its audio on `environment` (made
   * from the reader's gesture). It throws what making the audio throws (a grant of a kind it cannot connect with).
   */
  constructor(
    data: AppData,
    join: VoiceJoin & Partial<ServerTime>,
    environment: RoomEnvironment,
    runtime: VoiceCallRuntime,
  ) {
    this.#data = data;
    this.#runtime = runtime;
    this.#grant = roomGrant(join.grant);
    this.#alive = runtime.clock.now();
    this.#grantTimes(join.grant, join.serverTime);
    this.#state = {
      roomId: join.room.id,
      session: join.session,
      phase: "connecting",
      end: null,
      room: join.room,
      self: undefined,
      microphone: false,
      micWanted: false,
      micPending: false,
      micFailure: null,
      mics: new Map(),
      reactions: [],
      talking: nobody,
      playbackBlocked: false,
      connection: "connecting",
      mutedBy: null,
    };
    this.#learnSelf(join.room);
    this.#audio = runtime.createSession({
      grant: this.#grant,
      session: join.session,
      environment,
      requestGrant: () => this.#takeGrant(),
      signaling: {
        send: async ({ to, kind, sdp, candidate }) => {
          if (this.#over()) return;
          await sendVoiceSignal(data, this.roomId, this.session, {
            to,
            kind,
            ...(sdp === undefined ? {} : { sdp }),
            ...(candidate === undefined ? {} : { candidate }),
          });
        },
      },
    });
  }

  get roomId(): string {
    return this.#state.roomId;
  }

  get session(): string {
    return this.#state.session;
  }

  /** Whether the call ended. */
  get ended(): boolean {
    return this.#over();
  }

  /** Whether the call ended; a method, read anew after each await. */
  #over(): boolean {
    return this.#state.end !== null;
  }

  /** The RoomSession that carries the call's audio, the same for the whole call. */
  get audio(): RoomSession {
    return this.#audio;
  }

  /** The signal of the room's stream, aborted once the call ends: the same for the whole call. */
  get stream(): AbortSignal {
    return this.#stream.signal;
  }

  /** The timer of the heartbeat, the same for the whole call; undefined before `start` and after the end. */
  get heartbeat(): unknown {
    return this.#heartbeat;
  }

  subscribe = (listener: () => void): (() => void) => {
    this.#listeners.add(listener);
    return () => {
      this.#listeners.delete(listener);
    };
  };

  getState = (): VoiceCallState => this.#state;

  /** Calls `listener` once the call ends, with why; at once for a call that ended. */
  onEnd(listener: (end: CallEnd) => void): () => void {
    const { end } = this.#state;
    if (end !== null) {
      listener(end);
      return () => undefined;
    }
    this.#ends.add(listener);
    return () => {
      this.#ends.delete(listener);
    };
  }

  #set(change: Partial<VoiceCallState>): void {
    this.#state = { ...this.#state, ...change };
    for (const listener of [...this.#listeners]) listener();
  }

  /** Starts the audio's events, the room's stream, the heartbeat and the renewal of the grant; once. */
  start(): void {
    if (this.#started || this.#over()) return;
    this.#started = true;
    // The session of the app ended (signed out here or in another tab): nothing can be asked of the server any more.
    // Heard first, so that a session that ended already ends the call before anything else is started.
    this.#unhook.push(
      onDataClosed(this.#data, () => {
        this.#finish({ kind: "signed_out", reason: "" }, false);
      }),
    );
    if (this.#over()) return;
    const audio = this.#audio;
    this.#unhook.push(
      audio.on("connection", ({ state }) => {
        this.#set({ connection: state });
        if (state === "lost") this.#regrantSoon();
        else if (state === "connected") this.#regrants = 0;
      }),
      audio.on("talking", (talking) => {
        if (!this.#over()) this.#set({ talking });
      }),
      audio.on("playbackBlocked", (playbackBlocked) => {
        this.#set({ playbackBlocked });
      }),
      audio.on("failed", (failure) => {
        this.#finish({ kind: "failed", reason: failure.reason }, true);
      }),
      this.#runtime.watchNetwork?.(() => {
        // The network came back: audio that is lost asks for its grant now, not after the wait.
        if (this.#state.connection !== "lost" || this.#regranting || this.#over()) return;
        this.#runtime.clock.clearTimeout(this.#regrantTimer);
        this.#regrantTimer = undefined;
        void this.#regrant();
      }) ?? (() => undefined),
    );
    if (this.#over()) return;
    const { clock } = this.#runtime;
    this.#heartbeat = clock.setInterval(() => {
      void this.#beat();
    }, heartbeatInterval);
    this.#scheduleRenewal();
    this.#applyRoster();
    void this.#readStream();
  }

  // ---- The heartbeat ---------------------------------------------------------------------------------------------

  async #beat(): Promise<void> {
    if (this.#over()) return;
    const { clock } = this.#runtime;
    if (this.#beating) {
      // The heartbeat before has not come back: past the server's TTL the session is gone.
      if (clock.now() - this.#alive >= heartbeatTtl) this.#finish({ kind: "lost", reason: "heartbeat" }, false);
      return;
    }
    this.#beating = true;
    try {
      await sendVoiceHeartbeat(this.#data, this.roomId, this.session);
      this.#alive = clock.now();
    } catch (error) {
      if (isVoiceSessionGone(error)) this.#finish({ kind: "lost", reason: "session" }, false);
      else if (clock.now() - this.#alive >= heartbeatTtl) this.#finish({ kind: "lost", reason: "heartbeat" }, false);
      // Any other failure (offline, a server that went away) is tried again with the next heartbeat.
    } finally {
      this.#beating = false;
    }
  }

  // ---- The room's stream -----------------------------------------------------------------------------------------

  async #readStream(): Promise<void> {
    const { clock } = this.#runtime;
    try {
      await runVoiceStream(this.#data, {
        room: this.roomId,
        session: this.session,
        signal: this.#stream.signal,
        onOpen: () => {
          this.#set({ phase: "live" });
        },
        onDrop: () => {
          if (!this.#over()) this.#set({ phase: this.#state.phase === "connecting" ? "connecting" : "reconnecting" });
        },
        onEvent: (event) => {
          this.#handle(event);
        },
        sleep: (ms, signal) =>
          new Promise<void>((resolve) => {
            const timer = clock.setTimeout(done, ms);
            signal.addEventListener("abort", done, { once: true });
            function done() {
              clock.clearTimeout(timer);
              signal.removeEventListener("abort", done);
              resolve();
            }
          }),
      });
    } catch (error) {
      this.#finish({ kind: "lost", reason: isVoiceSessionGone(error) ? "session" : "stream" }, false);
    }
  }

  #handle(event: VoiceStreamEvent): void {
    if (this.#over()) return;
    switch (event.type) {
      case "state": {
        this.#heardState = true;
        this.#learnSelf(event.room);
        // A microphone is said only of a speaker who is still in the room.
        const speakers = new Set(event.room.participants.filter((p) => p.role === "speaker").map((p) => p.account.id));
        const mics = [...this.#state.mics].filter(([id]) => speakers.has(id));
        if (mics.length !== this.#state.mics.size) this.#set({ mics: new Map(mics) });
        this.#members = event.room.participants.flatMap((p) =>
          p.session === undefined || p.session === this.session
            ? []
            : [{ session: p.session, speaker: p.role === "speaker", participant: p.account.id }],
        );
        this.#applyRoom(event.room);
        return;
      }
      case "mic":
        this.#set({ mics: new Map([...this.#state.mics, [event.participant, event.on]]) });
        return;
      case "reaction":
        this.#react(event.participant, event.emoji);
        return;
      case "signal": {
        const { from, kind, sdp, candidate } = event.signal;
        this.#audio.receiveSignal({ from, kind, sdp, candidate });
        return;
      }
      case "muted":
        // The server turned the microphone off for everyone already: nothing to tell it.
        this.#micHeard = false;
        this.#set({ mutedBy: event.by });
        this.#changeMicrophone(false).catch(() => undefined);
        return;
      case "removed":
      case "replaced":
      case "ended":
        this.#finish({ kind: event.type, reason: event.reason }, false);
        return;
    }
  }

  /** Learns the reader's account from the room's participant of this session (only the stream names sessions). */
  #learnSelf(room: VoiceRoom): void {
    const own = room.participants.find((p) => p.session !== undefined && p.session === this.session);
    if (own !== undefined) this.#accountId = own.account.id;
  }

  #selfIn(room: VoiceRoom): VoiceRoomParticipant | undefined {
    const id = this.#accountId;
    return id === undefined ? undefined : room.participants.find((p) => p.account.id === id);
  }

  /** Takes the room as the stream or a write has it: into the cache, the state and the audio's roster. */
  #applyRoom(room: VoiceRoom): void {
    if (this.#over() || room.id !== this.roomId) return;
    const self = this.#selfIn(room);
    const spoke = this.#state.self?.role === "speaker";
    this.#set({ room, self });
    receiveVoiceRoom(this.#data, room);
    this.#applyRoster();
    // No longer a speaker: the microphone goes off.
    if (spoke && self?.role !== "speaker" && this.#state.micWanted)
      this.#changeMicrophone(false).catch(() => undefined);
  }

  #speaks(): boolean {
    return this.#state.self?.role === "speaker";
  }

  #applyRoster(): void {
    if (!this.#over()) this.#audio.update({ speaker: this.#speaks(), members: this.#members });
  }

  #react(participant: string, emoji: string): void {
    const id = ++this.#reactionId;
    const { clock } = this.#runtime;
    this.#set({ reactions: [...this.#state.reactions, { id, participant, emoji }] });
    const timer = clock.setTimeout(() => {
      this.#reactionTimers.delete(timer);
      this.#set({ reactions: this.#state.reactions.filter((r) => r.id !== id) });
    }, reactionDuration);
    this.#reactionTimers.add(timer);
  }

  // ---- The grant -------------------------------------------------------------------------------------------------

  /** Keeps when `grant` came and how long it lives, from the server's time of the answer when it gave one. */
  #grantTimes(grant: VoiceGrant, serverTime: number | null | undefined): void {
    const now = this.#runtime.clock.now();
    this.#grantAt = now;
    this.#grantLife = Date.parse(grant.expires_at) - (serverTime ?? now);
  }

  /**
   * Takes a P2P grant again before it expires: 10 minutes before, or halfway through a lifetime shorter than 20
   * minutes, and never sooner than 30 seconds after it came. A grant whose expiry cannot be read is not taken again.
   */
  #scheduleRenewal(): void {
    const { clock } = this.#runtime;
    clock.clearTimeout(this.#renewal);
    if (this.#over() || this.#grant.kind !== "p2p" || !Number.isFinite(this.#grantLife)) return;
    const after = Math.max(this.#grantLife - grantRenewalLead, this.#grantLife / 2, renewalFloor);
    const delay = Math.min(Math.max(this.#grantAt + after - clock.now(), 0), longestTimer);
    this.#renewal = clock.setTimeout(() => {
      if (delay === longestTimer) this.#scheduleRenewal();
      else void this.#renew();
    }, delay);
  }

  #retryRenewal(): void {
    const { clock } = this.#runtime;
    clock.clearTimeout(this.#renewal);
    this.#renewal = clock.setTimeout(() => {
      void this.#renew();
    }, grantRetry);
  }

  async #renew(): Promise<void> {
    if (this.#over()) return;
    try {
      const { grant, serverTime } = await renewVoiceGrant(this.#data, this.roomId, this.session);
      if (this.#over()) return;
      const renewed = roomGrant(grant);
      // The same credentials again (a TURN provider gives them until shortly before they expire), or none readable.
      if (!(renewed.expiresAt > this.#grant.expiresAt)) {
        this.#retryRenewal();
        return;
      }
      this.#grant = renewed;
      this.#grantTimes(grant, serverTime);
      this.#audio.renew(renewed);
      this.#scheduleRenewal();
    } catch (error) {
      if (this.#over()) return;
      if (isVoiceSessionGone(error)) {
        this.#finish({ kind: "lost", reason: "session" }, false);
        return;
      }
      this.#retryRenewal();
    }
  }

  /**
   * A new grant for audio that asks for one by itself (my's SFU, whose node moved the room or whose socket dropped),
   * kept as the call's grant; a session the server dropped ends the call as `lost`.
   */
  async #takeGrant(): Promise<RoomGrant> {
    if (this.#over()) throw new Error("app-features: the call ended");
    try {
      const { grant, serverTime } = await renewVoiceGrant(this.#data, this.roomId, this.session);
      if (this.#over()) throw new Error("app-features: the call ended");
      this.#grant = roomGrant(grant);
      this.#grantTimes(grant, serverTime);
      this.#scheduleRenewal();
      return this.#grant;
    } catch (error) {
      if (isVoiceSessionGone(error)) this.#finish({ kind: "lost", reason: "session" }, false);
      throw error;
    }
  }

  /** The audio is lost: asks for a new grant, at once the first time, then after a wait that grows with the tries. */
  #regrantSoon(): void {
    if (this.#over() || this.#regranting || this.#regrantTimer !== undefined) return;
    const delay =
      this.#regrants === 0 ? 0 : (regrantDelays[Math.min(this.#regrants - 1, regrantDelays.length - 1)] ?? 30_000);
    this.#regrants++;
    this.#regrantTimer = this.#runtime.clock.setTimeout(() => {
      this.#regrantTimer = undefined;
      void this.#regrant();
    }, delay);
  }

  /** The audio went away and needs a new grant: asks for one and connects again with it, until it is connected. */
  async #regrant(): Promise<void> {
    if (this.#regranting || this.#over()) return;
    this.#regranting = true;
    try {
      const { grant, serverTime } = await renewVoiceGrant(this.#data, this.roomId, this.session);
      if (this.#over()) return;
      this.#grant = roomGrant(grant);
      this.#grantTimes(grant, serverTime);
      this.#audio.reconnect(this.#grant);
      this.#scheduleRenewal();
    } catch (error) {
      if (isVoiceSessionGone(error)) this.#finish({ kind: "lost", reason: "session" }, false);
    } finally {
      this.#regranting = false;
    }
    // Still lost (the request failed, or the new connection went away too): try again after the wait.
    if (this.#state.connection === "lost") this.#regrantSoon();
  }

  // ---- What the reader does --------------------------------------------------------------------------------------

  /**
   * Turns the reader's microphone on or off and tells the participants; on asks the browser for it, and sends only
   * while the reader speaks. Changes apply one after another, the latest last; it rejects when the browser or the
   * server did not take it (the microphone is off then, and `micFailure` says why).
   */
  setMicrophone(on: boolean): Promise<void> {
    return this.#changeMicrophone(on);
  }

  #changeMicrophone(on: boolean): Promise<void> {
    this.#set({ micWanted: on, micPending: true, ...(on ? { mutedBy: null } : {}) });
    const change = this.#micChanges.then(() => this.#applyMicrophone());
    this.#micChanges = change.catch(() => undefined);
    return change;
  }

  /** Applies the latest wish, and tells the server whenever what applies differs from what it last heard. */
  async #applyMicrophone(): Promise<void> {
    if (this.#over()) return;
    const asked = this.#state.micWanted;
    const want = asked && this.#speaks();
    if (asked && !want) {
      this.#set({ micWanted: false, micPending: false, microphone: false, micFailure: "refused" });
      throw new Error("app-features: the reader does not speak in the room");
    }
    try {
      await this.#audio.setMicrophone(want);
    } catch (error) {
      this.#set({ micWanted: false, micPending: false, microphone: false, micFailure: micFailure(error) });
      throw error;
    }
    this.#set({ micPending: false, microphone: want, ...(want ? { micFailure: null } : {}) });
    if (want === this.#micHeard || this.#over()) return;
    try {
      await setVoiceMic(this.#data, this.roomId, this.session, want);
      this.#micHeard = want;
    } catch (error) {
      if (want) {
        // The server did not take it (the reader no longer speaks): the microphone goes off again.
        await this.#audio.setMicrophone(false).catch(() => undefined);
        this.#set({ microphone: false, micWanted: false, micFailure: "refused" });
      }
      throw error;
    }
  }

  /** Asks to speak, listens again, raises or lowers the hand; the room the server answers comes in. */
  async act(operation: VoiceSelfOperation): Promise<void> {
    const room = await voiceSelfOperation(this.#data, this.roomId, operation);
    // Once the stream says how the room stands, an answer that may be older than its latest `state` changes nothing.
    if (!this.#heardState) this.#applyRoom(room);
  }

  /** Sends a reaction (a Unicode emoji or a custom emoji's `:shortcode:`). */
  async react(emoji: string): Promise<void> {
    await reactInVoiceRoom(this.#data, this.roomId, emoji);
  }

  /** Runs the host's or a co-host's `operation` on the participant `accountId`. */
  async host(accountId: string, operation: VoiceHostOperation): Promise<void> {
    const room = await voiceHostOperation(this.#data, this.roomId, accountId, operation);
    if (room !== undefined && !this.#heardState) this.#applyRoom(room);
  }

  /** Plays the audio the browser blocked, from the reader's gesture. */
  async resumePlayback(): Promise<void> {
    await this.#audio.resumePlayback();
  }

  /**
   * Leaves the room, the host of a room with others in it naming the next host or ending it. When the server refuses
   * (a 409 for a host who did not choose, a 422 for a next host who may not take over) it rejects and the call goes
   * on; otherwise the call ends, also when the server could not be asked (it drops the session after its TTL).
   */
  async leave(choice?: VoiceLeaveChoice): Promise<void> {
    if (this.#over()) return;
    const left: CallEnd = { kind: "left", reason: "" };
    this.#leaving = left;
    try {
      await leaveVoiceRoom(this.#data, this.roomId, this.session, choice);
    } catch (error) {
      if (isVoiceLeaveRefused(error) && !this.#over()) {
        this.#leaving = null;
        throw error;
      }
    }
    this.#finish(left, false);
  }

  /**
   * Ends the room the reader hosts, for everyone in it, and with it the call, which ends as the reader's leaving. When
   * the server refuses (a reader who no longer hosts it) it rejects and the call goes on.
   */
  async endRoom(): Promise<void> {
    if (this.#over()) return;
    const left: CallEnd = { kind: "left", reason: "" };
    this.#leaving = left;
    try {
      await endVoiceRoom(this.#data, this.roomId);
    } catch (error) {
      if (!this.#over()) {
        this.#leaving = null;
        throw error;
      }
    }
    this.#finish(left, false);
  }

  /** The page goes away: the leave goes with it, outliving the page, and the call ends. */
  unload(): void {
    if (this.#over()) return;
    leaveVoiceRoomOnUnload(this.#data, this.roomId, this.session);
    this.#finish({ kind: "unloaded", reason: "" }, false);
  }

  /**
   * The reader asks to join another room, which leaves this one: what the server says of this session meanwhile is
   * the join's doing. `false` once the server refused the join, and the call goes on.
   */
  switching(on: boolean): void {
    if (this.#over()) return;
    if (on) {
      this.#beforeSwitch = this.#leaving;
      this.#leaving = { kind: "switched", reason: "" };
    } else {
      this.#leaving = this.#beforeSwitch;
      this.#beforeSwitch = null;
    }
  }

  /** Ends the call without asking the server: the reader's join of another room left this one already. */
  switched(): void {
    this.#finish({ kind: "switched", reason: "" }, false);
  }

  /** Ends the call once: stops the stream, the timers and the audio, and says why; `leave` also leaves the room. */
  #finish(said: CallEnd, leave: boolean): void {
    if (this.#over()) return;
    const end =
      this.#leaving !== null && ["removed", "replaced", "ended", "lost"].includes(said.kind) ? this.#leaving : said;
    const { clock } = this.#runtime;
    this.#stream.abort();
    clock.clearInterval(this.#heartbeat);
    this.#heartbeat = undefined;
    clock.clearTimeout(this.#renewal);
    clock.clearTimeout(this.#regrantTimer);
    this.#regrantTimer = undefined;
    for (const timer of this.#reactionTimers) clock.clearTimeout(timer);
    this.#reactionTimers.clear();
    for (const unhook of this.#unhook.splice(0)) unhook();
    this.#audio.close();
    this.#set({
      phase: "ended",
      end,
      microphone: false,
      micWanted: false,
      micPending: false,
      reactions: [],
      talking: nobody,
      connection: "closed",
    });
    if (end.kind !== "signed_out") {
      if (leave) void leaveVoiceRoom(this.#data, this.roomId, this.session).catch(() => undefined);
      voiceRoomLeft(this.#data, this.roomId);
    }
    for (const listener of [...this.#ends]) listener(end);
    this.#ends.clear();
  }
}
