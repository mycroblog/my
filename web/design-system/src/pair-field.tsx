// SPDX-License-Identifier: AGPL-3.0-only

// Pairs the reader writes, each a short name and what goes under it (the fields of a profile): a list of them in the
// order they were entered, each pair two fields side by side, the name first, which go one under the other where the
// row is too narrow for both, and its remove at its end. A field is named by its placeholder's words, as an option of
// a poll is, and takes the page's surface on a raised section. What is wrong with one field marks that field alone
// and stands directly under it (under a name, on a line of its own before the other field); what is wrong with the
// pairs as a whole (too many) stands under the list and marks none.
//
// For assistive technology the list is named by `label` and described by its error; a pair is a group named by
// `legend` ("Field 2"), so that one Name is told from the next, its remove says which pair it removes, and the field
// that is wrong is invalid and described by what is wrong with it.

import { useId, type ReactNode } from "react";
import { tv } from "tailwind-variants/lite";
import { FieldError } from "./form-page";
import { IconButton } from "./icon-button";
import { Box, Row, Stack } from "./layout";

const pair = tv({
  slots: {
    field: [
      "box-border h-control-form rounded-control border-0 bg-surface-raised px-control-timeline-inline font-sans",
      // Its line is as tall as the field: a field cuts what reaches under a shorter line (the tail of a g).
      "text-body leading-(--control-form-height) text-text outline-hidden placeholder:text-muted-text",
      "focus-visible:shadow-focus",
      "disabled:opacity-disabled [[data-surface-raised]_&]:bg-surface",
      "aria-invalid:outline-solid aria-invalid:outline-[length:var(--form-page-invalid)]",
      "aria-invalid:-outline-offset-[var(--form-page-invalid)] aria-invalid:outline-text",
    ],
  },
});

/**
 * The pairs (`PairField`), one under another, named by `label`; `error` says what is wrong with them as a whole,
 * right under the list. The list has the id `id` and takes the focus when what holds it gives it (no key leads to
 * it), so that a refusal of the pairs as a whole is read with the list and comes into view (`focusRefusedField`).
 */
export function PairList({
  id,
  label,
  error,
  children,
}: {
  id: string;
  label: string;
  error?: string | undefined;
  children: ReactNode;
}) {
  return (
    <Stack gap="form-page-field-gap">
      {/* A pair's two fields stand closer to one another than to the next pair, where they stand one under the other. */}
      <Stack
        as="ul"
        plain
        gap="shell-edge"
        id={id}
        tabIndex={-1}
        aria-label={label}
        aria-describedby={error === undefined ? undefined : `${id}-error`}
      >
        {children}
      </Stack>
      {error !== undefined && <FieldError id={`${id}-error`}>{error}</FieldError>}
    </Stack>
  );
}

export type PairFieldProps = {
  /**
   * The id of the name's field, for what holds the list to put the focus on the pair; the other field's is
   * `${id}-value`, and what is wrong with either has its field's id with `-error`, as a `TextField`'s has
   * (`focusRefusedField`).
   */
  id?: string | undefined;
  /** Which pair it is, for assistive technology ("Field 2"). */
  legend: string;
  /** The name's field: what it is called, and what it holds. */
  nameLabel: string;
  name: string;
  onName: (name: string) => void;
  /** The field of what goes under the name. */
  valueLabel: string;
  value: string;
  onValue: (value: string) => void;
  /** The most characters of each field. */
  maxLength: number;
  /** Removes the pair, under these words ("Remove field 2"). */
  remove: { label: string; onPress: () => void };
  /** What is wrong with one of its fields, directly under that field, which alone takes the line of the text's color. */
  error?: { of: "name" | "value"; text: string } | undefined;
  /** It cannot be changed now (the form is being sent). */
  isDisabled?: boolean | undefined;
  /** Its name's field takes the focus as the pair mounts (a pair the reader just added). */
  autoFocus?: boolean | undefined;
};

/** A pair of a list: the name's field, the field of what goes under it, and the pair's remove. */
export function PairField({
  id,
  legend,
  nameLabel,
  name,
  onName,
  valueLabel,
  value,
  onValue,
  maxLength,
  remove,
  error,
  isDisabled = false,
  autoFocus = false,
}: PairFieldProps) {
  const s = pair();
  const own = useId();
  const fieldId = { name: id ?? own, value: `${id ?? own}-value` } as const;
  const errorId = error === undefined ? undefined : `${fieldId[error.of]}-error`;
  const field = (of: "name" | "value", label: string, text: string, onChange: (text: string) => void) => (
    <Box
      grow="names"
      className={s.field()}
      as={
        <input
          id={fieldId[of]}
          value={text}
          placeholder={label}
          aria-label={label}
          aria-invalid={error?.of === of ? true : undefined}
          aria-describedby={error?.of === of ? errorId : undefined}
          maxLength={maxLength}
          disabled={isDisabled}
          autoFocus={of === "name" && autoFocus}
          dir="auto"
          onChange={(e) => {
            onChange(e.target.value);
          }}
        />
      }
    />
  );
  return (
    <Stack as="li" gap="form-page-field-gap">
      <Row wrap align="center" gap="entry-caption" role="group" aria-label={legend}>
        {field("name", nameLabel, name, onName)}
        {/* What is wrong with the name stands on a line of its own right under it, the other field after it. */}
        {error?.of === "name" && (
          <Box full>
            <FieldError id={errorId}>{error.text}</FieldError>
          </Box>
        )}
        {field("value", valueLabel, value, onValue)}
        <IconButton icon="close" label={remove.label} isDisabled={isDisabled} onPress={remove.onPress} />
      </Row>
      {error?.of === "value" && <FieldError id={errorId}>{error.text}</FieldError>}
    </Stack>
  );
}
