// SPDX-License-Identifier: AGPL-3.0-only

// A field that finds as the reader writes (the search, a list's members): a searchbox on a field's raised surface,
// the search glyph at its start, and at its end a spinner while what it asks for is fetched from afar (an account of
// another server looked up) or, once there are words, clear, which empties it and keeps the focus in it. Its label
// above it, its hint under the label, its error under it, both read with it, as a form's field (form-page.tsx). Escape
// empties it too, as the browser's search field does.

import { motion } from "@my/design-tokens";
import { useRef, useState } from "react";
import { tv } from "tailwind-variants/lite";
import { FieldError } from "./form-page";
import { IconButton } from "./icon-button";
import { Icon } from "./icons";
import { Box, Pin, Row, Stack } from "./layout";

const field = tv({
  slots: {
    label: "font-label",
    hidden: "sr-only",
    hint: "text-caption text-muted-text [word-break:auto-phrase]",
    glyph: "pointer-events-none text-muted-text",
    // Its line is as tall as the field: a field cuts what reaches under a shorter line (the tail of a g).
    input: [
      "box-border h-control-form appearance-none rounded-control border-0 bg-surface-raised font-sans",
      "ps-search-field-start pe-control-form",
      "text-body leading-(--control-form-height) text-text outline-hidden focus-visible:shadow-focus [&::-webkit-search-cancel-button]:appearance-none",
      "placeholder:text-muted-text",
      "aria-invalid:outline-solid aria-invalid:outline-[length:var(--form-page-invalid)]",
      "aria-invalid:-outline-offset-[var(--form-page-invalid)] aria-invalid:outline-text",
    ],
    end: "size-control-form",
    spinner: [
      "box-border size-spinner rounded-full border-spinner border-solid border-spinner-track border-t-current",
      "animate-spin text-muted-text",
      motion.spin.reduced === "still" ? "motion-reduce:animate-none" : "",
    ],
  },
});

export type SearchFieldProps = {
  id: string;
  /** Its label, which names the searchbox. */
  label: string;
  /** What it finds, under the label. */
  hint?: string | undefined;
  /** The words it starts with. */
  defaultValue?: string | undefined;
  /** Its words, where its owner holds them (a filter bar, whose Clear filters empties it too). */
  value?: string | undefined;
  /**
   * Its label is read, not shown, and stands in the empty field as its placeholder: where what stands around it says
   * what it searches (the filter bar over a list).
   */
  labelHidden?: boolean | undefined;
  /** Its words as the reader writes them, and once cleared (""). */
  onChange: (value: string) => void;
  /** What is wrong (the server refused what it found), under it. */
  error?: string | undefined;
  /** Something it asked for is fetched from afar: a spinner in place of clear, the field busy. */
  busy?: boolean | undefined;
  /** The name of clear. */
  clearLabel: string;
  /** It takes the focus as it mounts (a screen opened to search). */
  autoFocus?: boolean | undefined;
};

export function SearchField({
  id,
  label,
  hint,
  defaultValue = "",
  value: held,
  labelHidden = false,
  onChange,
  error,
  busy = false,
  clearLabel,
  autoFocus,
}: SearchFieldProps) {
  const s = field();
  const input = useRef<HTMLInputElement>(null);
  const [own, setOwn] = useState(defaultValue);
  const value = held ?? own;
  const change = (next: string) => {
    setOwn(next);
    onChange(next);
  };
  const described = [hint && `${id}-hint`, error && `${id}-error`].filter((part) => part).join(" ") || undefined;
  return (
    <Stack gap="form-page-field-gap">
      <label className={labelHidden ? s.hidden() : s.label()} htmlFor={id}>
        {label}
      </label>
      {hint !== undefined && (
        <p id={`${id}-hint`} className={s.hint()}>
          {hint}
        </p>
      )}
      {/* A little room under the hint, so that the field and its focus ring stand clear of it. */}
      <Row anchor align="center" spaceAbove="screen-air">
        <Pin start="control-timeline-inline" as={<Row as="span" inline align="center" />} className={s.glyph()}>
          <Icon name="search" place="action" />
        </Pin>
        <Box
          full
          className={s.input()}
          as={
            <input
              ref={input}
              id={id}
              type="search"
              value={value}
              autoComplete="off"
              placeholder={labelHidden ? label : undefined}
              spellCheck={false}
              autoFocus={autoFocus}
              aria-invalid={error ? true : undefined}
              aria-describedby={described}
              aria-busy={busy || undefined}
              onChange={(e) => {
                change(e.currentTarget.value);
              }}
              onKeyDown={(e) => {
                if (e.key === "Escape" && value !== "" && !e.nativeEvent.isComposing) {
                  e.preventDefault();
                  change("");
                }
              }}
            />
          }
        />
        {busy ? (
          <Pin end="0" as={<Row as="span" align="center" justify="center" />} className={s.end()} aria-hidden="true">
            <span className={s.spinner()} />
          </Pin>
        ) : (
          value !== "" && (
            <Pin end="0" as={<Row as="span" align="center" justify="center" />} className={s.end()}>
              <IconButton
                icon="close"
                label={clearLabel}
                onPress={() => {
                  change("");
                  input.current?.focus();
                }}
              />
            </Pin>
          )
        )}
      </Row>
      {error && <FieldError id={`${id}-error`}>{error}</FieldError>}
    </Stack>
  );
}
