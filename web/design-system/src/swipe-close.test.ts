// SPDX-License-Identifier: AGPL-3.0-only

// Closing a surface by a swipe down: the gesture, in rem and ms, and the styles of the surface and its backing as it
// follows the finger, springs back and leaves. Whether a surface listens is its own test's (composer-swipe, viewer-swipe).

import { describe, expect, test } from "bun:test";
import { swipeClose, transition } from "@my/design-tokens";
import { swipeAtRest, swipeBackingStyle, swipeGesture, swipeSurfaceStyle, type SwipeState } from "./swipe-close";

type Step = readonly [x: number, y: number, t: number, touches?: number];

/** A gesture of one finger that starts at (0, 0) at 0 ms, `held` if it starts where a hold selects, and its moves. */
function swipe(steps: readonly Step[], held?: boolean) {
  const g = swipeGesture();
  g.start(0, 0, 0, 1, held);
  const downs = steps.map(([x, y, t, touches = 1]) => g.move(x, y, t, touches));
  return { g, downs };
}

/** Whether a gesture of `steps`, let go at `end` ms, closes. */
const closes = (steps: readonly Step[], end: number, held?: boolean) => swipe(steps, held).g.end(end);

describe("a swipe down", () => {
  test("follows the finger from where it started once past the slop, and never above it", () => {
    const { downs } = swipe([
      [0, swipeClose.slop / 2, 10],
      [0, 3, 500],
      [0, -1, 1000],
      [0, 7, 1500],
    ]);
    expect(downs).toEqual([undefined, 3, 0, 7]);
  });

  test("let go slowly at the distance closes; short of it, springs back", () => {
    expect(
      closes(
        [
          [0, 1, 250],
          [0, swipeClose.distance, 1000],
        ],
        1500,
      ),
    ).toBe(true);
    expect(
      closes(
        [
          [0, 1, 250],
          [0, swipeClose.distance - 0.1, 1000],
        ],
        1500,
      ),
    ).toBe(false);
    expect(
      closes(
        [
          [0, 1, 250],
          [0, 2, 500],
          [0, 3, 750],
          [0, 4, 1000],
        ],
        1010,
      ),
    ).toBe(false);
  });

  test("a flick faster than the velocity over the last window closes, however short", () => {
    // 2.6 rem in 100 ms is 26 rem/s; 2.4 rem, 24 rem/s.
    expect(
      closes(
        [
          [0, 1, 1000],
          [0, 3.6, 1000 + swipeClose.window],
        ],
        1100,
      ),
    ).toBe(true);
    expect(
      closes(
        [
          [0, 1, 1000],
          [0, 3.4, 1000 + swipeClose.window],
        ],
        1100,
      ),
    ).toBe(false);
    expect(
      closes(
        [
          [0, 1, 1000],
          [0, 2, 1020],
        ],
        1020,
      ),
    ).toBe(true);
  });

  test("a flick that stops before the release springs back", () => {
    expect(
      closes(
        [
          [0, 1, 10],
          [0, 4, 60],
        ],
        70,
      ),
    ).toBe(true);
    expect(
      closes(
        [
          [0, 1, 10],
          [0, 4, 60],
        ],
        500,
      ),
    ).toBe(false);
  });

  test("a flick upward keeps it open, even past the distance", () => {
    expect(
      closes(
        [
          [0, 1, 500],
          [0, 8, 2000],
          [0, 6.5, 2050],
        ],
        2060,
      ),
    ).toBe(false);
    expect(
      closes(
        [
          [0, 1, 500],
          [0, 4, 1000],
          [0, 1, 1050],
        ],
        1060,
      ),
    ).toBe(false);
  });

  test("a touch that goes sideways or up first is never a swipe, even when it then goes down", () => {
    const across = swipe([
      [1, 0.2, 10],
      [1, 8, 20],
    ]);
    expect(across.downs).toEqual([undefined, undefined]);
    expect(across.g.end(30)).toBe(false);

    const up = swipe([
      [0, -1, 10],
      [0, 10, 20],
    ]);
    expect(up.downs).toEqual([undefined, undefined]);
    expect(up.g.end(1000)).toBe(false);
  });

  test("a touch that goes down more than sideways is a swipe", () => {
    expect(swipe([[0.4, 0.6, 10]]).downs).toEqual([0.6]);
  });

  test("where a hold selects, the first move past the slop must come within the hold", () => {
    const late = swipe(
      [
        [0, 1, swipeClose.hold + 50],
        [0, 9, 400],
      ],
      true,
    );
    expect(late.downs).toEqual([undefined, undefined]);
    expect(late.g.end(2000)).toBe(false);

    // A finger that rests inside the slop past the hold is selecting too.
    expect(
      swipe(
        [
          [0, 0.2, 100],
          [0, 3, 400],
        ],
        true,
      ).downs,
    ).toEqual([undefined, undefined]);

    const early = swipe(
      [
        [0, 9, swipeClose.hold - 10],
        [0, 12, 900],
      ],
      true,
    );
    expect(early.downs).toEqual([9, 12]);
    expect(early.g.end(1500)).toBe(true);
  });

  test("elsewhere a finger may rest before it moves", () => {
    const g = swipe(
      [
        [0, 1, swipeClose.hold + 50],
        [0, 9, 400],
      ],
      false,
    );
    expect(g.downs).toEqual([1, 9]);
    expect(g.g.end(2000)).toBe(true);
  });

  test("a second finger ends it and nothing closes, even past the distance", () => {
    const added = swipe([
      [0, 3, 10],
      [0, 4, 20, 2],
      [0, 9, 30],
    ]);
    expect(added.downs).toEqual([3, undefined, undefined]);
    expect(added.g.end(40)).toBe(false);
    expect(
      swipe([
        [0, 8, 10],
        [0, 8, 20, 2],
      ]).g.end(40),
    ).toBe(false);

    const two = swipeGesture();
    two.start(0, 0, 0, 2);
    expect(two.move(0, 8, 10, 2)).toBeUndefined();
    expect(two.move(0, 9, 20, 1)).toBeUndefined();
    expect(two.end(30)).toBe(false);
  });

  test("a touch the browser takes over closes nothing", () => {
    const { g } = swipe([[0, 8, 10]]);
    g.cancel();
    expect(g.end(20)).toBe(false);
  });

  test("a gesture starts anew at each touch", () => {
    const { g } = swipe([[0, 8, 10]]);
    expect(g.end(20)).toBe(true);
    g.start(0, 0, 100, 1);
    expect(g.move(0, 3, 1000, 1)).toBe(3);
    expect(g.end(1500)).toBe(false);
  });

  test("follows the measure it is given", () => {
    const g = swipeGesture({ slop: 1, distance: 2, velocity: 1000, window: 100, dim: 0.5, hold: 300 });
    g.start(0, 0, 0, 1);
    expect(g.move(0, 0.8, 1000, 1)).toBeUndefined();
    expect(g.move(0, 2.1, 2000, 1)).toBe(2.1);
    expect(g.end(3000)).toBe(true);
  });
});

