// SPDX-License-Identifier: AGPL-3.0-only

// The save state of a control that saves at once (ADR-0110 §5): a caption's line of a glyph and a few words, which
// takes the place of the line under the control's label while it has something to say, so that nothing on the screen
// moves: saving, a spinner; saved, a check, for a moment; failed, the error's glyph and the sentence in the text's
// color, with what the reader may do after it (try again), until it is answered. It has no hue: the accent is the
// primary action's and the links'. Its words are the design system's own, in the reader's language.
//
// What holds it (a setting's row, a switch, the submit bar) keeps a polite live region (`role="status"`) in its place
// from the start, so that assistive technology hears the state the moment it stands there, and the focus stays where
// it is: saved and failed are read, each a new element of the region; saving is not (it is no part of the live
// region, and is still read with the control it describes). Trying again is a button of the line; as it is pressed,
// the focus goes to the control whose change it saves, since the button leaves with the failure.

import { motion } from "@my/design-tokens";
import { createContext, use, type ReactNode } from "react";
import { tv } from "tailwind-variants/lite";
import { TextButton } from "./button";
import { Icon } from "./icons";
import { Box, Row } from "./layout";
import { stringsIn, useReader } from "./locale";
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

/** Puts the focus on the control whose change a save state tells of, where what holds the state has one. */
export const SaveControlContext = createContext<(() => void) | undefined>(undefined);

export type SaveStateProps = {
  state: "saving" | "saved" | "failed";
  /** Saves the change again after a failure: Try again, a button after the sentence. */
  onRetry?: (() => void) | undefined;
  /** What the reader may do about a failure in place of trying again, after the sentence (a `TextButton`). */
  action?: ReactNode;
  /** What it says, where the design system's own words do not fit: "Saving…", "Saved", "This could not be saved." */
  children?: string | undefined;
};

/**
 * The save state of a control that saves at once: a glyph and a few words, in the line under the control's label.
 * Give it to what holds the control (`save` of a `SettingRow` or a `Switch`, `result` of a `SubmitBar`), which reads
 * it to assistive technology.
 */
export function SaveState({ state, onRetry, action, children }: SaveStateProps) {
  const s = save({ state });
  const reader = useReader(undefined);
  const { t, lang } = stringsIn(reader.lang);
  const toControl = use(SaveControlContext);
  const words = children ?? t(`save_state.${state}`);
  const after =
    action ??
    (state === "failed" && onRetry !== undefined ? (
      <TextButton
        onPress={() => {
          // The button leaves as the change is saved again: the focus goes to the control first.
          toControl?.();
          onRetry();
        }}
      >
        <span lang={lang}>{t("save_state.retry")}</span>
      </TextButton>
    ) : undefined);
  return (
    <Row
      // Saved and failed are a new element of the live region that holds it, which is read; saving is none of it.
      key={state === "saving" ? "saving" : "told"}
      as="span"
      align="start"
      gap="entry-caption"
      className={s.root()}
      aria-live={state === "saving" ? "off" : undefined}
    >
      <Row as="span" inline line rigid align="center">
        {state === "saving" ? (
          <Box as="span" rigid className={s.spinner()} aria-hidden="true" />
        ) : (
          <Icon name={state === "saved" ? "chosen" : "error"} place="caption" />
        )}
      </Row>
      <span>
        <span lang={children === undefined ? lang : undefined}>{words}</span>
        {after !== undefined && (
          <>
            {actionJoint(words)}
            {after}
          </>
        )}
      </span>
    </Row>
  );
}
