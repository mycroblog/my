// SPDX-License-Identifier: AGPL-3.0-only
// How a feature tells the person what happened (web-client/16-look-states-and-wording.md G25, 21-experience-principles.md
// P2): never by the screen reader alone. Each call says where the result shows, and the announcement goes with it:
// - `inPlace`: the component the person acted on, or the one the change concerns, shows the result itself (a save
//   mark, the list that changed, a button that switched, the notice of a call): this reads it out.
// - `toast`: a toast shows and reads it out, for a result with no place of its own on the screen.
// A feature calls this instead of `useAnnouncer` (Oxlint, `restrict/feature`), so that each announcement names its
// visible place.
import { use } from "react";
import { type Politeness, useAnnouncer } from "./Announcer";
import { ToastContext, type ToastOptions } from "./Toast";

export type Notice = {
  /**
   * Reads `message` out for a result that the screen already shows where it happened. `assertive` interrupts, for
   * a failure or a change that stops what the person was doing.
   */
  inPlace: (message: string, politeness?: Politeness) => void;
  /** Shows `options` as a toast, which reads it out, and returns its id. Needs a ToastProvider. */
  toast: (options: ToastOptions) => number;
};

/** Returns the notice functions. Call them from event handlers and mutation callbacks, not during rendering. */
export function useNotice(): Notice {
  const announce = useAnnouncer();
  const toasts = use(ToastContext);
  return {
    inPlace: (message, politeness) => {
      announce(message, politeness);
    },
    toast: (options) => {
      if (toasts === null) throw new Error("useNotice().toast needs a <ToastProvider> above it");
      return toasts.show(options);
    },
  };
}