/** What a style keeps of its element's opacity, 1 where it says none. */
const kept = (style: { filter?: string }) => {
  const match = /^opacity\(([\d.]+)\)$/.exec(style.filter ?? "opacity(1)");
  if (match?.[1] === undefined) throw new Error(`not an opacity: ${String(style.filter)}`);
  return Number(match[1]);
};

const drag = (down: number): SwipeState => ({ phase: "drag", down });

describe("the styles of a swipe", () => {
  test("at rest the surface and its backing are as they were", () => {
    expect(swipeAtRest).toEqual({ phase: "rest", down: 0 });
    for (const reduced of [false, true]) {
      expect(swipeSurfaceStyle(swipeAtRest, reduced)).toEqual({});
      expect(swipeBackingStyle(swipeAtRest, reduced)).toEqual({});
    }
  });

  test("while dragged the surface follows the finger, and the backing keeps 1 − dim of the share of the distance", () => {
    expect(swipeSurfaceStyle(drag(3), false)).toEqual({ translate: "0 3rem" });
    expect(swipeBackingStyle(drag(3), false)).toEqual({ filter: "opacity(0.85)" });
    expect(kept(swipeBackingStyle(drag(swipeClose.distance), false))).toBeCloseTo(1 - swipeClose.dim);
  });

  test("however far it is dragged, the backing keeps at least 70%", () => {
    for (const down of [0, 1, 3, 6, 12, 40]) {
      const share = kept(swipeBackingStyle(drag(down), false));
      expect(share).toBeGreaterThanOrEqual(0.7);
      expect(share).toBeCloseTo(1 - 0.3 * Math.min(1, down / swipeClose.distance));
    }
    expect(swipeSurfaceStyle(drag(12), false)).toEqual({ translate: "0 12rem" });
  });

  test("a spring back takes the surface home and the backing back to whole, at the pop's duration and the enter easing", () => {
    const back: SwipeState = { phase: "back", down: 3 };
    const timing = `${String(transition.duration.pop)}ms ${transition.ease.enter}`;
    expect(swipeSurfaceStyle(back, false)).toEqual({ translate: "none", transition: `translate ${timing}` });
    expect(swipeBackingStyle(back, false)).toEqual({ transition: `filter ${timing}` });
  });

  test("with reduced motion nothing translates: the surface and its backing fade by the share, and come back at once", () => {
    for (const down of [3, 12]) {
      const surface = swipeSurfaceStyle(drag(down), true);
      expect(surface.translate).toBeUndefined();
      expect(kept(surface)).toBeCloseTo(1 - 0.3 * Math.min(1, down / swipeClose.distance));
      expect(swipeBackingStyle(drag(down), true)).toEqual(swipeBackingStyle(drag(down), false));
    }
    const back: SwipeState = { phase: "back", down: 3 };
    expect(swipeSurfaceStyle(back, true)).toEqual({});
    expect(swipeBackingStyle(back, true)).toEqual({});
  });

  test("a leave starts where the finger let go", () => {
    const leave: SwipeState = { phase: "leave", down: 3 };
    expect(swipeSurfaceStyle(leave, false)).toEqual(swipeSurfaceStyle(drag(3), false));
    expect(swipeSurfaceStyle(leave, true).translate).toBeUndefined();
  });
});
