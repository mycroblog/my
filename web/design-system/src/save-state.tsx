// SPDX-License-Identifier: AGPL-3.0-only

// The save state of a control that saves at once (ADR-0110 §5): a caption's line of a glyph and a few words, which
// takes the place of the line under the control's label while it has something to say, so that nothing on the screen
// moves: saving, a spinner; saved, a check, for a moment; failed, the error's glyph and the sentence in the text's
// color, with what the reader may do after it (try again), until it is answered. It has no hue: the accent is the
// primary action's and the links'.

import { motion } from "@my/design-tokens";
import type { ReactNode } from "react";
import { tv } from "tailwind-variants/lite";
import { Icon } from "./icons";
import { Box, Row } from "./layout";
import { actionJoint } from "./sentence";

const save = tv({
  slots: {
    // Wrapping between phrases, never inside a word of ja.
    root: "text-caption [word-break:auto-phrase]",
    spinner: [
      "box-border size-icon-caption rounded-full border-spinner border-solid border-spinner-track border-t-current",
      "animate-spin",
      motion.spin.reduced === "still" ? "motion-reduce:animate-none" : "",
    ],
  },
  variants: {
    state: {
      saving: { root: "text-muted-text" },
      saved: { root: "text-muted-text" },
      failed: { root: "font-label text-text" },
    },
  },
});

export type SaveStateProps = {
  state: "saving" | "saved" | "failed";
  /** What the reader may do about a failure, after the sentence (a `TextButton`: try again). */
  action?: ReactNode;
  /** What it says: "Saving…", "Saved", "Couldn't save." */
  children: string;
};

/** The save state of a control that saves at once: a glyph and a few words, in the line under the control's label. */
export function SaveState({ state, action, children }: SaveStateProps) {
  const s = save({ state });
  return (
    <Row as="span" align="start" gap="entry-caption" className={s.root()} role="status">
      <Row as="span" inline line rigid align="center">
        {state === "saving" ? (
          <Box as="span" rigid className={s.spinner()} aria-hidden="true" />
        ) : (
          <Icon name={state === "saved" ? "chosen" : "error"} place="caption" />
        )}
      </Row>
      <span>
        {children}
        {action !== undefined && (
          <>
            {actionJoint(children)}
            {action}
          </>
        )}
      </span>
    </Row>
  );
}
