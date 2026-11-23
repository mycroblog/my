// SPDX-License-Identifier: AGPL-3.0-only

// A modal over the page (ADR-0025 §1, §4): the composer and the keyboard shortcuts' help. Its backdrop dims the page
// with the scrim and blurs it by `--dialog-blur`, so that the reader's eye stays on the modal (the dim alone for a reader
// who asks for less transparency); its panel rises in. Motion moves both from one state the overlay holds (`hidden`,
// then `shown`; `hidden` again as it leaves): the backdrop fades in its blurred dim by opacity alone, and the panel
// rises by opacity and a transform, in a modal's time and easing, from the same frame. A modal leaves the same way back,
// in the easing of what leaves, and unmounts when it ends; it leaves only where its caller renders it in a
// `ModalPresence`. With reduced motion it is there at once and gone at once. Motion runs with the features of its
// animations alone (`domMin`), none of its gestures, layout or drag, which the modals do not use.
//
// It is React Aria's modal, built from its hooks: React Aria Components' `ModalOverlay` is not used, because it mounts
// its overlay hidden (`useEnterAnimation`: opacity 0, clipped) and reveals it through `runAfterKeyboard`, which, on a
// device with a touch screen (`navigator.maxTouchPoints > 0`) and focus in a text field, waits for a software keyboard
// to open, up to 600 ms on a PC that never opens one. The composer focuses its text at once, inside the press, so that
// a phone opens its keyboard, and lays itself out above it (`--keyboard-inset`); a modal here shows on the frame it
// mounts in. A press outside its panel dismisses it, as Escape does unless the caller handles Escape itself; focus
// stays in it and goes back where it was; the page behind is inert.
//
// Where a modal stands is its frame's alone (`ModalFrame`, with `ModalPanel`): a modal says its place, and writes its
// surface and what it holds. Under `tablet` a modal at `screen` is a screen of its own, so the frame is one of the
// frames of a page, which read the viewport.

import { transition } from "@my/design-tokens";
import { AnimatePresence, domMin, LazyMotion, m, useIsPresent, useReducedMotion, type Variants } from "motion/react";
import { useRef, type ReactNode } from "react";
import { DismissButton, Overlay } from "react-aria/Overlay";
import { useModalOverlay } from "react-aria/useModalOverlay";
import { OverlayTriggerStateContext } from "react-aria-components";

