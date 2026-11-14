// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { coveredHeight, watchKeyboardInset } from "./keyboard-inset";

/** A visual viewport whose size the test sets, and its listeners. */
function fakeViewport(height: number) {
  const listeners = new Set<() => void>();
  const viewport = {
    height,
    offsetTop: 0,
    addEventListener: (_: string, l: () => void) => {
      listeners.add(l);
    },
    removeEventListener: (_: string, l: () => void) => {
      listeners.delete(l);
    },
  };
  const fire = () => {
    for (const l of listeners) l();
  };
  return { viewport, fire, listeners };
}

test("what the keyboard covers is the layout height the visual viewport lost, never below 0", () => {
  expect(coveredHeight(800, { height: 800, offsetTop: 0 })).toBe(0);
  expect(coveredHeight(800, { height: 500, offsetTop: 0 })).toBe(300);
  expect(coveredHeight(800, { height: 500, offsetTop: 100 })).toBe(200);
  expect(coveredHeight(800, { height: 900, offsetTop: 0 })).toBe(0);
});

test("it tells the inset at once and on each change, and stops when asked", () => {
  const { viewport, fire, listeners } = fakeViewport(800);
  const heard: number[] = [];
  const stop = watchKeyboardInset((px) => heard.push(px), {
    viewport: viewport as never,
    layoutHeight: () => 800,
  });
  viewport.height = 480;
  fire();
  fire();
  expect(heard).toEqual([0, 320]);
  stop();
  expect(listeners.size).toBe(0);
});

test("without a visual viewport it tells nothing", () => {
  const heard: number[] = [];
  watchKeyboardInset((px) => heard.push(px), { viewport: null })();
  expect(heard).toEqual([]);
});
