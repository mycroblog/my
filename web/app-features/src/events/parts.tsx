// SPDX-License-Identifier: AGPL-3.0-only

// What the screens of events share: their strings, the line that says a list loads, failed or is empty, and a field
// of a date and a time under its name.

import { DateTimeField, FieldError, FieldLabel, Stack, TextButton, useLocale } from "@my/design-system";
import { stringsIn } from "../locale";

export type T = ReturnType<typeof stringsIn>["t"];

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
