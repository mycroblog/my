// SPDX-License-Identifier: AGPL-3.0-only

// The composer closed by a swipe down on a phone: where a swipe starts, what closes and what springs back, and what
// never moves it. The simulation lays nothing out: the motion is read from the styles the frame sets, and its time
// from the events' own, so a slow drag waits for real.

import { afterEach, expect, test } from "bun:test";
import { swipeClose, transition } from "@my/design-tokens";
import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ComposerFrame, type ComposerFrameProps } from "./composer";
import { curve, swipeExit } from "./modal";

type HappyWindow = {
  happyDOM: {
    setViewport(viewport: { width: number; height: number }): void;
    settings: { device: { prefersReducedMotion: string } };
  };
};
const { happyDOM } = window as unknown as HappyWindow;

/** A phone's viewport, or a tablet's. */
const phone = () => {
  happyDOM.setViewport({ width: 375, height: 700 });
};
const tablet = () => {
  happyDOM.setViewport({ width: 800, height: 1000 });
};

/** The reader's motion preference, as happy-dom's media queries answer it. */
function reduceMotion(reduce: boolean) {
  happyDOM.settings.device.prefersReducedMotion = reduce ? "reduce" : "no-preference";
}

afterEach(() => {
  reduceMotion(false);
  happyDOM.setViewport({ width: 1024, height: 768 });
});

/** The px of a rem in the simulation. */
const rem = 16;

/** A touch event of `type` with a finger at each of `at` (px), none for an end. */
function touch(target: Element, type: string, ...at: { x: number; y: number }[]) {
  const e = new Event(type, { bubbles: true, cancelable: true });
  Object.defineProperty(e, "touches", { value: at.map(({ x, y }) => ({ clientX: x, clientY: y })) });
  act(() => {
    target.dispatchEvent(e);
  });
}

/** Time passing between two touches, for the gesture's clock, which is the events'. */
const pause = (ms: number) =>
  act(async () => {
    await new Promise((resolve) => setTimeout(resolve, ms));
  });

/**
 * A finger on `target` drawn down by each of `downs` rem from where it went down, waiting `rest` ms before it lets go:
 * a release after a rest past the velocity's window is a slow one.
 */
async function drag(target: Element, downs: number[], { rest = 0, x = 20 }: { rest?: number; x?: number } = {}) {
  const y = 100;
  touch(target, "touchstart", { x, y });
  for (const down of downs) touch(target, "touchmove", { x, y: y + down * rem });
  if (rest > 0) await pause(rest);
  touch(target, "touchend");
}

/** A release slow enough that only the distance decides. */
const slow = swipeClose.window + 60;

/** The composer over a page, with a text that holds words, and how it was asked to leave. */
async function composer(props: Partial<ComposerFrameProps> = {}) {
  const left: string[] = [];
  render(
    <>
      <button type="button">Behind</button>
      <ComposerFrame
        title="New post"
        closeLabel="Close"
        onClose={() => left.push("close")}
        onDismiss={() => left.push("dismiss")}
        action={<button type="button">Post</button>}
        {...props}
      >
        <div role="textbox" aria-label="Text" contentEditable suppressContentEditableWarning>
          A walk along the river this morning.
        </div>
      </ComposerFrame>
    </>,
  );
  const dialog = await screen.findByRole("dialog", { name: "New post" });
  const head = within(dialog).getByRole("heading", { name: "New post" });
  const text = within(dialog).getByRole("textbox", { name: "Text" });
  const body = dialog.querySelector<HTMLElement>("[data-composer-body]");
  if (body === null) throw new Error("the composer has no body");
  return { left, dialog, head, text, body };
}

/** The elements of the document the frame moves, with how far. */
const moved = () =>
  [...document.querySelectorAll<HTMLElement>("[style]")]
    .map((el) => el.style.translate)
    .filter((translate) => translate !== "" && translate !== "none");

/** What each element the frame fades keeps of its opacity. */
const faded = () =>
  [...document.querySelectorAll<HTMLElement>("[style]")]
    .map((el) => /opacity\(([\d.]+)\)/.exec(el.style.filter)?.[1])
    .filter((share) => share !== undefined)
    .map(Number);

