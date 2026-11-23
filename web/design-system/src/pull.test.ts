// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { pullGesture, type PullMeasure } from "./pull";

const measure: PullMeasure = { slop: 0.5, threshold: 4, most: 6, resistance: 0.5 };

/** A gesture at the top unless `top` says otherwise, at each call. */
function gesture(top: () => boolean = () => true) {
  return pullGesture(measure, top);
}

describe("a pull from the top", () => {
  test("follows the finger down with resistance, past the slop, to a farthest point", () => {
    const g = gesture();
    g.start(10, 10, 1);
    expect(g.move(10, 10.25, 1)).toBeUndefined();
    expect(g.move(10, 14.5, 1)).toEqual({ phase: "pulling", distance: 2 });
    expect(g.move(10, 18.5, 1)).toEqual({ phase: "armed", distance: 4 });
    expect(g.move(10, 40, 1)).toEqual({ phase: "armed", distance: 6 });
  });

  test("let go past the threshold reads the feed anew; short of it, nothing", () => {
    const past = gesture();
    past.start(0, 0, 1);
    past.move(0, 9, 1);
    expect(past.end()).toBe(true);
    const short = gesture();
    short.start(0, 0, 1);
    short.move(0, 8, 1);
    expect(short.end()).toBe(false);
  });

  test("pulled past the threshold and back short of it, lets go of nothing", () => {
    const g = gesture();
    g.start(0, 0, 1);
    g.move(0, 12, 1);
    expect(g.move(0, 4, 1)?.phase).toBe("pulling");
    expect(g.end()).toBe(false);
  });

  test("a touch that starts away from the top is no pull", () => {
    const g = gesture(() => false);
    g.start(0, 0, 1);
    expect(g.move(0, 12, 1)).toBeUndefined();
    expect(g.end()).toBe(false);
  });

  test("ends where the page leaves its top, and lets go of nothing", () => {
    let top = true;
    const g = gesture(() => top);
    g.start(0, 0, 1);
    expect(g.move(0, 12, 1)?.phase).toBe("armed");
    top = false;
    expect(g.move(0, 12, 1)).toBeUndefined();
    top = true;
    expect(g.move(0, 12, 1)).toBeUndefined();
    expect(g.end()).toBe(false);
  });

  test("a touch that goes up or sideways first is no pull, even when it then goes down", () => {
    const up = gesture();
    up.start(0, 10, 1);
    expect(up.move(0, 9, 1)).toBeUndefined();
    expect(up.move(0, 30, 1)).toBeUndefined();
    const across = gesture();
    across.start(0, 0, 1);
    expect(across.move(3, 1, 1)).toBeUndefined();
    expect(across.move(3, 30, 1)).toBeUndefined();
    expect(across.end()).toBe(false);
  });

  test("two fingers are no pull, and a second finger ends one", () => {
    const two = gesture();
    two.start(0, 0, 2);
    expect(two.move(0, 12, 2)).toBeUndefined();
    const added = gesture();
    added.start(0, 0, 1);
    added.move(0, 12, 1);
    expect(added.move(0, 12, 2)).toBeUndefined();
    expect(added.end()).toBe(false);
  });

  test("a touch the browser takes over lets go of nothing", () => {
    const g = gesture();
    g.start(0, 0, 1);
    g.move(0, 12, 1);
    g.cancel();
    expect(g.end()).toBe(false);
  });
});
