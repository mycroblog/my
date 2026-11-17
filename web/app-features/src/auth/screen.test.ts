// SPDX-License-Identifier: AGPL-3.0-only

// The logic of the auth screens that needs no DOM: the consent's wait, and the data block the entry reads. What each
// screen shows and what its form submits is in screen.dom.test.tsx.

import { expect, test } from "bun:test";
import { holdWhileWaiting, secondsLeft } from "./consent-wait";
import { parseAuthPage } from "./data";

test("the consent's wait counts whole seconds down to 0, and no submission gets through while it lasts", () => {
  expect([0, 1, 999, 1000, 4001, 5000, 9000].map((ms) => secondsLeft(ms))).toEqual([5, 5, 5, 4, 1, 0, 0]);
  let prevented = 0;
  const event = { preventDefault: () => void prevented++ };
  holdWhileWaiting(3, event);
  holdWhileWaiting(1, event);
  holdWhileWaiting(0, event);
  expect(prevented).toBe(2);
});

test("the data block names its screen and its server, or the entry fails", () => {
  expect(parseAuthPage('{"screen":"sign_in","server":{"name":"x"}}').screen).toBe("sign_in");
  for (const bad of [null, "[]", '{"screen":"nope","server":{"name":"x"}}', '{"screen":"sign_in"}'])
    expect(() => parseAuthPage(bad)).toThrow();
});
