// SPDX-License-Identifier: AGPL-3.0-only

// What the call's store tells the voice feature. What the shell's bar shows of the call needs a rendered page (useCall
// reads the store in the browser alone): the Shell story InCall of web/app checks it.

import { afterEach, expect, test } from "bun:test";
import { enterCall, leaveCall, muteCall, updateCall, type Call } from "./store";

const room: Call = { roomId: "r1", room: "Morning coffee chat", host: "Sam Rivera", participants: 12, muted: false };

afterEach(() => {
  leaveCall();
});

test("muting from the bar tells the voice feature to turn the microphone off, and unmuting to turn it on", () => {
  const told: boolean[] = [];
  enterCall(room, {
    setMuted: (muted) => {
      told.push(muted);
    },
  });
  muteCall(true);
  muteCall(false);
  expect(told).toEqual([true, false]);
});

test("leaving from the bar tells the voice feature once, and a call left has nothing more to tell", () => {
  const told: string[] = [];
  enterCall(room, { setMuted: () => told.push("mute"), leave: () => told.push("leave") });
  leaveCall();
  muteCall(true);
  leaveCall();
  expect(told).toEqual(["leave"]);
});

test("one call at a time: entering another replaces the first, and its controls with it", () => {
  const told: string[] = [];
  enterCall(room, { leave: () => told.push("r1") });
  enterCall({ ...room, roomId: "r2", room: "Release party" }, { leave: () => told.push("r2") });
  updateCall({ participants: 13 });
  leaveCall();
  expect(told).toEqual(["r2"]);
});

test("a call the voice feature gave no controls mutes and leaves without them", () => {
  enterCall(room);
  expect(() => {
    muteCall(true);
    leaveCall();
  }).not.toThrow();
});
