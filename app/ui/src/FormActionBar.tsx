// SPDX-License-Identifier: AGPL-3.0-only
// The end of a form saved by its submit button (web-client/16-look-states-and-wording.md G24, 21-experience-principles.md
// P2): the buttons, and the result of the last submit right beside them, where the person pressed, rather than at the
// top of a long page. A success is a check and its words in a `role="status"` region, a failure a warning and its
// words in a `role="alert"` region; both regions are in the page, empty, from the first render, so that a screen
// reader reads the result when it arrives (a live region announces only what changes after it is there). The
// form's submit button keeps its own pending state (`Button`'s `isPending`). In a long form, `sticky` keeps the bar at the
// bottom edge of the screen while the form is in view (ADR-0110 §5), so the button and its result stay in sight from
// any field.
import type { ReactNode } from "react";
import { tv } from "tailwind-variants";
import { ActionResult, type ActionResultValue } from "./ActionResult";

/** The result of the last submit, already translated. */
export type FormActionResult = ActionResultValue;

const formActionBar = tv({
  slots: {
    base: "flex flex-wrap items-center gap-x-4 gap-y-2",
    buttons: "flex flex-wrap items-center gap-2",
    // The result takes the room left beside the buttons; its words wrap there.
    result: "min-w-0 flex-1",
  },
  variants: {
    // The bar stays above what the app shell fixes at the bottom of the screen: the app's stylesheet sets
    // --shell-inset-block-end to the tab bar below md and to the call bar while a call shows (a component asks no
    // viewport, web-client/README.md D2). On short screens (a phone in landscape, 400% zoom) it does not stick, so
    // that what is fixed stays within half the screen's height (WCAG 1.4.10). The raised surface and the rule above it
    // keep the fields scrolling under it out of sight.
    sticky: {
      true: {
        base: [
          "sticky z-10 border-t border-border bg-surface-raised py-3",
          "bottom-(--shell-inset-block-end,0px) short:static",
        ],
      },
      false: {},
    },
  },
});

export type FormActionBarProps = {
  /** The submit button first (`Button type="submit"`), then at most one other action (Cancel). */
  children: ReactNode;
  /** The result of the last submit; nothing before the first one, and again once the form changes. */
  result?: FormActionResult | undefined;
  /** Keeps the bar at the bottom edge of the screen while the form scrolls: for a form taller than the screen. */
  sticky?: boolean;
};

/** The submit button of a form with the result of its last submit beside it. Put it last in the form. */
export function FormActionBar({ children, result, sticky = false }: FormActionBarProps) {
  const slots = formActionBar({ sticky });
  return (
    <div className={slots.base()} data-form-action-bar="" data-sticky={sticky ? "" : undefined}>
      <div className={slots.buttons()}>{children}</div>
      <div className={slots.result()}>
        <ActionResult result={result} />
      </div>
    </div>
  );
}