test("on a phone, a swipe down from the head follows the finger and closes the composer as Close does", async () => {
  phone();
  const { left, head } = await composer();

  touch(head, "touchstart", { x: 20, y: 100 });
  touch(head, "touchmove", { x: 20, y: 120 });
  touch(head, "touchmove", { x: 20, y: 140 });
  expect(moved()).toEqual(["0 2.5rem"]);
  expect(faded().length).toBeGreaterThan(0);
  touch(head, "touchmove", { x: 20, y: 100 + 8 * rem });
  await pause(slow);
  touch(head, "touchend");

  expect(left).toEqual(["close"]);
});

test("a short fast flick down closes it", async () => {
  phone();
  const { left, head } = await composer();

  await drag(head, [1, 2.5]);

  expect(left).toEqual(["close"]);
});

test("a drag short of the distance, let go slowly, springs back and closes nothing", async () => {
  phone();
  const { left, head } = await composer();

  await drag(head, [1, 2, 3, 4], { rest: slow });

  expect(left).toEqual([]);
  expect(moved()).toEqual([]);
  expect(faded().every((share) => share === 1)).toBe(true);
  expect(screen.getByRole("dialog", { name: "New post" })).not.toBeNull();
});

test("a drag past the distance, let go slowly, closes it", async () => {
  phone();
  const { left, head } = await composer();

  await drag(head, [1, 4, swipeClose.distance + 1], { rest: slow });

  expect(left).toEqual(["close"]);
});

test("a flick back up keeps it open, even from past the distance", async () => {
  phone();
  const { left, head } = await composer();

  touch(head, "touchstart", { x: 20, y: 100 });
  touch(head, "touchmove", { x: 20, y: 100 + rem });
  touch(head, "touchmove", { x: 20, y: 100 + 8 * rem });
  await pause(slow);
  for (const down of [8, 7.5, 7, 6.5]) touch(head, "touchmove", { x: 20, y: 100 + down * rem });
  touch(head, "touchend");

  expect(left).toEqual([]);
});

test("however far it is dragged, the backing keeps at least 70% of itself", async () => {
  phone();
  const { head } = await composer();

  touch(head, "touchstart", { x: 20, y: 100 });
  touch(head, "touchmove", { x: 20, y: 100 + rem });
  touch(head, "touchmove", { x: 20, y: 100 + 30 * rem });

  expect(faded().length).toBeGreaterThan(0);
  expect(Math.min(...faded())).toBeCloseTo(1 - swipeClose.dim);
  touch(head, "touchcancel");
});

test("a swipe also starts in the text, while it is at its top and nothing is selected", async () => {
  phone();
  const { left, text } = await composer();

  await drag(text, [1, swipeClose.distance + 2], { rest: slow });

  expect(left).toEqual(["close"]);
});

test("a swipe in the text closes nothing while the text is scrolled down", async () => {
  phone();
  const { left, text, body } = await composer();
  body.scrollTop = 40;

  await drag(text, [1, swipeClose.distance + 2], { rest: slow });

  expect(left).toEqual([]);
  expect(moved()).toEqual([]);
});

test("a swipe in the text closes nothing while something is selected", async () => {
  phone();
  const { left, text } = await composer();
  document.getSelection()?.selectAllChildren(text);

  await drag(text, [1, swipeClose.distance + 2], { rest: slow });

  expect(left).toEqual([]);
  expect(moved()).toEqual([]);
  document.getSelection()?.removeAllRanges();
});

test("a finger that rests in the text past the hold selects, and never swipes", async () => {
  phone();
  const { left, text } = await composer();

  touch(text, "touchstart", { x: 20, y: 100 });
  await pause(swipeClose.hold + 50);
  touch(text, "touchmove", { x: 20, y: 100 + rem });
  touch(text, "touchmove", { x: 20, y: 100 + 10 * rem });
  touch(text, "touchend");

  expect(left).toEqual([]);
  expect(moved()).toEqual([]);
});

