// SPDX-License-Identifier: AGPL-3.0-only
// The save state of a control that saves itself as soon as it changes (web-client/16-look-states-and-wording.md G24,
// 21-experience-principles.md P2): next to the control, a turning ring and "Saving…" while the change is on its way, a
// check and "Saved" once it is kept, a warning and "Not saved" when it failed (the control shows why). Each state comes
// in with a quick fade and grow under `motion-safe:` (`duration-normal`, `ease-enter`, no overshoot); with reduced
// motion the icon's shape and color and the words tell the same.
//
// It is not a live region: the hook that saves reads the result out once through the Announcer (`useNotice`'s
// `inPlace`), so that nothing is read twice; a screen reader that browses the page still finds the words here.
import type { ReactNode } from "react";
import { tv } from "tailwind-variants";
import { CheckIcon, WarningIcon } from "./Icons";
import { VisuallyHidden } from "./VisuallyHidden";

/** Where a change of a control that saves itself is: none yet, on its way, kept, or not kept. */
export type SaveState = "idle" | "saving" | "saved" | "failed";

/** The words of each state, already translated ("Saving…", "Saved", "Not saved"). */
export type SaveIndicatorLabels = { saving: string; saved: string; failed: string };

const saveIndicator = tv({
  slots: {
    base: [
      "inline-flex shrink-0 items-center gap-1 text-xs font-medium",
      // Each state enters on its own (the element is keyed by the state): quiet and quick, no overshoot (P2).
      "motion-safe:transition-[opacity,scale] motion-safe:duration-normal motion-safe:ease-enter",
      "motion-safe:starting:scale-75 motion-safe:starting:opacity-0",
    ],
    icon: "inline-grid size-4 shrink-0 place-items-center [&>svg]:size-4",
    // The ring of Spinner, decorative here: the words name the state.
    ring: "size-3.5 rounded-full border-2 border-border border-t-current motion-safe:animate-spin",
  },
  variants: {
    state: {
      idle: {},
      saving: { base: "text-fg-muted", icon: "text-accent" },
      saved: { base: "text-success" },
      failed: { base: "text-danger" },
    },
  },
});

export type SaveIndicatorProps = {
  /** The state of the last change; `idle` shows nothing. */
  state: SaveState;
  labels: SaveIndicatorLabels;
  /** `text` (default) shows the icon with the words; `icon` keeps the words for assistive technology only, in a table's cell. */
  display?: "text" | "icon";
};

/** The mark of a save state, next to its control. `AutosaveControl` places it; a table's cell places it itself. */
export function SaveIndicator({ state, labels, display = "text" }: SaveIndicatorProps) {
  if (state === "idle") return null;
  const slots = saveIndicator({ state });
  const icon =
    state === "saving" ? <span className={slots.ring()} /> : state === "saved" ? <CheckIcon /> : <WarningIcon filled />;
  const words = labels[state];
  return (
    <span key={state} data-save-state={state} className={slots.base()}>
      <span aria-hidden="true" className={slots.icon()}>
        {icon}
      </span>
      {display === "text" ? <span>{words}</span> : <VisuallyHidden>{words}</VisuallyHidden>}
    </span>
  );
}

export type AutosaveControlProps = {
  /** The state of the control's last change, from the hook that saves it: a control that saves itself must take one. */
  state: SaveState;
  labels: SaveIndicatorLabels;
  /** The control (a `Switch`, a `Select`, a `RadioGroup`, a group of checkboxes) with its label and its error. */
  children: ReactNode;
};

/**
 * A control that saves itself as soon as it changes, with its save state at its inline end, level with its label: the
 * control takes the room left, and the mark takes only its own width while it shows.
 */
export function AutosaveControl({ state, labels, children }: AutosaveControlProps) {
  return (
    <div className="flex items-start gap-3" data-autosave="">
      <div className="min-w-0 flex-1">{children}</div>
      <SaveIndicator state={state} labels={labels} />
    </div>
  );
}
