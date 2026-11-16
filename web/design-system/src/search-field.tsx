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

const field = tv({
  slots: {
    root: "grid gap-form-page-field-gap",
    label: "font-label",
    hint: "m-0 text-caption text-muted-text [word-break:auto-phrase]",
    // A little room under the hint, so that the field and its focus ring stand clear of it.
    box: "relative mt-screen-air flex items-center",
    glyph: "pointer-events-none absolute start-control-timeline-inline inline-flex text-muted-text",
    input: [
      "box-border h-control-form w-full appearance-none rounded-control border-0 bg-surface-raised font-sans",
      "ps-[calc(var(--control-timeline-padding-inline)*2+var(--icon-action))] pe-[var(--control-form-height)]",
      "text-body text-text outline-hidden focus-visible:shadow-focus [&::-webkit-search-cancel-button]:appearance-none",
      "aria-invalid:outline-solid aria-invalid:outline-[length:var(--form-page-invalid)]",
      "aria-invalid:-outline-offset-[var(--form-page-invalid)] aria-invalid:outline-text",
    ],
    end: "absolute end-0 inline-flex size-[var(--control-form-height)] items-center justify-center",
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
  onChange,
  error,
  busy = false,
  clearLabel,
  autoFocus,
}: SearchFieldProps) {
  const s = field();
  const input = useRef<HTMLInputElement>(null);
  const [value, setValue] = useState(defaultValue);
  const change = (next: string) => {
    setValue(next);
    onChange(next);
  };
  const described = [hint && `${id}-hint`, error && `${id}-error`].filter((part) => part).join(" ") || undefined;
  return (
    <div className={s.root()}>
      <label className={s.label()} htmlFor={id}>
        {label}
      </label>
      {hint !== undefined && (
        <p id={`${id}-hint`} className={s.hint()}>
          {hint}
        </p>
      )}
      <div className={s.box()}>
        <span className={s.glyph()}>
          <Icon name="search" place="action" />
        </span>
        <input
          ref={input}
          id={id}
          type="search"
          className={s.input()}
          value={value}
          autoComplete="off"
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
        {busy ? (
          <span className={s.end()} aria-hidden="true">
            <span className={s.spinner()} />
          </span>
        ) : (
          value !== "" && (
            <span className={s.end()}>
              <IconButton
                icon="close"
                label={clearLabel}
                onPress={() => {
                  change("");
                  input.current?.focus();
                }}
              />
            </span>
          )
        )}
      </div>
      {error && <FieldError id={`${id}-error`}>{error}</FieldError>}
    </div>
  );
}
