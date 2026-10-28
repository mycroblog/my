// SPDX-License-Identifier: AGPL-3.0-only
import { useT } from "../../../lib/i18n";

/** The count message of a text field with a limit: the characters left, or how many are over. */
export function useFormatRemaining(): (remaining: number) => string {
  const t = useT();
  return (remaining) =>
    remaining < 0
      ? t("client:compose.over", { count: -remaining })
      : t("client:compose.remaining", { count: remaining });
}
