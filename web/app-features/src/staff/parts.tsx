// SPDX-License-Identifier: AGPL-3.0-only

// What the staff's screens share: their strings in the reader's locale, how a moment is written (the reader's zone, a
// date and its time held together), a heading over what it heads, a notice that a screen's read loads or failed, the
// status that says what a press did and takes the focus, and a field with its label and the error under it.

import { sentenceHandleOf } from "@my/app-components";
import type { Account } from "@my/app-data";
import {
  Box,
  CaptionLine,
  dateFormat,
  FeedNotice,
  FieldError,
  FieldLabel,
  FormAlert,
  GroupHeading,
  Stack,
  TextButton,
  useLocale,
  useTimeZone,
} from "@my/design-system";
import { useState, type ReactNode, type RefObject } from "react";
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

/** What a press said, in the screen's status. */
type Said = { readonly n: number; readonly words: string; readonly alert: boolean };

/**
 * The status of a screen, a polite live region there from the start (`StatusRegion`, whose element is `region`, made
 * by the component), which says what a press did, or that it failed, each time a new element, and takes the focus as
 * it does.
 */
export function useStatus(region: RefObject<HTMLDivElement | null>): {
  readonly said: Said | undefined;
  readonly say: (words: string, alert?: boolean) => void;
} {
  const [said, setSaid] = useState<Said | undefined>(undefined);
  return {
    said,
    say: (words, alert = false) => {
      setSaid((s) => ({ n: (s?.n ?? 0) + 1, words, alert }));
      requestAnimationFrame(() => region.current?.focus());
    },
  };
}

/** The region of `useStatus`, where it stands on the screen. */
export function StatusRegion({
  said,
  region,
}: {
  said: Said | undefined;
  region: RefObject<HTMLDivElement | null>;
}): ReactNode {
  return (
    <Box as={<div ref={region} role="status" tabIndex={-1} />}>
      {said !== undefined &&
        (said.alert ? (
          <FormAlert key={said.n} regular>
            {said.words}
          </FormAlert>
        ) : (
          <CaptionLine key={said.n} icon="accepted">
            {said.words}
          </CaptionLine>
        ))}
    </Box>
  );
}

/**
 * A control that has no label of its own (a choice from a list, a number) under its label, and what is wrong with its
 * value right under it, at `<id>-error`, which the control names among what describes it.
 */
export function Field({
  id,
  label,
  error,
  children,
}: {
  id: string;
  label: string;
  error?: string | undefined;
  children: ReactNode;
}): ReactNode {
  return (
    <Stack gap="entry-line">
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      {children}
      {error !== undefined && <FieldError id={`${id}-error`}>{error}</FieldError>}
    </Stack>
  );
}

/** The ids that describe the control of `id`: its error, while it has one. */
export const errorOf = (id: string, error: string | undefined): string | undefined =>
  error === undefined ? undefined : `${id}-error`;
