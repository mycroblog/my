// SPDX-License-Identifier: AGPL-3.0-only

// A prompt inside a screen, not over it (the Web Push prompt over the notifications, ADR-0086 §18–§19): on the raised
// surface, no title, a glyph and one line or a short text of what it offers, and under it the action that does it, the
// primary button, beside a quiet one that dismisses it. A reason under the text, at the text's measure and in a
// caption's size, says why the action did not work (the reader closed the browser's question, the subscription
// failed), announced as it comes; when nothing is left to try, the prompt keeps only its dismiss. It is named by its
// text for assistive technology.

import { useId, type HTMLAttributes, type ReactNode } from "react";
import { tv } from "tailwind-variants/lite";
import { Button } from "./button";
import { Icon, type IconName } from "./icons";
import { Row, Stack } from "./layout";

const prompt = tv({
  slots: {
    root: "rounded-overlay bg-surface-raised px-embed py-shell-inset text-body text-text",
    // A text that wraps breaks between phrases, never inside a word of ja.
    text: "[word-break:auto-phrase]",
    note: "text-caption [word-break:auto-phrase]",
  },
});

/** The prompt's box: a container, so that the reason takes the panel's measure where the panel is wide. */
function PromptFrame({ className, ...rest }: HTMLAttributes<HTMLElement> & { className: string }) {
  return <section {...rest} className={`${className} @container`} />;
}

/** The reason: at the text's measure in a narrow panel; from the glyph's start, the panel's whole measure, in a wide one. */
function PromptNote({ className, ...rest }: HTMLAttributes<HTMLParagraphElement> & { className: string }) {
  return <p {...rest} className={`${className} @min-[30rem]:-ms-[calc(var(--icon-action)+var(--entry-avatar-gap))]`} />;
}

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
    <PromptFrame className={s.root()} aria-labelledby={id}>
      <Row align="start" gap="entry-avatar">
        <Row as="span" rigid line align="center">
          <Icon name={icon} place="action" />
        </Row>
        <Stack grow gap="shell-inset">
          <p id={id} className={s.text()} dir="auto">
            {children}
          </p>
          {reason !== undefined && (
            <PromptNote className={s.note()} role="alert" dir="auto">
              {reason}
            </PromptNote>
          )}
          <Row wrap align="center" gap="shell-inset">
            {action !== undefined && (
              <Button kind="primary" size="timeline" onPress={action.onPress}>
                {action.label}
              </Button>
            )}
            <Button kind="quiet" size="timeline" onPress={dismiss.onPress}>
              {dismiss.label}
            </Button>
          </Row>
        </Stack>
      </Row>
    </PromptFrame>
  );
}
