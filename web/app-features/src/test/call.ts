// SPDX-License-Identifier: AGPL-3.0-only

// What the tests of a call run on in place of the browser's (calls.ts `runCallsOn`): a RoomSession that records what
// the call asked of it and says what a test makes it say, an environment that holds nothing, a clock that moves only
// when the test moves it, and the page's going away and unload held by hand.

import type {
  ConnectionState,
  IncomingSignal,
  RoomEnvironment,
  RoomGrant,
  RoomRoster,
  RoomSession,
  RoomSessionEvents,
  RoomSessionOptions,
  RoomSessionSnapshot,
} from "@my/rtc";
import type { CallRuntime } from "../call/calls";
import type { Clock } from "../call/voice-call";

/** A RoomSession that keeps what it was asked. */
type FakeRoomSession = RoomSession & {
  readonly options: RoomSessionOptions;
  readonly rosters: RoomRoster[];
  readonly renewed: RoomGrant[];
  readonly reconnected: RoomGrant[];
  readonly signals: IncomingSignal[];
  readonly microphones: boolean[];
  readonly closed: () => boolean;
  /** Says what the session's events say. */
  readonly emit: <E extends keyof RoomSessionEvents>(event: E, ...args: Parameters<RoomSessionEvents[E]>) => void;
};

/** Makes a fake RoomSession for `options`; `refuseMicrophone` rejects turning it on with that error. */
function fakeRoomSession(options: RoomSessionOptions, refuseMicrophone?: Error): FakeRoomSession {
  const listeners = new Map<keyof RoomSessionEvents, Set<(...args: never[]) => void>>();
  const rosters: RoomRoster[] = [];
  const renewed: RoomGrant[] = [];
  const reconnected: RoomGrant[] = [];
  const signals: IncomingSignal[] = [];
  const microphones: boolean[] = [];
  let closed = false;
  let state: ConnectionState = "connecting";
  const snapshot = (): RoomSessionSnapshot => ({
    connection: { state, participants: [] },
    talking: new Set(),
    playbackBlocked: false,
    microphone: microphones.at(-1) ?? false,
    quality: null,
    failure: null,
  });
  return {
    kind: "p2p",
    session: options.session,
    capabilities: { outputSelection: false },
    options,
    rosters,
    renewed,
    reconnected,
    signals,
    microphones,
    closed: () => closed,
    update: (roster) => {
      rosters.push(roster);
    },
    setMicrophone: (on) => {
      if (on && refuseMicrophone !== undefined) return Promise.reject(refuseMicrophone);
      microphones.push(on);
      return Promise.resolve();
    },
    setOutputDevice: () => Promise.reject(new Error("fake: no output selection")),
    renew: (grant) => {
      renewed.push(grant);
    },
    reconnect: (grant) => {
      reconnected.push(grant);
    },
    receiveSignal: (signal) => {
      signals.push(signal);
    },
    resumePlayback: () => Promise.resolve(),
    snapshot,
    on: (event, listener) => {
      const set = listeners.get(event) ?? new Set();
      listeners.set(event, set);
      set.add(listener);
      return () => {
        set.delete(listener);
      };
    },
    close: () => {
      closed = true;
      state = "closed";
    },
    emit: (event, ...args) => {
      if (event === "connection") state = (args[0] as { state: ConnectionState }).state;
      for (const listener of listeners.get(event) ?? []) (listener as (...a: unknown[]) => void)(...args);
    },
  };
}

/** A clock that moves only by `advance`, running the timers that come due, in their order. */
type ManualClock = Clock & {
  /** Moves the clock on by `ms`, running each timer as it comes due and letting what it started settle. */
  readonly advance: (ms: number) => Promise<void>;
};

/** Lets what is on its way settle: the promises, and the fake API's answers. */
export async function settle(): Promise<void> {
  for (let i = 0; i < 10; i++) await new Promise((resolve) => setTimeout(resolve, 0));
}

function manualClock(start = Date.now()): ManualClock {
  let now = start;
  let next = 0;
  const timers = new Map<number, { at: number; run: () => void; every?: number }>();
  const add = (run: () => void, ms: number, every?: number) => {
    next += 1;
    timers.set(next, { at: now + Math.max(0, ms), run, ...(every === undefined ? {} : { every }) });
    return next;
  };
  const clear = (timer: unknown) => {
    if (typeof timer === "number") timers.delete(timer);
  };
  return {
    now: () => now,
    setTimeout: (run, ms) => add(run, ms),
    clearTimeout: clear,
    setInterval: (run, ms) => add(run, ms, ms),
    clearInterval: clear,
    advance: async (ms) => {
      const until = now + ms;
      for (;;) {
        await settle();
        let due: [number, { at: number; run: () => void; every?: number }] | undefined;
        for (const entry of timers)
          if (entry[1].at <= until && (due === undefined || entry[1].at < due[1].at)) due = entry;
        if (due === undefined) break;
        const [id, timer] = due;
        now = timer.at;
        if (timer.every === undefined) timers.delete(id);
        else timer.at += timer.every;
        timer.run();
      }
      now = until;
      await settle();
    },
  };
}

/** What a test's runtime of the calls holds: the sessions it made, the page's going away, and the held unloads. */
export type FakeCallRuntime = {
  readonly runtime: Partial<CallRuntime>;
  readonly clock: ManualClock;
  readonly sessions: FakeRoomSession[];
  /** The environments made, each as the reader joined. */
  readonly environments: () => number;
  /** The page goes away. */
  readonly hidePage: () => void;
  /** How many holds of the unload are on now. */
  readonly holds: () => number;
};

/** A runtime of the calls on fakes; `refuseMicrophone` makes every session refuse the microphone with that error. */
export function fakeCallRuntime(refuseMicrophone?: Error): FakeCallRuntime {
  const clock = manualClock();
  const sessions: FakeRoomSession[] = [];
  const hides = new Set<() => void>();
  let holds = 0;
  let environments = 0;
  const environment: RoomEnvironment = {
    createPeerConnection: () => {
      throw new Error("fake: no peer connections");
    },
    getMicrophone: () => Promise.reject(new Error("fake: no microphone")),
    createStream: () => {
      throw new Error("fake: no streams");
    },
    createRemoteAudio: () => {
      throw new Error("fake: no audio");
    },
  };
  return {
    clock,
    sessions,
    environments: () => environments,
    hidePage: () => {
      for (const hide of [...hides]) hide();
    },
    holds: () => holds,
    runtime: {
      clock,
      createSession: (options) => {
        const made = fakeRoomSession(options, refuseMicrophone);
        sessions.push(made);
        return made;
      },
      environment: () => {
        environments += 1;
        return environment;
      },
      watchPageHide: (listener) => {
        hides.add(listener);
        return () => {
          hides.delete(listener);
        };
      },
      holdUnload: () => {
        holds += 1;
        let held = true;
        return () => {
          if (held) holds -= 1;
          held = false;
        };
      },
    },
  };
}
