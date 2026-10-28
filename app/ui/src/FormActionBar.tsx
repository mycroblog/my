// SPDX-License-Identifier: AGPL-3.0-only
// The end of a form saved by its submit button (web-client/16-look-states-and-wording.md G24, 21-experience-principles.md
// P2): the buttons, and the result of the last submit right beside them, where the person pressed, rather than at the
// top of a long page. A success is a check and its words in a `role="status"` region, a failure a warning and its
// words in a `role="alert"` region; both regions are in the page, empty, from the first render, so that a screen
// reader reads the result when it arrives (a live region announces only what changes after it is there). The
// form's submit button keeps its own pending state (`Button`'s `isPending`).
import type { ReactNode } from "react";
import { tv } from "tailwind-variants";
import { CheckIcon, WarningIcon } from "./Icons";

/** The result of the last submit, already translated. */
export type FormActionResult = { tone: "success" | "danger"; message: string };

const formActionBar = tv({
  slots: {
    base: "flex flex-wrap items-center gap-x-4 gap-y-2",
    buttons: "flex flex-wrap items-center gap-2",
    // The result takes the room left beside the buttons; its words wrap there. The regions stay in the accessibility
    // tree while empty (not display: none), or a screen reader would not read what arrives in them.
    result: "min-w-0 flex-1",
    message: [
      "flex items-start gap-2 text-sm font-medium wrap-anywhere",
      "motion-safe:transition-[opacity,translate] motion-safe:duration-normal motion-safe:ease-enter",
      "motion-safe:starting:translate-y-1 motion-safe:starting:opacity-0",
    ],
    icon: "mt-0.5 shrink-0 [&>svg]:size-4",
  },
  variants: {
    tone: {
      success: { message: "text-success" },
      danger: { message: "text-danger" },
    },
  },
});

export type FormActionBarProps = {
  /** The submit button first (`Button type="submit"`), then at most one other action (Cancel). */
  children: ReactNode;
  /** The result of the last submit; nothing before the first one, and again once the form changes. */
  result?: FormActionResult | undefined;
};

/** The submit button of a form with the result of its last submit beside it. Put it last in the form. */
export function FormActionBar({ children, result }: FormActionBarProps) {
  const slots = formActionBar();
  const message = (tone: FormActionResult["tone"]) =>
    result?.tone === tone && (
      // A new result enters again (keyed by its words).
      <p key={result.message} className={slots.message({ tone })}>
        <span aria-hidden="true" className={slots.icon()}>
          {tone === "success" ? <CheckIcon /> : <WarningIcon filled />}
        </span>
        <span>{result.message}</span>
      </p>
    );
  return (
    <div className={slots.base()} data-form-action-bar="">
      <div className={slots.buttons()}>{children}</div>
      <div className={slots.result()}>
        <div role="status">{message("success")}</div>
        <div role="alert">{message("danger")}</div>
      </div>
    </div>
  );
}
