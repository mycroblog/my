// SPDX-License-Identifier: AGPL-3.0-only

// What the screens of events share: their strings, the line that says a list loads, failed or is empty, a field of a
// date and a time under its name, and the focus handed on once a dialog that holds it is gone.

import { DateTimeField, FieldError, FieldLabel, Stack, TextButton, useLocale } from "@my/design-system";
import { stringsIn } from "../locale";

export type T = ReturnType<typeof stringsIn>["t"];

/** Whether a dialog over the page (the composer) holds the focus. */
const inDialog = () => document.activeElement?.closest("[role=dialog]") != null;

/**
 * Moves the focus by `focus` now, or, while a dialog over the page holds it (the composer opened from what is gone),
 * once that dialog has left the document and given the focus back to nothing: a modal gives it back to what opened it,
 * and to nothing when that is gone.
 */
export function focusWhenFree(focus: () => void): void {
  if (!inDialog()) {
    focus();
    return;
  }
  const watch = new MutationObserver(() => {
    if (inDialog()) return;
    watch.disconnect();
    const active = document.activeElement;
    if (active === null || active === document.body) focus();
  });
  watch.observe(document.body, { childList: true, subtree: true });
}

/** The strings of the screens of events, in the reader's locale. */
export function useEventStrings(): T {
  return stringsIn(useLocale().locale).t;
}

/** The button that tries again what failed. */
export function Retry({ onPress, t }: { onPress: () => void; t: T }) {
  return <TextButton onPress={onPress}>{t("events.retry")}</TextButton>;
}

/** A field of a date and a time under its name, what is wrong with it under it. */
export function DateField({
  id,
  label,
  value,
  onChange,
  min,
  max,
  error,
  zoneId,
}: {
  id: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
  min: string;
  max?: string | undefined;
  error: string | undefined;
  /** The id of what names the zone the field is read in, where the field's place does not say it. */
  zoneId?: string | undefined;
}) {
  return (
    <Stack gap="form-page-field-gap">
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <DateTimeField
        id={id}
        label={label}
        value={value}
        onChange={onChange}
        min={min}
        max={max}
        invalid={error !== undefined}
        describedBy={[zoneId, error === undefined ? undefined : `${id}-error`].filter(Boolean).join(" ") || undefined}
      />
      {error !== undefined && <FieldError id={`${id}-error`}>{error}</FieldError>}
    </Stack>
  );
}
