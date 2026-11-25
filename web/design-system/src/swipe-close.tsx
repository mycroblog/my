// SPDX-License-Identifier: AGPL-3.0-only

// Closing a modal by a swipe down (the composer on a phone, the media viewer), by the tokens `swipeClose` and
// `transition`. The first `slop` decides: down more than across, it is a swipe to close for the rest of the touch;
// across or up first, it never is one, and a second finger ends it. The swiped surface follows the finger down at
// once, never above its place, and its backing gives up a share of its strength by the way (`dim`, all of it at
// `distance`), keeping the rest until the modal leaves. Let go `distance` down or more, or at a flick down of
// `velocity` over the drag's last `window` ms, it closes as its Close does (a flick up at the release keeps it); the
// surface goes on down out of the viewport from where it was let go, the backing fading out with it, as its modal
// leaves (modal.tsx). Else it springs back in a pop's time with the easing of what enters. With reduced motion the
// surface stays in place and fades by the share the backing keeps; let go, it is gone or whole again at once.
//
// The composer hears its touches here (`useSwipeClose`): a touch in what is written takes the gesture only while
// what scrolls there is at its top, with no text selected, when the first move past `slop` comes within `hold` ms of
// the touch, before a held finger would select; else the text scrolls and selects as ever. The listener of the moves
// is not passive and takes them over once the gesture is a swipe (`preventDefault`), so that the text keeps its
// scrolling and its `touch-action`. The media viewer feeds the same gesture from its own pointers (viewer.tsx).

import { swipeClose, transition } from "@my/design-tokens";
import { useEffect, useEffectEvent, useState, type Dispatch, type SetStateAction } from "react";

/** The lengths a swipe is measured by: rem, rem/s and ms (`swipeClose` of @my/design-tokens). */
export type SwipeMeasure = {
  readonly slop: number;
  readonly distance: number;
  readonly velocity: number;
  readonly window: number;
  readonly dim: number;
  readonly hold: number;
};

const swipeMeasure: SwipeMeasure = swipeClose;

/** What a swipe answers to the touches it is told of, in rem and ms. */
export type SwipeGesture = {
  /**
   * A touch began at `x`, `y` at `t`, with `touches` fingers on the screen; `held`: on what a held finger selects, so
   * that its first move past the slop must come within `hold` ms to be a swipe.
   */
  start: (x: number, y: number, t: number, touches: number, held?: boolean) => void;
  /** The finger moved: how far down the surface follows it, or undefined while it is no swipe (from then on, once not). */
  move: (x: number, y: number, t: number, touches: number) => number | undefined;
  /** The finger left the screen at `t`: whether the swipe closes. */
  end: (t: number) => boolean;
  /** The browser or the caller took the touch over: nothing is let go. */
  cancel: () => void;
};

/**
 * The swipe of one touch at a time, by `measure`: a swipe once the finger has gone `slop` and more down than across
 * (within `hold` ms of the touch where it is `held`), and from then on until it leaves; a touch that goes across or up
 * first, or a second finger, ends it, and nothing is swiped until the next touch.
 */
export function swipeGesture(measure: SwipeMeasure = swipeMeasure): SwipeGesture {
  let from: { x: number; y: number; t: number; held: boolean } | undefined;
  let swiping = false;
  let down = 0;
  let way: { t: number; y: number }[] = [];
  const stop = () => {
    from = undefined;
    swiping = false;
    down = 0;
    way = [];
  };
  return {
    start: (x, y, t, touches, held = false) => {
      stop();
      if (touches !== 1) return;
      from = { x, y, t, held };
      way = [{ t, y }];
    },
    move: (x, y, t, touches) => {
      if (from === undefined) return undefined;
      if (touches !== 1) {
        stop();
        return undefined;
      }
      const dy = y - from.y;
      if (!swiping) {
        const across = Math.abs(x - from.x);
        if (Math.max(across, Math.abs(dy)) < measure.slop) return undefined;
        if (dy <= across || (from.held && t - from.t > measure.hold)) {
          stop();
          return undefined;
        }
        swiping = true;
      }
      way = [...way.filter((p) => t - p.t <= measure.window), { t, y }];
      down = Math.max(0, dy);
      return down;
    },
    end: (t) => {
      const seen = way.filter((p) => t - p.t <= measure.window);
      const first = seen[0];
      const last = seen.at(-1);
      const speed = first && last && last.t > first.t ? ((last.y - first.y) / (last.t - first.t)) * 1000 : 0;
      const closes = swiping && ((down >= measure.distance && speed > -measure.velocity) || speed >= measure.velocity);
      stop();
      return closes;
    },
    cancel: stop,
  };
}

/** The share of its strength the backing keeps with the surface `down` rem down: never less than `1 - dim`. */
function swipeKept(down: number, measure: SwipeMeasure = swipeMeasure): number {
  return 1 - measure.dim * Math.min(1, Math.max(0, down) / measure.distance);
}

/**
 * Where a swipe stands: `rest` before any, `drag` while the finger holds the surface `down` rem down, `back` as it
 * springs back to its place, `leave` once let go to close, from `down`.
 */
export type SwipeState = { readonly phase: "rest" | "drag" | "back" | "leave"; readonly down: number };

export const swipeAtRest: SwipeState = { phase: "rest", down: 0 };

/** What a swipe sets on what it moves or fades: the properties a modal's own motion leaves alone. */
export type SwipeStyle = { readonly translate?: string; readonly filter?: string; readonly transition?: string };

const back = (property: string) => `${property} ${String(transition.duration.pop)}ms ${transition.ease.enter}`;

