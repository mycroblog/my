// SPDX-License-Identifier: AGPL-3.0-only

// The media viewer closed by a swipe down: what closes it and where the focus goes, what springs back, and what a drag
// still does instead: move between the media, pan a zoomed picture. The simulation lays nothing out: the motion is
// read from the styles the viewer sets, and its time from the events' own, so a slow drag waits for real.

import { afterEach, expect, test } from "bun:test";
import { swipeClose } from "@my/design-tokens";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import type { MediaItem } from "./media";
import { MediaViewer } from "./viewer";

type HappyWindow = {
  happyDOM: {
    setViewport(viewport: { width: number; height: number }): void;
    settings: { device: { prefersReducedMotion: string } };
  };
};
const { happyDOM } = window as unknown as HappyWindow;

/** The reader's motion preference, as happy-dom's media queries answer it. */
function reduceMotion(reduce: boolean) {
  happyDOM.settings.device.prefersReducedMotion = reduce ? "reduce" : "no-preference";
}

afterEach(() => {
  reduceMotion(false);
  happyDOM.setViewport({ width: 1024, height: 768 });
});

const items: MediaItem[] = [
  { id: "a", src: "https://m.example/a-small.webp", href: "https://m.example/a.webp", alt: "A ridge at dawn" },
  { id: "b", src: "https://m.example/b-small.webp", href: "https://m.example/b.webp", alt: "" },
];

/** The px of a rem in the simulation. */
const rem = 16;

type Calls = { moves: number[]; closes: number };

/** A post's button that opens the viewer, as a post holds it open, and what the viewer asked for. */
function Post({ calls }: { calls: Calls }) {
  const [index, setIndex] = useState(0);
  const [open, setOpen] = useState(false);
  return (
    <>
      <button
        type="button"
        onClick={() => {
          setOpen(true);
        }}
      >
        Open media
      </button>
      {open && (
        <MediaViewer
          items={items}
          index={index}
          onIndexChange={(next) => {
            calls.moves.push(next);
            setIndex(next);
          }}
          onClose={() => {
            calls.closes += 1;
            setOpen(false);
          }}
          locale="en"
        />
      )}
    </>
  );
}

/** Opens the viewer on a phone from the post's button, once it holds the focus: its dialog, picture and calls. */
async function opened() {
  happyDOM.setViewport({ width: 375, height: 700 });
  const user = userEvent.setup();
  const calls: Calls = { moves: [], closes: 0 };
  render(<Post calls={calls} />);
  await user.click(screen.getByRole("button", { name: "Open media" }));
  const dialog = await screen.findByRole("dialog", { name: "Media" });
  await waitFor(() => {
    expect(dialog.contains(document.activeElement)).toBe(true);
  });
  return { user, dialog, calls, picture: () => within(dialog).getByRole("presentation") };
}

/** Time passing, for the gesture's clock, which is the events'. */
const pause = (ms: number) =>
  act(async () => {
    await new Promise((resolve) => setTimeout(resolve, ms));
  });

/** A release slow enough that only the distance decides. */
const slow = swipeClose.window + 60;

type Point = { x: number; y: number };

/** A mouse pressed on `target` at `from`, drawn through `through`, and let go there after `rest` ms. */
async function drag(
  user: ReturnType<typeof userEvent.setup>,
  target: Element,
  from: Point,
  through: Point[],
  rest = 0,
) {
  await user.pointer([
    { keys: "[MouseLeft>]", target, coords: { clientX: from.x, clientY: from.y } },
    ...through.map(({ x, y }) => ({ pointerName: "mouse", target, coords: { clientX: x, clientY: y } })),
  ]);
  if (rest > 0) await pause(rest);
  const last = through.at(-1) ?? from;
  await user.pointer({ keys: "[/MouseLeft]", target, coords: { clientX: last.x, clientY: last.y } });
}

/** The group that moves with the finger: the picture, its caption and its controls. */
const groupOf = (dialog: HTMLElement) => {
  const group = dialog.querySelector<HTMLElement>("[data-viewer-backing]");
  if (group === null) throw new Error("the viewer has no moving group");
  return group;
};

/** What each element the viewer fades keeps of its opacity. */
const faded = () =>
  [...document.querySelectorAll<HTMLElement>("[style]")]
    .map((el) => /opacity\(([\d.]+)\)/.exec(el.style.filter)?.[1])
    .filter((share) => share !== undefined)
    .map(Number);

const moved = (el: HTMLElement) => el.style.translate !== "" && el.style.translate !== "none";

test("a swipe down on a picture that is not zoomed closes the viewer, and the focus goes back to what opened it", async () => {
  const { user, calls, picture } = await opened();

  await drag(user, picture(), { x: 100, y: 100 }, [
    { x: 100, y: 200 },
    { x: 100, y: 300 },
  ]);

  expect(calls.closes).toBe(1);
  expect(calls.moves).toEqual([]);
  expect(screen.queryByRole("dialog")).toBeNull();
  await waitFor(() => {
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Open media" }));
  });
});

