// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { fakeData } from "./test/data";
import {
  joinVoiceRoom,
  parseVoiceStreamEvent,
  runVoiceStream,
  sendVoiceHeartbeat,
  voiceStreamDelays,
  type VoiceStreamEvent,
} from "./voice";

/** Waits a turn of the event loop, for the fake's answers. */
const turn = () => new Promise((resolve) => setTimeout(resolve, 0));

/** A sleep that resolves when the test lets it, with the waits it was asked for. */
function heldSleep() {
  const waits: number[] = [];
  let wake: () => void = () => undefined;
  return {
    waits,
    sleep: (ms: number) =>
      new Promise<void>((resolve) => {
        waits.push(ms);
        wake = resolve;
      }),
    wake: () => {
      wake();
    },
  };
}

test("the room's stream starts with the room's state, and opens again on the same session when it drops", async () => {
  const { fake, data, requests } = fakeData();
  const room = fake.voice.roomId(0);
  const { session } = await joinVoiceRoom(data, room);
  const events: VoiceStreamEvent[] = [];
  const opens: boolean[] = [];
  const held = heldSleep();
  const stop = new AbortController();
  const running = runVoiceStream(data, {
    room,
    session,
    signal: stop.signal,
    onOpen: (again) => opens.push(again),
    onEvent: (event) => events.push(event),
    sleep: held.sleep,
  });
  for (let i = 0; i < 5; i++) await turn();
  expect(opens).toEqual([false]);
  expect(events.map((e) => e.type)).toEqual(["state"]);
  fake.voice.drop();
  for (let i = 0; i < 5; i++) await turn();
  expect(held.waits).toEqual([voiceStreamDelays[0] ?? 0]);
  held.wake();
  for (let i = 0; i < 5; i++) await turn();
  expect(opens).toEqual([false, true]);
  expect(requests.filter((r) => r.includes("/stream"))).toEqual([
    `GET /voice/rooms/${room}/stream?session=${session}`,
    `GET /voice/rooms/${room}/stream?session=${session}`,
  ]);
  stop.abort();
  await running;
});

test("the stream ends with its removed, replaced or ended, after handing it on", async () => {
  const { fake, data } = fakeData();
  const room = fake.voice.roomId(0);
  const { session } = await joinVoiceRoom(data, room);
  const events: VoiceStreamEvent[] = [];
  const running = runVoiceStream(data, {
    room,
    session,
    signal: new AbortController().signal,
    onEvent: (event) => events.push(event),
  });
  for (let i = 0; i < 5; i++) await turn();
  fake.voice.send({ event: "ended", data: { reason: "host" } });
  await running;
  expect(events.at(-1)).toEqual({ type: "ended", reason: "host" });
  // The server holds the session no more.
  expect(await sendVoiceHeartbeat(data, room, session).catch((error: unknown) => error)).toMatchObject({ status: 404 });
});

test("a stream the server refuses (no such session) rejects, and is not opened again", async () => {
  const { fake, data, requests } = fakeData();
  const room = fake.voice.roomId(0);
  const running = runVoiceStream(data, {
    room,
    session: "not-joined",
    signal: new AbortController().signal,
    onEvent: () => undefined,
  });
  expect(await running.catch((error: unknown) => error)).toMatchObject({ status: 404 });
  expect(requests.filter((r) => r.includes("/stream"))).toHaveLength(1);
});

test("an event of the stream is typed by its name, and one it cannot read is left out", () => {
  expect(parseVoiceStreamEvent("mic", '{"participant":"a1","on":true}')).toEqual({
    type: "mic",
    participant: "a1",
    on: true,
  });
  expect(parseVoiceStreamEvent("muted", '{"by":"a2"}')).toEqual({ type: "muted", by: "a2" });
  expect(parseVoiceStreamEvent("removed", '{"reason":"blocked"}')).toEqual({ type: "removed", reason: "blocked" });
  expect(parseVoiceStreamEvent("mic", '{"participant":"a1"}')).toBeNull();
  expect(parseVoiceStreamEvent("state", "not json")).toBeNull();
  expect(parseVoiceStreamEvent("newer", "{}")).toBeNull();
});
