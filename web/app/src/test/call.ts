// SPDX-License-Identifier: AGPL-3.0-only

// What a route test's call runs on in place of the browser's (`runCallsOn` of @my/app-features), as the feature's own
// fakes do (web/app-features/src/test/call.ts), which the app cannot import: a RoomSession that keeps what it was
// asked, an environment that makes nothing, a clock that moves only when the test moves it, and the page's going away
// and its held unload by hand. The calls run on it until the test ends, and the test ends any call it left on.

import { currentCall, runCallsOn } from "@my/app-features";
import { onTestFinished } from "bun:test";

type Runtime = Required<Parameters<typeof runCallsOn>[0]>;
type Clock = Runtime["clock"];
type RoomSession = ReturnType<Runtime["createSession"]>;
type RoomSessionOptions = Parameters<Runtime["createSession"]>[0];
type Snapshot = ReturnType<RoomSession["snapshot"]>;

/** A RoomSession that keeps what it was asked. */
export type FakeRoomSession = RoomSession & {
  readonly options: RoomSessionOptions;
  /** The grants given to it without connecting again, in their order. */
  readonly renewed: Parameters<RoomSession["renew"]>[0][];
  /** The grants it was asked to connect again with, in their order. */
  readonly reconnected: Parameters<RoomSession["reconnect"]>[0][];
  readonly closed: () => boolean;
};

function fakeRoomSession(options: RoomSessionOptions): FakeRoomSession {
  const renewed: Parameters<RoomSession["renew"]>[0][] = [];
  const reconnected: Parameters<RoomSession["reconnect"]>[0][] = [];
  const microphones: boolean[] = [];
  let closed = false;
  let state: Snapshot["connection"]["state"] = "connecting";
  return {
    kind: "p2p",
    session: options.session,
    capabilities: { outputSelection: false },
    options,
    renewed,
    reconnected,
    closed: () => closed,
    update: () => undefined,
    setMicrophone: (on) => {
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
    receiveSignal: () => undefined,
    resumePlayback: () => Promise.resolve(),
    snapshot: () => ({
      connection: { state, participants: [] },
      talking: new Set(),
      playbackBlocked: false,
      microphone: microphones.at(-1) ?? false,
      quality: null,
      failure: null,
    }),
    on: () => () => undefined,
    close: () => {
      closed = true;
      state = "closed";
    },
  };
}

/** A clock that moves only by `advance`. */
export type ManualClock = Clock & {
  /** Moves the clock on by `ms`, running each timer as it comes due and letting what it started settle. */
  readonly advance: (ms: number) => Promise<void>;
};

/** Lets what is on its way settle: the promises, and the fake API's answers. */
async function settle(): Promise<void> {
  for (let i = 0; i < 10; i++) await new Promise((resolve) => setTimeout(resolve, 0));
}

function manualClock(start = Date.now()): ManualClock {
  type Timer = { at: number; run: () => void; every?: number };
  let now = start;
  let next = 0;
  const timers = new Map<number, Timer>();
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
        let due: [number, Timer] | undefined;
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

/** The calls' runtime of a route test. */
export type FakeCalls = {
  readonly clock: ManualClock;
  /** The RoomSessions made, one a join. */
  readonly sessions: readonly FakeRoomSession[];
  /** The page goes away (`pagehide`). */
  readonly hidePage: () => void;
  /** How many holds of the page's unload are on now. */
  readonly holds: () => number;
};

/**
 * Runs the calls on fakes until the test ends, and then ends the call the test left on, as a page that closes does.
 * Call it before the test joins.
 */
export function callsOnFakes(): FakeCalls {
  const clock = manualClock();
  const sessions: FakeRoomSession[] = [];
  const hides = new Set<() => void>();
  let holds = 0;
  const environment: ReturnType<Runtime["environment"]> = {
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
  const restore = runCallsOn({
    clock,
    createSession: (options) => {
      const made = fakeRoomSession(options);
      sessions.push(made);
      return made;
    },
    environment: () => environment,
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
  });
  onTestFinished(() => {
    currentCall()?.unload();
    restore();
  });
  return {
    clock,
    sessions,
    hidePage: () => {
      for (const hide of [...hides]) hide();
    },
    holds: () => holds,
  };
}
