// SPDX-License-Identifier: AGPL-3.0-only

// A file the reader picks from their device that is no picture (a CSV file to import): a field of a form whose control
// is the button that opens the system's picker for one file, in the caller's words (choose one, or another than the
// one named), with the name of the file it holds after it, or that it holds none yet. What it takes (`accept`) is
// what the system's picker offers first; the caller still checks what it is given.
//
// For assistive technology the field is a group named by its label and described by its hint and its error; its
// button is described by the name of the file it holds, and takes the focus when the field is refused (its `id`), the
// error right under what it is about.

import { useRef } from "react";
import { tv } from "tailwind-variants/lite";
import { Button } from "./button";
import { describedBy } from "./field";
import { FieldError } from "./form-page";
import { Row, Stack } from "./layout";

const fileField = tv({
  slots: {
    label: "",
    hint: "text-caption text-muted-text [word-break:auto-phrase]",
    // The name of the file it holds, which someone else wrote: it breaks anywhere it must.
    name: "[overflow-wrap:anywhere]",
    none: "text-muted-text [word-break:auto-phrase]",
  },
  variants: {
    // A field's label; among the rows of settings, a setting's name: the text's own weight, as the names beside it.
    row: { true: { label: "[word-break:auto-phrase]" }, false: { label: "font-label" } },
  },
});

export type FileFieldProps = {
  /** The id of its button, which takes the focus when the field is refused; its error is `<id>-error`. */
  id: string;
  label: string;
  hint?: string | undefined;
  /** What is wrong with the file, or that one is missing, right under the field. */
  error?: string | undefined;
  /** The words of the button that opens the picker: choosing a file, or another than the one named. */
  choose: string;
  /** The name of the file it holds; left out for none. */
  file?: string | undefined;
  /** What stands in the name's place while it holds none (No file chosen). */
  none: string;
  /** What the picker offers, as an `<input type="file">` takes it (".csv,text/csv"). */
  accept?: string | undefined;
  /** The file the reader picked. */
  onFile: (file: File) => void;
  /** It cannot be changed now (the form is being sent). */
  isDisabled?: boolean | undefined;
  /**
   * It stands among the rows of settings of a section (a `SettingRow` before or after it): its label looks as their
   * names do, not as the label of a field in a form of fields.
   */
  row?: boolean | undefined;
};

export function FileField({
  id,
  label,
  hint,
  error,
  choose,
  file,
  none,
  accept,
  onFile,
  isDisabled = false,
  row = false,
}: FileFieldProps) {
  const s = fileField({ row });
  const input = useRef<HTMLInputElement>(null);
  return (
    <Stack
      gap="form-page-field-gap"
      role="group"
      aria-labelledby={`${id}-label`}
      aria-describedby={describedBy(hint !== undefined && `${id}-hint`, error !== undefined && `${id}-error`)}
    >
      <span id={`${id}-label`} className={s.label()}>
        {label}
      </span>
      {hint !== undefined && (
        <p id={`${id}-hint`} className={s.hint()}>
          {hint}
        </p>
      )}
      <Row wrap align="center" gap="screen-row-gap">
        <Row as="span" inline rigid>
          <Button
            id={id}
            size="timeline"
            touch
            isDisabled={isDisabled}
            aria-describedby={`${id}-file`}
            onPress={() => {
              input.current?.click();
            }}
          >
            {choose}
          </Button>
        </Row>
        {file === undefined ? (
          <span id={`${id}-file`} className={s.none()}>
            {none}
          </span>
        ) : (
          <bdi id={`${id}-file`} className={s.name()}>
            {file}
          </bdi>
        )}
        <input
          ref={input}
          type="file"
          accept={accept}
          hidden
          onChange={(e) => {
            const picked = e.target.files?.[0];
            // The same file picked again is a change again.
            e.target.value = "";
            if (picked !== undefined) onFile(picked);
          }}
        />
      </Row>
      {error !== undefined && <FieldError id={`${id}-error`}>{error}</FieldError>}
    </Stack>
  );
}
