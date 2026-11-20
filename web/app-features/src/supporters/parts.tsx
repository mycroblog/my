// SPDX-License-Identifier: AGPL-3.0-only

// What the supporters' page shares among its parts: its strings, a day as the reader writes it, and the button that
// tries again what failed.

import { dateFormat, TextButton, useLocale, useTimeZone } from "@my/design-system";
import { stringsIn } from "../locale";

export type T = ReturnType<typeof stringsIn>["t"];

/** The strings of the supporters' page, in the reader's locale. */
export function useSupporterStrings(): T {
  return stringsIn(useLocale().locale).t;
}

/** The space a line does not break at (U+00A0), by its number: written out, it reads as any space. */
const noBreak = String.fromCharCode(0xa0);

/**
 * A day as the reader's language and time zone write it, with its year (when a period ends, when a change applies),
 * holding together: its spaces do not break.
 */
export function useDay(): (iso: string) => string {
  const { locale } = useLocale();
  const format = dateFormat(locale, useTimeZone(), { dateStyle: "medium" });
  return (iso) => format.format(new Date(iso)).replaceAll(" ", noBreak);
}

/** The button that tries again what failed. */
export function Retry({ onPress, t }: { onPress: () => void; t: T }) {
  return <TextButton onPress={onPress}>{t("supporters.retry")}</TextButton>;
}
