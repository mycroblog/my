// SPDX-License-Identifier: AGPL-3.0-only

// The reader's call in a screen's story (app.samples.tsx): a real call of the feature (calls.ts) on the fake server,
// whose sound is a session that connects to nobody and plays nothing, on a clock that never moves, so that what the
// story shows stays as it was put (a reaction does not fade, no heartbeat goes). The story joins a room of the fake as
// the app's join does, then puts the call in its state: the reader a speaker or with a hand raised, muted by a host,
// with reactions, with people talking, the sound held back, the microphone refused, the connection lost, or the call
// ended by the server (removed, replaced, the room ended). A host's story opens a room of their own as the form does,
// or joins the room the fake has them host, and puts it in its state by the host's own operations: a raised hand let
// speak, a co-host named, an invitation sent, the room handed over; and the room's maximum lowered below the people in
// it, by the reader who hosts it or by its host.

import { inviteToVoiceRoom, updateVoiceRoom, type AppData } from "@my/app-data";
import { currentCall, hostsOthers, joinRoom, openRoom, runCallsOn, type CallRuntime } from "@my/app-features";
import type { FakeApi } from "@my/e2e/fake";

type RoomSession = ReturnType<CallRuntime["createSession"]>;
type Listener = (...args: never[]) => void;

/** What a story's call is put in once it joined. */
type StoryCallStep =
  | "speaker"
  | "hand"
  | "muted"
  | "reactions"
  | "talking"
  | "held"
  | "mic-refused"
  | "reconnecting"
  | "removed"
  | "replaced"
  | "ended"
  | "granted"
  | "cohost"
  | "invited"
  | "handed-over"
  | "lowered";

/**
 * The room a story's reader joined (the fake's room at `room` of its list, or `opened`: one they opened, alone in
 * it), and what the call went through.
 */
export type StoryCall = { readonly room: number | "opened"; readonly steps?: readonly StoryCallStep[] };

/** A session of the sound that connects to nobody, and says what the story makes it say. */
type StorySession = RoomSession & {
  readonly emit: (event: string, value: unknown) => void;
  refuseMicrophone: boolean;
};

function storySession(session: string): StorySession {
  const listeners = new Map<string, Set<Listener>>();
  let microphone = false;
  const made: StorySession = {
    kind: "p2p",
    session,
    capabilities: { outputSelection: false },
    refuseMicrophone: false,
    update: () => undefined,
    setMicrophone: (on) => {
      if (on && made.refuseMicrophone) return Promise.reject(new DOMException("story", "NotAllowedError"));
      microphone = on;
      return Promise.resolve();
    },
    setOutputDevice: () => Promise.reject(new Error("story: no output selection")),
    renew: () => undefined,
    reconnect: () => undefined,
    receiveSignal: () => undefined,
    resumePlayback: () => Promise.resolve(),
    snapshot: () => ({
      connection: { state: "connected", participants: [] },
      talking: new Set(),
      playbackBlocked: false,
      microphone,
      quality: null,
      failure: null,
    }),
    on: (event, listener) => {
      const set = listeners.get(event) ?? new Set();
      listeners.set(event, set);
      set.add(listener);
      return () => {
        set.delete(listener);
      };
    },
    close: () => undefined,
    emit: (event, value) => {
      for (const listener of listeners.get(event) ?? []) (listener as (v: unknown) => void)(value);
    },
  };
  return made;
}

/** A clock that never moves: no timer comes due. */
const stillClock: CallRuntime["clock"] = {
  now: () => Date.now(),
  setTimeout: () => 0,
  clearTimeout: () => undefined,
  setInterval: () => 0,
  clearInterval: () => undefined,
};

const notInStory = () => {
  throw new Error("story: the call's sound makes nothing");
};

/** Lets what is on its way settle: the fake's answers and the stream's first state. */
const settle = () =>
  new Promise<void>((resolve) => {
    setTimeout(resolve, 60);
  });

/**
 * Joins the story's reader to the room of `call` on `fake` and puts the call in its steps; returns what ends it and
 * gives the calls their runtime back, for the story's unmount.
 */