/**
 * The swiped surface's style: down with the finger by `translate` (which adds to the transform its modal moves it by),
 * back to its place in a pop's time; with reduced motion in place, faded by the share the backing keeps.
 */
export function swipeSurfaceStyle(at: SwipeState, reduced: boolean): SwipeStyle {
  if (at.phase === "rest") return {};
  if (reduced) return at.phase === "back" ? {} : { filter: `opacity(${String(swipeKept(at.down))})` };
  if (at.phase === "back") return { translate: "none", transition: back("translate") };
  return { translate: `0 ${String(at.down)}rem` };
}

/**
 * The style of what fades with the backing (the backdrop, the viewer's scrim and its buttons): the share it keeps, by
 * `filter`, which multiplies the opacity its modal moves it by; whole again in a pop's time.
 */
export function swipeBackingStyle(at: SwipeState, reduced: boolean): SwipeStyle {
  if (at.phase === "rest") return {};
  if (at.phase === "back") return reduced ? {} : { transition: back("filter") };
  return { filter: `opacity(${String(swipeKept(at.down))})` };
}

/** The root's font size in px, by which a touch's way is told in rem (a swipe's, a feed's pull's). */
export function remOf(el: Element): number {
  const view = el.ownerDocument.defaultView;
  const size = view ? Number.parseFloat(view.getComputedStyle(el.ownerDocument.documentElement).fontSize) : NaN;
  return Number.isFinite(size) && size > 0 ? size : 16;
}

/** Whether text is selected (a selection that is not collapsed, in the page or in the field with the focus). */
function selecting(doc: Document): boolean {
  const active = doc.activeElement;
  if (
    (active instanceof HTMLTextAreaElement || active instanceof HTMLInputElement) &&
    active.selectionStart !== active.selectionEnd
  )
    return true;
  const selection = doc.getSelection();
  return selection !== null && !selection.isCollapsed;
}

/** Whether anything from `target` up to `root` is scrolled away from its top. */
function scrolled(target: Element, root: Element): boolean {
  for (let el: Element | null = target; el !== null; el = el.parentElement) {
    if (el.scrollTop > 0) return true;
    if (el === root) break;
  }
  return false;
}

/**
 * A swipe down that closes, heard from the touches on the element `ref` is given: `starts` says where a touch may start
 * one, anywhere (`any`) or in what is written (`text`, by the stricter rules), and `on` whether one may start or close
 * now; `setAt` keeps where it stands (the state of the modal's owner, `swipeAtRest` to start), and `onClose` runs
 * as a release closes, in the same update as that turns `leave`. It returns the ref of the element.
 */
export function useSwipeClose({
  starts,
  on,
  onClose,
  setAt,
}: {
  starts: (target: Element) => "any" | "text" | undefined;
  on: () => boolean;
  onClose: () => void;
  setAt: Dispatch<SetStateAction<SwipeState>>;
}): (el: HTMLDivElement | null) => void {
  const [el, ref] = useState<HTMLDivElement | null>(null);
  const where = useEffectEvent(starts);
  const may = useEffectEvent(on);
  const close = useEffectEvent(onClose);
  useEffect(() => {
    if (!el) return;
    const gesture = swipeGesture();
    let rem = 16;
    let text = false;
    let taken = false;
    const springBack = () => {
      if (!taken) return;
      taken = false;
      setAt((now) => ({ phase: "back", down: now.down }));
    };
    const onStart = (e: TouchEvent) => {
      const touch = e.touches[0];
      springBack();
      gesture.cancel();
      if (!touch || e.touches.length !== 1 || !may() || !(e.target instanceof Element)) return;
      const kind = where(e.target);
      if (kind === undefined) return;
      text = kind === "text";
      if (text && (scrolled(e.target, el) || selecting(el.ownerDocument))) return;
      rem = remOf(el);
      gesture.start(touch.clientX / rem, touch.clientY / rem, e.timeStamp, 1, text);
    };
    const onMove = (e: TouchEvent) => {
      const touch = e.touches[0];
      const down = touch && gesture.move(touch.clientX / rem, touch.clientY / rem, e.timeStamp, e.touches.length);
      if (down === undefined) {
        springBack();
        return;
      }
      if (!taken) {
        // A scroll the browser began, or a selection made meanwhile, keeps the touch.
        if (!e.cancelable || (text && selecting(el.ownerDocument))) {
          gesture.cancel();
          return;
        }
        taken = true;
        // A press the touch began (a button in the head or the row) ends, as it does when the browser pans.
        e.target?.dispatchEvent(new PointerEvent("pointercancel", { bubbles: true }));
      }
      e.preventDefault();
      setAt({ phase: "drag", down });
    };
    const onEnd = (e: TouchEvent) => {
      const closes = gesture.end(e.timeStamp);
      if (!taken) return;
      if (e.cancelable) e.preventDefault();
      if (closes && may()) {
        taken = false;
        setAt((now) => ({ phase: "leave", down: now.down }));
        close();
      } else springBack();
    };
    const onCancel = () => {
      gesture.cancel();
      springBack();
    };
    el.addEventListener("touchstart", onStart, { passive: true });
    el.addEventListener("touchmove", onMove, { passive: false });
    el.addEventListener("touchend", onEnd, { passive: false });
    el.addEventListener("touchcancel", onCancel, { passive: true });
    return () => {
      el.removeEventListener("touchstart", onStart);
      el.removeEventListener("touchmove", onMove);
      el.removeEventListener("touchend", onEnd);
      el.removeEventListener("touchcancel", onCancel);
    };
  }, [el, setAt]);
  return ref;
}
