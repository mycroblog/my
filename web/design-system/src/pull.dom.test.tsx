// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, test } from "bun:test";
import { act, render, screen } from "@testing-library/react";
import { Entry } from "./entry";
import { Feed, FeedItem } from "./feed";
import { FeedPulls } from "./pull";

/** A pull of the test: what it was asked, and its end, which the test settles. */
function puller() {
  const asked: Array<() => void> = [];
  const onPull = () =>
    new Promise<void>((resolve) => {
      asked.push(resolve);
    });
  return { asked, onPull };
}

/** A feed of two posts that `onPull` reads anew, where the app lets feeds be pulled (`on`). */
function feed(onPull: () => Promise<unknown>, on = true) {
  return render(
    <FeedPulls on={on}>
      <Feed label="Home" count={2} more={false} busy={false} onPull={onPull}>
        {["1", "2"].map((id, i) => (
          <FeedItem key={id} id={id} position={i + 1}>
            <Entry>
              <p>Post {id}</p>
            </Entry>
          </FeedItem>
        ))}
      </Feed>
    </FeedPulls>,
  );
}

/** A touch event of `type` with one finger at `y` px, or none (an end). */
function touch(target: Element, type: string, y?: number) {
  const e = new Event(type, { bubbles: true });
  const touches = y === undefined ? [] : [{ clientX: 0, clientY: y }];
  Object.defineProperty(e, "touches", { value: touches });
  act(() => {
    target.dispatchEvent(e);
  });
}

/** A finger on the first post, pulled down by `by` px, then let go. */
function pullDown(by: number) {
  const post = screen.getByText("Post 1");
  touch(post, "touchstart", 0);
  touch(post, "touchmove", by / 2);
  touch(post, "touchmove", by);
  touch(post, "touchend");
}

/** The end of what the pull read, and of the mark's motion as it leaves. */
const settle = () =>
  act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 400));
  });

/** The live region of the pull, which says while the feed is read anew. */
const place = () => document.querySelector('[role="status"][data-feed-pulls]');

afterEach(() => {
  document.documentElement.scrollTop = 0;
});

describe("a feed pulled from the top", () => {
  test("past the threshold, reads the feed anew and says so until it is read", async () => {
    const { asked, onPull } = puller();
    feed(onPull);
    // 200 px is 12.5 rem: half of it past the slop is beyond the threshold.
    pullDown(200);
    expect(asked).toHaveLength(1);
    expect(place()?.textContent).toBe("Loading what’s new…");
    asked[0]?.();
    await settle();
    expect(place()?.textContent).toBe("");
  });

  test("short of the threshold, reads nothing", async () => {
    const { asked, onPull } = puller();
    feed(onPull);
    pullDown(48);
    await settle();
    expect(asked).toHaveLength(0);
  });

  test("away from the top, reads nothing", () => {
    const { asked, onPull } = puller();
    feed(onPull);
    document.documentElement.scrollTop = 300;
    pullDown(200);
    expect(asked).toHaveLength(0);
  });

  test("where the app does not let feeds be pulled (from tablet), reads nothing and holds no place for it", () => {
    const { asked, onPull } = puller();
    feed(onPull, false);
    pullDown(200);
    expect(asked).toHaveLength(0);
    expect(document.querySelector("[data-feed-pulls]")).toBeNull();
  });

  test("with a mouse, reads nothing", () => {
    const { asked, onPull } = puller();
    feed(onPull);
    const post = screen.getByText("Post 1");
    for (const [type, y] of [
      ["pointerdown", 0],
      ["mousedown", 0],
      ["pointermove", 200],
      ["mousemove", 200],
      ["pointerup", 200],
      ["mouseup", 200],
    ] as const)
      act(() => {
        post.dispatchEvent(new MouseEvent(type, { bubbles: true, clientY: y }));
      });
    expect(asked).toHaveLength(0);
  });
});