export function joinStoryCall(data: AppData, fake: FakeApi, call: StoryCall): () => void {
  let session: StorySession | undefined;
  const restore = runCallsOn({
    clock: stillClock,
    createSession: (options) => (session = storySession(options.session)),
    environment: () => ({
      createPeerConnection: notInStory,
      getMicrophone: notInStory,
      createStream: notInStory,
      createRemoteAudio: notInStory,
    }),
    watchPageHide: () => () => undefined,
    holdUnload: () => () => undefined,
  });
  let stopped = false;
  const steps = call.steps ?? [];
  const run = async () => {
    const joined =
      call.room === "opened"
        ? await openRoom(data, { name: "Evening sketch session", icon: "🎨", hashtags: ["sketch"], speakers: "manual" })
        : await joinRoom(data, fake.voice.roomId(call.room));
    await settle();
    session?.emit("connection", { state: "connected", participants: [] });
    for (const step of steps) {
      if (stopped) return;
      const room = joined.getState().room;
      const stage = room.participants.filter((p) => p.role === "speaker" && p.session !== undefined);
      // The participants a host's step acts on: the first raised hand, and the first listener without one.
      const raised = room.participants.find((p) => p.hand_raised);
      const listener = room.participants.find((p) => p.role === "listener" && !p.hand_raised);
      switch (step) {
        case "speaker":
          await joined.act("speak");
          await joined.setMicrophone(true);
          break;
        case "hand":
          await joined.act("raise-hand");
          break;
        case "muted":
          fake.voice.send({ event: "muted", data: { by: room.host.id } });
          break;
        case "reactions":
          for (const [i, emoji] of ["👏", "❤️", "😂", "🔥", "👏"].entries()) {
            const from = room.participants[i % room.participants.length];
            if (from !== undefined)
              fake.voice.send({ event: "reaction", data: { participant: from.account.id, emoji } });
          }
          break;
        case "talking":
          for (const p of stage.slice(0, 2))
            fake.voice.send({ event: "mic", data: { participant: p.account.id, on: true } });
          session?.emit(
            "talking",
            new Set(stage.slice(0, 1).flatMap((p) => (p.session === undefined ? [] : [p.session]))),
          );
          break;
        case "held":
          session?.emit("playbackBlocked", true);
          break;
        case "mic-refused":
          if (session !== undefined) session.refuseMicrophone = true;
          await joined.setMicrophone(true).catch(() => undefined);
          break;
        case "reconnecting":
          session?.emit("connection", { state: "reconnecting", participants: [] });
          break;
        case "removed":
          fake.voice.send({ event: "removed", data: { reason: "removed" } });
          break;
        case "replaced":
          fake.voice.send({ event: "replaced", data: { reason: "replaced" } });
          break;
        case "ended":
          fake.voice.send({ event: "ended", data: { reason: "host" } });
          break;
        case "granted":
          if (raised !== undefined) await joined.host(raised.account.id, "grant");
          break;
        case "cohost":
          if (listener !== undefined) await joined.host(listener.account.id, "cohost");
          break;
        case "invited":
          await inviteToVoiceRoom(data, joined.roomId, fake.accountId("sam"), "speaker");
          await inviteToVoiceRoom(data, joined.roomId, fake.accountId("layla"), "listener");
          break;
        case "handed-over": {
          const next = stage.find((p) => !p.hosting);
          if (next !== undefined) await joined.host(next.account.id, "hand-over");
          break;
        }
        case "lowered": {
          // The room's maximum lowered to two below the people in it: by the reader where they host it, else by its
          // host; nobody leaves.
          const max = Math.max(2, room.participants_count - 2);
          if (joined.getState().self?.hosting === true)
            await updateVoiceRoom(data, joined.roomId, { max_participants: max });
          else fake.voice.hostSetsMax(max);
          break;
        }
      }
      await settle();
    }
  };
  void run().catch(() => undefined);
  return () => {
    stopped = true;
    // A host with others in the room ends it: the next story starts with no call.
    const left = currentCall();
    const state = left?.getState();
    void left
      ?.leave(state !== undefined && hostsOthers(state.room, state.self) ? { end: true } : undefined)
      .catch(() => undefined);
    restore();
  };
}
