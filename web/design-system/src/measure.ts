// SPDX-License-Identifier: AGPL-3.0-only

// The times of the design system's interactions (ADR-0167 §2): it owns the kinds of change of screen (motion.tsx) and
// the presses of its buttons and its shortcuts, so it times them here, once for every part: a change of screen from
// its start to the next paint after the new screen is in place, named by its kind (`push`); a press from the input to
// the next paint, named by the action ID its shortcut has (`post.reply`, what `useAction` returns as `data-action`).
// The parts write no code for it. Where the samples go is the app's (`measureWith`); without it nothing is timed.

import type { TransitionKind } from "./motion";

/** One sample of the design system: a change of screen by its kind, or a press by its action ID, in milliseconds. */
export type InteractionSample =
  { metric: "transition"; name: TransitionKind; value: number } | { metric: "press"; name: string; value: number };

let sink: ((sample: InteractionSample) => void) | null = null;

/** Gives every sample to `report` until the function it returns is called. */
export function measureWith(report: (sample: InteractionSample) => void): () => void {
  sink = report;
  return () => {
    if (sink === report) sink = null;
  };
}

const now = () => performance.now();

/** Calls `then` after the next paint: past the frame that draws what the current task changed. */
function afterNextPaint(then: () => void): void {
  const later = () => setTimeout(then, 0);
  if (typeof requestAnimationFrame === "function") requestAnimationFrame(later);
  else later();
}

/** Reports a sample from `start` to the next paint, if the samples go somewhere. */
function untilPaint(make: (value: number) => InteractionSample, start: number): void {
  const report = sink;
  if (report === null) return;
  afterNextPaint(() => {
    if (sink === report) report(make(Math.max(0, now() - start)));
  });
}

/**
 * Times the press of the action `action`, from `start` (an input's timeStamp; now by default) to the next paint after
 * what it changed.
 */
export function timePress(action: string, start: number = now()): void {
  untilPaint((value) => ({ metric: "press", name: action, value }), start);
}

/**
 * Times a change of screen of `kind` from now: the function it returns ends it, once the new screen is in place, and
 * its sample follows the next paint. Without a destination it times nothing.
 */
export function timeTransition(kind: TransitionKind): () => void {
  if (sink === null) return () => undefined;
  const start = now();
  let ended = false;
  return () => {
    if (ended) return;
    ended = true;
    untilPaint((value) => ({ metric: "transition", name: kind, value }), start);
  };
}
