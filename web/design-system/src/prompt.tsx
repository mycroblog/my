// SPDX-License-Identifier: AGPL-3.0-only

// A prompt inside a screen, not over it (the Web Push prompt over the notifications, ADR-0086 §18–§19): on the raised
// surface, no title, a glyph and one line or a short text of what it offers, and under it the action that does it, the
// primary button, beside a quiet one that dismisses it. A reason under the text, at the text's measure and in a
// caption's size, says why the action did not work (the reader closed the browser's question, the subscription
// failed), announced as it comes; when nothing is left to try, the prompt keeps only its dismiss. It is named by its
// text for assistive technology.

import { useId, type ReactNode } from "react";
import { tv } from "tailwind-variants/lite";
import { Button } from "./button";
import { Icon, type IconName } from "./icons";

const prompt = tv({
  slots: {
    // A container, so that the reason takes the panel's measure where the panel is wide.
    root: "@container rounded-overlay bg-surface-raised px-embed py-shell-inset text-body text-text",
    row: "flex items-start gap-entry-avatar",
    glyph: "inline-flex h-[1lh] flex-none items-center",
    body: "grid min-w-0 flex-1 gap-shell-inset",
    // A text that wraps breaks between phrases, never inside a word of ja.
    text: "m-0 [word-break:auto-phrase]",
    // At the text's measure in a narrow panel; from the glyph's start, the panel's whole measure, in a wide one.
    note: [
      "m-0 text-caption [word-break:auto-phrase]",
      "@min-[30rem]:-ms-[calc(var(--icon-action)+var(--entry-avatar-gap))]",
    ],
    actions: "flex flex-wrap items-center gap-shell-inset",
  },
});

export type InlinePromptProps = {
  /** The glyph of what it offers. */
  icon: IconName;
  /** What it offers, in one line or a short text; it names the prompt. */
  children: ReactNode;
  /**
   * What does it; none once nothing is left to try. It shows no spinner, since the prompt goes once it worked (a
   * pending button that unmounts leaves its announcement behind): its owner ignores a second press while it runs.
   */
  action?: { label: string; onPress: () => void } | undefined;
  /** Dismissing it, for good. */
  dismiss: { label: string; onPress: () => void };
  /** Why the action did not work, under the text. */
  reason?: string | undefined;
};

/** A prompt inside a screen: its glyph and text, its action, and its dismiss; a reason under it when the action failed. */
export function InlinePrompt({ icon, children, action, dismiss, reason }: InlinePromptProps) {
  const s = prompt();
  const id = useId();
  return (
    <section className={s.root()} aria-labelledby={id}>
      <div className={s.row()}>
        <span className={s.glyph()}>
          <Icon name={icon} place="action" />
        </span>
        <div className={s.body()}>
          <p id={id} className={s.text()} dir="auto">
            {children}
          </p>
          {reason !== undefined && (
            <p className={s.note()} role="alert" dir="auto">
              {reason}
            </p>
          )}
          <div className={s.actions()}>
            {action !== undefined && (
              <Button kind="primary" size="timeline" onPress={action.onPress}>
                {action.label}
              </Button>
            )}
            <Button kind="quiet" size="timeline" onPress={dismiss.onPress}>
              {dismiss.label}
            </Button>
          </div>
        </div>
      </div>
    </section>
  );
}