test("on the head a finger may rest before it swipes", async () => {
  phone();
  const { left, head } = await composer();

  touch(head, "touchstart", { x: 20, y: 100 });
  await pause(swipeClose.hold + 50);
  touch(head, "touchmove", { x: 20, y: 100 + rem });
  touch(head, "touchmove", { x: 20, y: 100 + 10 * rem });
  touch(head, "touchend");

  expect(left).toEqual(["close"]);
});

test("a drag that goes sideways first closes nothing", async () => {
  phone();
  const { left, head } = await composer();

  touch(head, "touchstart", { x: 20, y: 100 });
  touch(head, "touchmove", { x: 60, y: 104 });
  touch(head, "touchmove", { x: 60, y: 100 + 10 * rem });
  touch(head, "touchend");

  expect(left).toEqual([]);
  expect(moved()).toEqual([]);
});

test("a second finger springs it back, and nothing closes", async () => {
  phone();
  const { left, head } = await composer();

  touch(head, "touchstart", { x: 20, y: 100 });
  touch(head, "touchmove", { x: 20, y: 100 + 3 * rem });
  touch(head, "touchmove", { x: 20, y: 100 + 8 * rem }, { x: 120, y: 100 });
  touch(head, "touchend");

  expect(left).toEqual([]);
  expect(moved()).toEqual([]);
});

test("while a post is in flight, a swipe neither moves nor closes it", async () => {
  phone();
  const { left, head, text } = await composer({ busy: true });

  touch(head, "touchstart", { x: 20, y: 100 });
  touch(head, "touchmove", { x: 20, y: 100 + rem });
  touch(head, "touchmove", { x: 20, y: 100 + 10 * rem });
  expect(moved()).toEqual([]);
  touch(head, "touchend");
  await drag(text, [1, 10]);

  expect(left).toEqual([]);
});

test("the dialog from a tablet never moves", async () => {
  tablet();
  const { left, head, text } = await composer();

  touch(head, "touchstart", { x: 20, y: 100 });
  touch(head, "touchmove", { x: 20, y: 100 + rem });
  touch(head, "touchmove", { x: 20, y: 100 + 10 * rem });
  expect(moved()).toEqual([]);
  touch(head, "touchend");
  await drag(text, [1, 10]);

  expect(left).toEqual([]);
});

test("a mouse's drag down the head closes nothing: the composer swipes by touch alone", async () => {
  phone();
  const user = userEvent.setup();
  const { left, head } = await composer();

  await user.pointer([
    { keys: "[MouseLeft>]", target: head, coords: { clientX: 20, clientY: 100 } },
    { pointerName: "mouse", target: head, coords: { clientX: 20, clientY: 140 } },
    { pointerName: "mouse", target: head, coords: { clientX: 20, clientY: 100 + 10 * rem } },
    { keys: "[/MouseLeft]", target: head, coords: { clientX: 20, clientY: 100 + 10 * rem } },
  ]);

  expect(left).toEqual([]);
  expect(moved()).toEqual([]);
});

test("with reduced motion nothing translates: the composer fades by the share, and a release past the distance closes at once", async () => {
  // The preference first: the viewport's change is what tells the media queries already asked.
  reduceMotion(true);
  phone();
  const { left, head } = await composer();

  touch(head, "touchstart", { x: 20, y: 100 });
  touch(head, "touchmove", { x: 20, y: 100 + rem });
  touch(head, "touchmove", { x: 20, y: 100 + 3 * rem });
  expect(moved()).toEqual([]);
  expect(faded()).toContain(1 - swipeClose.dim * (3 / swipeClose.distance));
  touch(head, "touchmove", { x: 20, y: 100 + 8 * rem });
  touch(head, "touchend");

  expect(left).toEqual(["close"]);
});

test("the leave slides the surface down out of the viewport, at the modal's duration and its exit easing", () => {
  phone();
  const exit = swipeExit();

  expect(exit.y).toBeGreaterThanOrEqual(window.innerHeight);
  expect(exit.transition.duration).toBe(transition.duration.modal / 1000);
  expect(exit.transition.ease).toEqual(curve(transition.ease.exit));
});
