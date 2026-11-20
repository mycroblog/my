// SPDX-License-Identifier: AGPL-3.0-only

// What the operations screens share: a count in the reader's locale.

import { useLocale } from "@my/design-system";

/** Writes a count in the reader's locale (1,204). */
export function useCount(): (n: number) => string {
  const { locale } = useLocale();
  const format = new Intl.NumberFormat(locale);
  return (n) => format.format(n);
}
