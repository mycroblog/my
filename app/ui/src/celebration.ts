// SPDX-License-Identifier: AGPL-3.0-only
import { useEffect, useState } from "react";

/**
 * How long an element stays `data-celebrating` after `celebrate()`: the bounce (`duration-slow`) and a moment to see the
 * tint that replaces it under reduced motion.
 */
export const celebrationMs = 600;

/**
 * The one micro-interaction of a celebrated moment (web-client/21-experience-principles.md §21.5): while the element
 * carries `data-celebrating`, its icon (a child `svg`) grows in with the little overshoot of `ease-celebrate`. With
 * reduced motion nothing moves; the element takes the `soft` fill instead, on top of the change of shape and color that
 * the moment itself shows (a filled star). Put these classes on the control and spread `celebrationProps` on it.
 */
export const celebrationClasses = [
  "motion-safe:data-celebrating:[&>svg]:animate-celebrate",
  "motion-reduce:data-celebrating:bg-soft motion-reduce:data-celebrating:data-hovered:bg-soft",
].join(" ");

export type Celebration = {
  /** Starts the moment: call it when the person's own action has just succeeded or been made, never on load. */
  celebrate: () => void;
  /** Ends it early, such as when the person undoes the action. */
  stop: () => void;
  isCelebrating: boolean;
  /** Spread on the celebrating element; an empty object outside the moment. */
  celebrationProps: { "data-celebrating"?: true };
};

/**
 * The state of a celebrated moment: `celebrate()` marks the element for `celebrationMs`. Only the moments listed in
 * §21.5 use it; every other reaction stays quiet (`duration-fast` / `duration-normal`, no overshoot).
 */
export function useCelebration(): Celebration {
  const [isCelebrating, setIsCelebrating] = useState(false);
  // Each call restarts the timer, so pressing again within the moment keeps it until the last press is done.
  const [round, setRound] = useState(0);
  useEffect(() => {
    if (!isCelebrating) return;
    const timer = setTimeout(() => {
      setIsCelebrating(false);
    }, celebrationMs);
    return () => {
      clearTimeout(timer);
    };
  }, [isCelebrating, round]);
  const celebrate = () => {
    setIsCelebrating(true);
    setRound((n) => n + 1);
  };
  const stop = () => {
    setIsCelebrating(false);
  };
  return {
    celebrate,
    stop,
    isCelebrating,
    celebrationProps: isCelebrating ? { "data-celebrating": true } : {},
  };
}
