// SPDX-License-Identifier: AGPL-3.0-only
// The reason an operator gives for an action (a moderation action, lifting it, a report's resolution): a
// multi-line field with the server's character limit always counted, so the operator never learns of the
// limit from a failed request.
import { TextArea, type TextAreaProps } from "./TextArea";

export type ReasonFieldProps = Omit<TextAreaProps, "characterLimit" | "formatRemaining"> & {
  /** The most characters the server accepts for this reason (5,000 for most). */
  characterLimit: number;
  /** The count message, e.g. "12 characters left" / "3 characters too many"; `remaining` is negative when over. */
  formatRemaining: (remaining: number) => string;
};

/** A reason field: a `TextArea` of three rows with a character count. Mark it `isRequired` when the action needs one. */
export function ReasonField({ rows = 3, ...props }: ReasonFieldProps) {
  return <TextArea rows={rows} {...props} />;
}
