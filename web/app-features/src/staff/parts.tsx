// SPDX-License-Identifier: AGPL-3.0-only

// What the staff's screens share: their strings in the reader's locale, how a moment is written (the reader's zone, a
// date and its time held together), a heading over what it heads, and a notice that a screen's read loads or failed.

import { sentenceHandleOf } from "@my/app-components";
import type { Account } from "@my/app-data";
import { dateFormat, FeedNotice, GroupHeading, Stack, TextButton, useLocale, useTimeZone } from "@my/design-system";
import type { ReactNode } from "react";
import { stringsIn } from "../locale";

export type T = ReturnType<typeof stringsIn>["t"];

/** The strings of the staff's screens, in the reader's locale, and the locale. */
export function useStaffStrings(): { t: T; locale: string } {
  const { locale } = useLocale();
  return { t: stringsIn(locale).t, locale };
}

/** Writes a moment as a date and a time in the reader's zone, never broken inside (no-break spaces). */
export function useMoment(): (iso: string) => string {
  const { locale } = useLocale();
  const zone = useTimeZone();
  const format = dateFormat(locale, zone, { dateStyle: "medium", timeStyle: "short" });
  return (iso) => format.format(new Date(iso)).replace(/ /g, " ");
}

/** The handle a sentence names an account by; the words for an account that is gone without one. */
export function handleIn(t: T, account: Account | undefined): string {
  return account === undefined ? t("staff.account_gone") : sentenceHandleOf(account);
}

/** A heading over what it heads, close under it. */
export function Titled({ title, children }: { title: string; children: ReactNode }) {
  return (
    <Stack gap="entry-line">
      <GroupHeading>{title}</GroupHeading>
      {children}
    </Stack>
  );
}

/** That a read loads (`loading`), or could not be read (`failed`) with Try again. */
export function ReadNotice({
  status,
  loading,
  failed,
  retry,
}: {
  status: "pending" | "error";
  loading: string;
  failed: string;
  retry: () => void;
}) {
  const { t } = useStaffStrings();
  if (status === "pending") return <FeedNotice lead>{loading}</FeedNotice>;
  return (
    <FeedNotice alert lead action={<TextButton onPress={retry}>{t("staff.retry")}</TextButton>}>
      {failed}
    </FeedNotice>
  );
}