test("while dragged down the picture follows the finger and the rest fades, keeping at least 70%", async () => {
  const { user, dialog, picture } = await opened();
  const target = picture();

  await user.pointer([
    { keys: "[MouseLeft>]", target, coords: { clientX: 100, clientY: 100 } },
    { pointerName: "mouse", target, coords: { clientX: 100, clientY: 120 } },
    { pointerName: "mouse", target, coords: { clientX: 100, clientY: 140 } },
  ]);
  expect(groupOf(dialog).style.translate).toBe("0 2.5rem");
  expect(faded()).toContain(1 - swipeClose.dim * (2.5 / swipeClose.distance));

  await user.pointer({ pointerName: "mouse", target, coords: { clientX: 100, clientY: 100 + 30 * rem } });
  expect(faded().length).toBeGreaterThan(0);
  expect(Math.min(...faded())).toBeCloseTo(1 - swipeClose.dim);
});

test("a drag past the distance, let go slowly, closes the viewer", async () => {
  const { user, calls, picture } = await opened();

  await drag(
    user,
    picture(),
    { x: 100, y: 100 },
    [
      { x: 100, y: 140 },
      { x: 100, y: 100 + (swipeClose.distance + 1) * rem },
    ],
    slow,
  );

  expect(calls.closes).toBe(1);
});

test("a short drag let go slowly springs back and keeps the viewer open", async () => {
  const { user, dialog, calls, picture } = await opened();

  await drag(
    user,
    picture(),
    { x: 100, y: 100 },
    [
      { x: 100, y: 115 },
      { x: 100, y: 130 },
    ],
    slow,
  );

  expect(calls.closes).toBe(0);
  expect(moved(groupOf(dialog))).toBe(false);
  expect(faded().every((share) => share === 1)).toBe(true);
  expect(screen.getByRole("dialog", { name: "Media" })).not.toBeNull();
});

test("a horizontal swipe still moves to the next medium, and closes nothing", async () => {
  const { user, dialog, calls, picture } = await opened();

  await drag(user, picture(), { x: 300, y: 100 }, [
    { x: 100, y: 105 },
    { x: 20, y: 110 },
  ]);

  expect(calls.moves).toEqual([1]);
  expect(calls.closes).toBe(0);
  expect(moved(groupOf(dialog))).toBe(false);
  expect(within(dialog).getByText("2 of 2")).not.toBeNull();
});

test("a drag down a zoomed picture pans it, and neither moves nor closes the viewer", async () => {
  const { user, dialog, calls, picture } = await opened();
  await user.click(within(dialog).getByRole("button", { name: "Zoom in" }));
  const before = picture().style.transform;

  await drag(user, picture(), { x: 100, y: 100 }, [
    { x: 100, y: 200 },
    { x: 100, y: 300 },
  ]);

  expect(calls.closes).toBe(0);
  expect(moved(groupOf(dialog))).toBe(false);
  expect(picture().style.transform).not.toBe(before);
  expect(within(dialog).getByRole("button", { name: "Zoom out" })).not.toBeNull();
});

test("a second finger springs the picture back, and nothing closes", async () => {
  const { user, dialog, calls, picture } = await opened();
  const target = picture();

  await user.pointer([
    { keys: "[TouchA>]", target, coords: { clientX: 100, clientY: 100 } },
    { pointerName: "TouchA", target, coords: { clientX: 100, clientY: 140 } },
  ]);
  expect(groupOf(dialog).style.translate).toBe("0 2.5rem");
  await user.pointer([
    { keys: "[TouchB>]", target, coords: { clientX: 200, clientY: 100 } },
    { pointerName: "TouchA", target, coords: { clientX: 100, clientY: 300 } },
  ]);
  expect(moved(groupOf(dialog))).toBe(false);
  await user.pointer([
    { keys: "[/TouchB]", target },
    { keys: "[/TouchA]", target },
  ]);

  expect(calls.closes).toBe(0);
});

test("with reduced motion nothing translates: the viewer fades by the share, and a release past the distance closes at once", async () => {
  reduceMotion(true);
  const { user, dialog, calls, picture } = await opened();
  const target = picture();

  await user.pointer([
    { keys: "[MouseLeft>]", target, coords: { clientX: 100, clientY: 100 } },
    { pointerName: "mouse", target, coords: { clientX: 100, clientY: 120 } },
    { pointerName: "mouse", target, coords: { clientX: 100, clientY: 100 + 3 * rem } },
  ]);
  expect(moved(groupOf(dialog))).toBe(false);
  expect(faded()).toContain(1 - swipeClose.dim * (3 / swipeClose.distance));
  await user.pointer([
    { pointerName: "mouse", target, coords: { clientX: 100, clientY: 100 + 8 * rem } },
    { keys: "[/MouseLeft]", target, coords: { clientX: 100, clientY: 100 + 8 * rem } },
  ]);

  expect(calls.closes).toBe(1);
  expect(screen.queryByRole("dialog")).toBeNull();
});
