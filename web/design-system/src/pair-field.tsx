// SPDX-License-Identifier: AGPL-3.0-only

// Pairs the reader writes, each a short name and what goes under it (the fields of a profile): a list of them in the
// order they were entered, each pair two fields side by side, the name first, which go one under the other where the
// row is too narrow for both, and its remove at its end. A field is named by its placeholder's words, as an option of
// a poll is, and takes the page's surface on a raised section. What is wrong with the pairs as a whole stands right
// under the list.
//
// For assistive technology the list is named by `label` and described by its error; a pair is a group named by
// `legend` ("Field 2"), so that one Name is told from the next, and its remove says which pair it removes.

import type { ReactNode } from "react";
import { tv } from "tailwind-variants/lite";
import { FieldError } from "./form-page";
import { IconButton } from "./icon-button";
import { Box, Row, Stack } from "./layout";

const pair = tv({
  slots: {
    field: [
      "box-border h-control-form rounded-control border-0 bg-surface-raised px-control-timeline-inline font-sans",
      "text-body text-text outline-hidden placeholder:text-muted-text focus-visible:shadow-focus",
      "disabled:opacity-disabled [[data-surface-raised]_&]:bg-surface",
      "aria-invalid:outline-solid aria-invalid:outline-[length:var(--form-page-invalid)]",
      "aria-invalid:-outline-offset-[var(--form-page-invalid)] aria-invalid:outline-text",
    ],
  },
});

/**
 * The pairs (`PairField`), one under another, named by `label`; `error` says what is wrong with them, right under
 * the list, and marks every field of it.
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
  /** The id of the name's field, for what holds the list to put the focus on the pair. */
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
  /** Its fields take the line of the text's color: the list's error is about them. */
  invalid?: boolean | undefined;
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
  invalid = false,
  isDisabled = false,
  autoFocus = false,
}: PairFieldProps) {
  const s = pair();
  const field = (label: string, text: string, onChange: (text: string) => void, first: boolean) => (
    <Box
      grow="names"
      className={s.field()}
      as={
        <input
          id={first ? id : undefined}
          value={text}
          placeholder={label}
          aria-label={label}
          aria-invalid={invalid ? true : undefined}
          maxLength={maxLength}
          disabled={isDisabled}
          autoFocus={first && autoFocus}
          dir="auto"
          onChange={(e) => {
            onChange(e.target.value);
          }}
        />
      }
    />
  );
  return (
    <Stack as="li">
      <Row wrap align="center" gap="entry-caption" role="group" aria-label={legend}>
        {field(nameLabel, name, onName, true)}
        {field(valueLabel, value, onValue, false)}
        <IconButton icon="close" label={remove.label} isDisabled={isDisabled} onPress={remove.onPress} />
      </Row>
    </Stack>
  );
}
