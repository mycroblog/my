// SPDX-License-Identifier: AGPL-3.0-only
// A change that someone or something else made to the person, shown where it concerns them
// (web-client/16-look-states-and-wording.md G25): "The host turned your microphone off", "The room ended", on the dock
// or the mini-player of a call. An icon, the words, an optional follow-up action, and a button that dismisses it. It
// enters with a quick fade under `motion-safe:`; key it by its words, so that a new change enters again.
//
// It is not a live region: the hook that learns of the change reads it out once (`useNotice`'s `inPlace`), at the
// same time as this shows it.
import type { ReactNode } from "react";
import { tv } from "tailwind-variants";
import { IconButton } from "./IconButton";
import { AboutIcon, CloseIcon, WarningIcon } from "./Icons";

const changeNotice = tv({
  slots: {
    base: [
      "flex items-start gap-2 rounded-md bg-surface-sunken py-1 ps-3 pe-1 text-sm text-fg",
      "motion-safe:transition-[opacity,translate] motion-safe:duration-normal motion-safe:ease-enter",
      "motion-safe:starting:translate-y-1 motion-safe:starting:opacity-0",
    ],
    // The tone shows by this icon's shape as well as its color (WCAG 1.4.1).
    icon: "mt-2 shrink-0 [&>svg]:size-4",
    body: "flex min-w-0 flex-1 flex-wrap items-center gap-x-3 gap-y-1 py-1.5",
    message: "min-w-0 wrap-anywhere",
  },
  variants: {
    tone: {
      neutral: { icon: "text-fg-muted" },
      warning: { icon: "text-warning" },
    },
  },
  defaultVariants: { tone: "neutral" },
});

export type ChangeNoticeProps = {
  /** What changed, already translated. */
  children: ReactNode;
  /** `warning` for a change that stopped something (the call ended); `neutral` (default) for the rest. */
  tone?: "neutral" | "warning";
  /** A follow-up, such as a link back to the room, after the words. */
  action?: ReactNode;
  /** The name of the dismiss button ("Dismiss"). */
  dismissLabel: string;
  onDismiss: () => void;
};

/** The notice of a change made from elsewhere, with its dismiss button. */
export function ChangeNotice({ children, tone, action, dismissLabel, onDismiss }: ChangeNoticeProps) {
  const slots = changeNotice({ tone });
  return (
    <div className={slots.base()} data-change-notice="">
      <span aria-hidden="true" className={slots.icon()}>
        {tone === "warning" ? <WarningIcon filled /> : <AboutIcon />}
      </span>
      <div className={slots.body()}>
        <p className={slots.message()}>{children}</p>
        {action}
      </div>
      <IconButton label={dismissLabel} onPress={onDismiss}>
        <CloseIcon />
      </IconButton>
    </div>
  );
}
