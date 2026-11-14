// SPDX-License-Identifier: AGPL-3.0-only

// A modal over the page (ADR-0025 §1, §4): the composer and the keyboard shortcuts' help. Its backdrop dims the page
// with the scrim and blurs it by `--dialog-blur`, so that the reader's eye stays on the modal (the dim alone for a reader
// who asks for less transparency); its panel rises in. Motion moves both from one state the overlay holds (`hidden`,
// then `shown`; `hidden` again as it leaves): the backdrop fades in its blurred dim by opacity alone, and the panel
// rises by opacity and a transform, in a modal's time and easing, from the same frame. A modal leaves the same way back,
// in the easing of what leaves, and unmounts when it ends; it leaves only where its caller renders it in a
// `ModalPresence`. With reduced motion it is there at once and gone at once.
//
// It is React Aria's modal, built from its hooks: React Aria Components' `ModalOverlay` is not used, because it mounts
// its overlay hidden (`useEnterAnimation`: opacity 0, clipped) and reveals it through `runAfterKeyboard`, which, on a
// device with a touch screen (`navigator.maxTouchPoints > 0`) and focus in a text field, waits for a software keyboard
// to open, up to 600 ms on a PC that never opens one. The composer focuses its text at once, inside the press, so that
// a phone opens its keyboard, and lays itself out above it (`--keyboard-inset`); a modal here shows on the frame it
// mounts in. A press outside its panel dismisses it, as Escape does unless the caller handles Escape itself; focus
// stays in it and goes back where it was; the page behind is inert.

import { transition } from "@my/design-tokens";
import {
  AnimatePresence,
  domAnimation,
  LazyMotion,
  m,
  useIsPresent,
  useReducedMotion,
  type Variants,
} from "motion/react";
import { useRef, type ReactNode } from "react";
import { DismissButton, Overlay } from "react-aria/Overlay";
import { useModalOverlay } from "react-aria/useModalOverlay";
import { OverlayTriggerStateContext } from "react-aria-components";

/** The control points of a `cubic-bezier()` easing token. */
function curve(easing: string): [number, number, number, number] {
  const [a = 0, b = 0, c = 1, d = 1] = (easing.match(/-?\d*\.?\d+/g) ?? []).map(Number);
  return [a, b, c, d];
}

const enter = { duration: transition.duration.modal / 1000, ease: curve(transition.ease.enter) };
const leave = { duration: transition.duration.modal / 1000, ease: curve(transition.ease.exit) };

/** The two states of a modal, as each of its elements reads them; the overlay holds which one it is in. */
export const modalMotion = {
  backdrop: { hidden: { opacity: 0, transition: leave }, shown: { opacity: 1, transition: enter } },
  panel: {
    hidden: { opacity: 0, y: `${String(transition.rise)}rem`, transition: leave },
    shown: { opacity: 1, y: 0, transition: enter },
  },
} as const satisfies Record<string, Variants>;

/** The state the overlay starts in, goes to and leaves by: none to move through with reduced motion. */
export function modalStates(reduced: boolean) {
  return reduced
    ? ({ initial: false, animate: "shown" } as const)
    : ({ initial: "hidden", animate: "shown", exit: "hidden" } as const);
}

/** The backdrop: at every width, or from `tablet`. */
export const modalBackdrop = {
  always:
    "fixed inset-0 bg-scrim backdrop-blur-[var(--dialog-blur)] [@media(prefers-reduced-transparency:reduce)]:backdrop-blur-none",
  // Under `tablet` the modal is a screen of its own, with nothing to dim behind it.
  tablet:
    "fixed inset-0 bg-scrim backdrop-blur-[var(--dialog-blur)] [@media(prefers-reduced-transparency:reduce)]:backdrop-blur-none max-tablet:hidden",
};

/** Lets a modal rendered in it leave by its motion before it unmounts: wrap where the modal is rendered or not. */
export function ModalPresence({ children }: { children: ReactNode }) {
  return <AnimatePresence>{children}</AnimatePresence>;
}

export type ModalLayerProps = {
  /** A press outside the panel, and Escape unless `ownEscape`. */
  onDismiss: () => void;
  /** The caller handles Escape itself (the composer, which tells leaving on purpose from by accident). */
  ownEscape?: boolean | undefined;
  /** The backdrop at every width, or from `tablet`, under which the panel is a screen of its own. */
  backdrop?: "always" | "tablet" | undefined;
  /** The layout of the overlay over the viewport, which places the panel and scrolls. */
  overlay: string;
  /** The panel's box. */
  panel: string;
  children: ReactNode;
};

/** A modal over the page: its backdrop and its panel, shown on the frame it mounts in. */
export function ModalLayer({
  onDismiss,
  ownEscape = false,
  backdrop: dim = "always",
  overlay,
  panel,
  children,
}: ModalLayerProps) {
  const ref = useRef<HTMLDivElement>(null);
  const present = useIsPresent();
  const reduced = useReducedMotion() ?? false;
  const state = {
    isOpen: present,
    setOpen: (open: boolean) => {
      if (!open) onDismiss();
    },
    open: () => undefined,
    close: onDismiss,
    toggle: onDismiss,
    point: null,
    setPoint: () => undefined,
  };
  const { modalProps } = useModalOverlay({ isDismissable: true, isKeyboardDismissDisabled: ownEscape }, state, ref);
  return (
    <Overlay isExiting={!present}>
      <LazyMotion features={domAnimation} strict>
        <m.div className={present ? overlay : `${overlay} pointer-events-none`} {...modalStates(reduced)}>
          <m.div className={modalBackdrop[dim]} variants={modalMotion.backdrop} />
          <div {...modalProps} ref={ref} className="contents">
            <DismissButton onDismiss={onDismiss} />
            <m.div className={panel} variants={modalMotion.panel}>
              <OverlayTriggerStateContext value={state}>{children}</OverlayTriggerStateContext>
            </m.div>
          </div>
        </m.div>
      </LazyMotion>
    </Overlay>
  );
}
