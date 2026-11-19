// SPDX-License-Identifier: AGPL-3.0-only

// What the staff's screens share: their strings in the reader's locale, how a moment is written (the reader's zone, a
// date and its time held together), a heading over what it heads, a notice that a screen's read loads or failed, the
// pages of a list by the server's cursor with the way back along them kept in the address, and the status a press says
// what it did in.

import { sentenceHandleOf } from "@my/app-components";
import type { Account } from "@my/app-data";
import {
  Box,
  CaptionLine,
  dateFormat,
  FeedNotice,
  FormAlert,
  GroupHeading,
  RecordPages,
  Stack,
  TextButton,
  useLocale,
  useTimeZone,
} from "@my/design-system";
import { useRef, useState, type ReactNode } from "react";
import type { StaffAddress, StaffSearch } from "./entries";
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

// The pages of a list by the server's cursor, kept in the address, and the status a press says what it did in.

/** The cursor of the page an address's query asks for; the first page without one. */
export const cursorOf = (search: StaffSearch): string | undefined =>
  search["cursor"] === undefined || search["cursor"] === "" ? undefined : search["cursor"];

/** The cursors of the pages before the one shown, the first page's as empty: the way back by Previous. */
const trailOf = (search: StaffSearch): string[] =>
  search["trail"] === undefined ? [] : search["trail"].split(",").map((c) => decodeURIComponent(c));

/** `search` at the page of `cursor`, after the `trail` of pages before it. */
function atPage(search: StaffSearch, cursor: string | undefined, trail: readonly string[]): StaffSearch {
  const { cursor: _cursor, trail: _trail, ...rest } = search;
  return {
    ...rest,
    ...(cursor === undefined ? {} : { cursor }),
    ...(trail.length === 0 ? {} : { trail: trail.map((c) => encodeURIComponent(c)).join(",") }),
  };
}

/** `search` with `change` made and at the first page, as a change of the filters starts. */
export function filtered(search: StaffSearch, change: StaffSearch): StaffSearch {
  return atPage({ ...search, ...change }, undefined, []);
}

/** Previous and Next under a list, along the pages of the address of `address`, Next to the page of `next`. */
export function Pages({ address, next }: { address: StaffAddress; next: string | undefined }) {
  const { search, onSearch } = address;
  const trail = trailOf(search);
  const previous = trail.at(-1);
  return (
    <RecordPages
      previous={
        previous === undefined
          ? undefined
          : {
              onPress: () => {
                onSearch(atPage(search, previous === "" ? undefined : previous, trail.slice(0, -1)));
              },
            }
      }
      next={
        next === undefined
          ? undefined
          : {
              onPress: () => {
                onSearch(atPage(search, next, [...trail, cursorOf(search) ?? ""]));
              },
            }
      }
    />
  );
}

/** What a press said: its words, whether they say it failed, and a count that makes each saying a new element. */
type Said = { readonly n: number; readonly words: string; readonly alert: boolean };

/**
 * A status of a screen that says what a press did, or that it failed: `say` puts the words in it and moves the focus
 * there, since the press may have taken its own button away; `status` is the region, there from the start.
 */
export function useSaid(): { say: (words: string, alert?: boolean) => void; status: ReactNode } {
  const [said, setSaid] = useState<Said | undefined>(undefined);
  const ref = useRef<HTMLDivElement>(null);
  const say = (words: string, alert = false) => {
    setSaid((s) => ({ n: (s?.n ?? 0) + 1, words, alert }));
    requestAnimationFrame(() => ref.current?.focus());
  };
  const status = (
    <Box grow as={<div ref={ref} role="status" tabIndex={-1} />}>
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
  return { say, status };
}