/** The control points of a `cubic-bezier()` easing token, as Motion takes them. */
export function curve(easing: string): [number, number, number, number] {
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

/** The backdrop: at every width, or from `tablet`; or dimmed alone, unblurred, at every width. */
const modalBackdrop = {
  // A sheet that keeps what it writes into in sight (the emoji picker's).
  dim: "fixed inset-0 bg-scrim",
  always:
    "fixed inset-0 bg-scrim backdrop-blur-[var(--dialog-blur)] [@media(prefers-reduced-transparency:reduce)]:backdrop-blur-none",
  // Under `tablet` the modal is a screen of its own: under it the screen's surface, with nothing of the page showing
  // around it (behind the translucent bar an installed app's software keyboard has over the page on iOS).
  tablet: [
    "fixed inset-0 bg-scrim backdrop-blur-[var(--dialog-blur)] [@media(prefers-reduced-transparency:reduce)]:backdrop-blur-none",
    "max-tablet:bg-surface max-tablet:backdrop-blur-none",
  ].join(" "),
};

/**
 * Lets a modal rendered in it leave by its motion before it unmounts: wrap where the modal is rendered or not.
 * `onExited` runs once one has left.
 */
export function ModalPresence({ onExited, children }: { onExited?: (() => void) | undefined; children: ReactNode }) {
  return (
    <AnimatePresence {...(onExited === undefined ? {} : { onExitComplete: onExited })}>{children}</AnimatePresence>
  );
}

/**
 * Where a modal's panel stands over the viewport:
 * - `center`: in its middle, a dialog's most width at most, a margin from its edges (a confirmation).
 * - `sheet`: on its bottom edge at its full width, room above it showing the page; `sheet-above-keyboard` keeps its
 *   bottom above the software keyboard where one is open.
 * - `screen`: under `tablet` the whole of it, a screen of its own; from `tablet` at its top, in its middle, the room
 *   of the composer above and below, the overlay scrolling with what the panel holds.
 * - `cover`: the whole of it at every width (the media viewer).
 */
type ModalPlace = "center" | "sheet" | "sheet-above-keyboard" | "screen" | "cover";

const sheetOverlay = "fixed inset-0 z-50 box-border flex flex-col justify-end pt-shell-sheet-room";
const OVERLAY: Record<ModalPlace, string> = {
  center: "fixed inset-0 z-50 box-border flex items-center justify-center p-dialog-margin",
  sheet: sheetOverlay,
  "sheet-above-keyboard": `${sheetOverlay} pb-[var(--keyboard-inset,env(keyboard-inset-height,0px))]`,
  screen: [
    "fixed inset-0 z-50 box-border grid items-start overflow-y-auto",
    "tablet:justify-items-center tablet:px-dialog-margin tablet:pt-composer-top tablet:pb-composer-bottom",
  ].join(" "),
  cover: "fixed inset-0 z-50",
};

// Positioned, so that the panel paints over the fixed backdrop before it in the overlay.
const sheetPanel = "relative flex min-h-0 w-full flex-col";
const PANEL: Record<ModalPlace, string> = {
  center: "relative box-border w-full max-w-[var(--dialog-max-width)]",
  sheet: sheetPanel,
  "sheet-above-keyboard": sheetPanel,
  screen: "box-border w-full",
  cover: "size-full",
};

/**
 * The frame of a modal: the overlay over the viewport, which places the panel by `place` and lays the backdrop over
 * the page behind it. Motion moves it through `states`; `idle`, it takes no pointer.
 */
function ModalFrame({
  place,
  backdrop,
  idle,
  states,
  children,
}: {
  place: ModalPlace;
  backdrop: keyof typeof modalBackdrop;
  idle: boolean;
  states: ReturnType<typeof modalStates>;
  children: ReactNode;
}) {
  return (
    <m.div className={idle ? `${OVERLAY[place]} pointer-events-none` : OVERLAY[place]} {...states}>
      <m.div className={modalBackdrop[backdrop]} variants={modalMotion.backdrop} />
      {children}
    </m.div>
  );
}

/** The panel of a modal, where `place` puts it in its frame, on the surface its modal gives it (`className`). */
function ModalPanel({
  place,
  className,
  children,
}: {
  place: ModalPlace;
  className: string | undefined;
  children: ReactNode;
}) {
  return (
    <m.div
      className={className === undefined ? PANEL[place] : `${className} ${PANEL[place]}`}
      variants={modalMotion.panel}
    >
      {children}
    </m.div>
  );
}

export type ModalLayerProps = {
  /** A press outside the panel, and Escape unless `ownEscape`. */
  onDismiss: () => void;
  /** The caller handles Escape itself (the composer, which tells leaving on purpose from by accident). */
  ownEscape?: boolean | undefined;
  /**
   * The backdrop at every width, or from `tablet`, under which the panel is a screen of its own on the screen's
   * surface; or `dim`, at every width with no blur.
   */
  backdrop?: "always" | "tablet" | "dim" | undefined;
  /** Where the panel stands over the viewport. */
  place: ModalPlace;
  /** The panel's own surface: its paint, its padding. */
  panelClassName?: string | undefined;
  /** It takes no pointer yet (a sheet whose opening press has not ended). */
  idle?: boolean | undefined;
  children: ReactNode;
};

/** A modal over the page: its backdrop and its panel, shown on the frame it mounts in. */
export function ModalLayer({
  onDismiss,
  ownEscape = false,
  backdrop: dim = "always",
  place,
  panelClassName,
  idle = false,
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
      <LazyMotion features={domMin} strict>
        <ModalFrame place={place} backdrop={dim} idle={idle || !present} states={modalStates(reduced)}>
          <div {...modalProps} ref={ref} className="contents">
            <DismissButton onDismiss={onDismiss} />
            <ModalPanel place={place} className={panelClassName}>
              <OverlayTriggerStateContext value={state}>{children}</OverlayTriggerStateContext>
            </ModalPanel>
          </div>
        </ModalFrame>
      </LazyMotion>
    </Overlay>
  );
}
