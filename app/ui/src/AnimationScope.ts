// SPDX-License-Identifier: AGPL-3.0-only
// A part of a page whose animations play together while animations do not play by themselves (ADR-0054 §3): a post or a
// profile's header. Its avatar, custom emoji and animated images move while the pointer is over it, while keyboard focus
// is inside it, and from a tap on it until another tap or until it leaves the screen. They still never move while the
// system asks for reduced motion: the scope only lets the moving renditions in, behind the same media query.
import { type FocusEvent, type PointerEvent, type RefObject, use, useEffect, useRef, useState } from "react";
import { AnimationsPlayContext, AutoplayContext } from "./Picture";

/** How far a finger may move between down and up, in CSS pixels, for a tap rather than a drag or a scroll. */
const tapSlop = 10;

/** How long a finger may rest, in milliseconds, for a tap rather than a long press (as react-aria's `useLongPress`). */
const longPress = 500;

/**
 * The controls inside a scope, which do something of their own: a tap on one of them is that control's and does not
 * toggle the scope (a container never takes over the activation of its controls).
 */
const interactive = [
  "a[href]",
  "button",
  "input",
  "select",
  "textarea",
  "summary",
  "label",
  "video",
  "audio",
  "[contenteditable]:not([contenteditable='false'])",
  "[tabindex]:not([tabindex='-1'])",
  "[role='button']",
  "[role='link']",
  "[role='checkbox']",
  "[role='switch']",
  "[role='menuitem']",
  "[role='option']",
  "[role='tab']",
  "[role='slider']",
].join(", ");

/** Whether `target` is, or is inside, a control below `scope` (not the scope itself, which may be focusable). */
function onControl(target: EventTarget, scope: Element): boolean {
  if (!(target instanceof Element)) return false;
  const control = target.closest(interactive);
  return control !== null && control !== scope && scope.contains(control);
}

/** Whether a focus came from the keyboard: the platform shows a focus ring (`:focus-visible`) only then. */
function focusVisible(element: Element): boolean {
  try {
    return element.matches(":focus-visible");
  } catch {
    // A browser without `:focus-visible` counts every focus.
    return true;
  }
}

/** The ref and the handlers the element of an animation scope takes. */
export type AnimationScopeProps<T extends HTMLElement> = {
  ref: RefObject<T | null>;
  onPointerEnter: (event: PointerEvent<T>) => void;
  onPointerLeave: (event: PointerEvent<T>) => void;
  onPointerDown: (event: PointerEvent<T>) => void;
  onPointerUp: (event: PointerEvent<T>) => void;
  onPointerCancel: (event: PointerEvent<T>) => void;
  onFocus: (event: FocusEvent<T>) => void;
  onBlur: (event: FocusEvent<T>) => void;
};

/** Where and when a finger went down on the scope. */
type Press = { id: number; x: number; y: number; time: number };

/**
 * Makes an element an animation scope. Spread `props` on the element (chaining any handlers of its own) and provide
 * `playing` to its content with `<AnimationsPlayContext value={playing}>`; `Picture`, `CustomEmoji` and the looping
 * videos of `MediaGrid` read it. It adds no focus stop and no role: the element stays what it is. A scope inside a playing
 * one plays too. While animations play by themselves (`AutoplayContext`), it does nothing.
 */
export function useAnimationScope<T extends HTMLElement>(): { playing: boolean; props: AnimationScopeProps<T> } {
  const autoplay = use(AutoplayContext);
  const outer = use(AnimationsPlayContext);
  const [hovered, setHovered] = useState(false);
  const [focused, setFocused] = useState(false);
  const [tapped, setTapped] = useState(false);
  const elementRef = useRef<T | null>(null);
  const pressRef = useRef<Press | undefined>(undefined);

  // A tapped scope stops once it is off the screen, as when a scroll moves it away.
  useEffect(() => {
    const target = elementRef.current;
    if (!tapped || target === null || typeof IntersectionObserver === "undefined") return;
    const observer = new IntersectionObserver((entries) => {
      if (entries.some((entry) => !entry.isIntersecting)) setTapped(false);
    });
    observer.observe(target);
    return () => {
      observer.disconnect();
    };
  }, [tapped]);

  const props: AnimationScopeProps<T> = {
    ref: elementRef,
    onPointerEnter: (event) => {
      if (!autoplay && event.pointerType !== "touch") setHovered(true);
    },
    onPointerLeave: (event) => {
      if (event.pointerType !== "touch") setHovered(false);
    },
    onPointerDown: (event) => {
      pressRef.current =
        !autoplay && event.pointerType === "touch" && event.isPrimary
          ? { id: event.pointerId, x: event.clientX, y: event.clientY, time: event.timeStamp }
          : undefined;
    },
    onPointerUp: (event) => {
      const start = pressRef.current;
      pressRef.current = undefined;
      if (start === undefined || start.id !== event.pointerId) return;
      // A drag, a scroll that the browser did not take, or a long press (which opens the system's menu) is not a tap.
      if (
        Math.hypot(event.clientX - start.x, event.clientY - start.y) > tapSlop ||
        event.timeStamp - start.time >= longPress
      )
        return;
      if (onControl(event.target, event.currentTarget)) return;
      setTapped((on) => !on);
    },
    onPointerCancel: () => {
      // The browser took the touch for a scroll or a zoom.
      pressRef.current = undefined;
    },
    onFocus: (event) => {
      if (!autoplay && focusVisible(event.target)) setFocused(true);
    },
    onBlur: (event) => {
      if (!(event.relatedTarget instanceof Node && event.currentTarget.contains(event.relatedTarget)))
        setFocused(false);
    },
  };
  return { playing: outer || (!autoplay && (hovered || focused || tapped)), props };
}
