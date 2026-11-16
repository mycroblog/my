// SPDX-License-Identifier: AGPL-3.0-only

// A prompt inside a screen, not over it (the Web Push prompt over the notifications, ADR-0086 §18–§19): no title, a
// glyph and one line or a short text of what it offers, the action that does it, and a way to dismiss it. A reason
// under the text says why the action did not work (the reader closed the browser's question, the subscription
// failed), announced as it comes; when nothing is left to try, the prompt keeps only its dismiss. It is named by its
// text for assistive technology.

import { useId, type ReactNode } from "react";
import { tv } from "tailwind-variants/lite";
import { Button, TextButton } from "./button";
import { IconButton } from "./icon-button";
import { Icon, type IconName } from "./icons";

const prompt = tv({
  slots: {
    root: "flex items-start gap-entry-avatar text-body text-text",
    glyph: "inline-flex h-[1lh] flex-none items-center",
    body: "grid min-w-0 flex-1 gap-shell-inset",
    // A text that wraps breaks between phrases, never inside a word of ja.
    text: "m-0 [word-break:auto-phrase]",
    note: "m-0 flex items-start gap-entry-caption text-caption font-label [word-break:auto-phrase] [&_svg]:mt-[0.1em]",
    actions: "flex flex-wrap items-center gap-shell-inset",
    close: "-my-[calc((var(--control-timeline-height)-1lh)/2)] -me-shell-inset flex-none",
  },
  variants: {
    // How it stands in the screen: on the raised surface, its action under the text and its close at the end; on the
    // page's surface, its action and dismiss as words after the text; or on the raised surface, its two buttons under
    // the text and no close.
    look: {
      raised: { root: "rounded-overlay bg-surface-raised px-embed py-shell-inset" },
      line: { root: "py-shell-inset" },
      stacked: { root: "rounded-overlay bg-surface-raised px-embed py-shell-inset" },
    },
  },
});

/** How an inline prompt stands in its screen (InlinePrompt's `look`). */
export type PromptLook = "raised" | "line" | "stacked";

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
  look?: PromptLook | undefined;
};

/** A prompt inside a screen: its glyph and text, its action, and its dismiss; a reason under it when the action failed. */
export function InlinePrompt({ icon, children, action, dismiss, reason, look = "raised" }: InlinePromptProps) {
  const s = prompt({ look });
  const id = useId();
  const note = reason !== undefined && (
    <p className={s.note()} role="alert">
      <Icon name="error" place="caption" />
      <span dir="auto">{reason}</span>
    </p>
  );
  if (look === "line")
    return (
      <section className={s.root()} aria-labelledby={id}>
        <span className={s.glyph()}>
          <Icon name={icon} place="action" />
        </span>
        <div className={s.body()}>
          <p className={s.text()} dir="auto">
            <span id={id}>{children}</span>{" "}
            {action !== undefined && (
              <>
                <TextButton onPress={action.onPress}>{action.label}</TextButton>
                <span aria-hidden="true"> · </span>
              </>
            )}
            <TextButton onPress={dismiss.onPress}>{dismiss.label}</TextButton>
          </p>
          {note}
        </div>
      </section>
    );
  return (
    <section className={s.root()} aria-labelledby={id}>
      <span className={s.glyph()}>
        <Icon name={icon} place="action" />
      </span>
      <div className={s.body()}>
        <p id={id} className={s.text()} dir="auto">
          {children}
        </p>
        {note}
        {(action !== undefined || look === "stacked") && (
          <div className={s.actions()}>
            {action !== undefined && (
              <Button kind="primary" size="timeline" onPress={action.onPress}>
                {action.label}
              </Button>
            )}
            {look === "stacked" && (
              <Button kind="quiet" size="timeline" onPress={dismiss.onPress}>
                {dismiss.label}
              </Button>
            )}
          </div>
        )}
      </div>
      {look === "raised" && (
        <span className={s.close()}>
          <IconButton icon="close" label={dismiss.label} onPress={dismiss.onPress} />
        </span>
      )}
    </section>
  );
}
