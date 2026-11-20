// SPDX-License-Identifier: AGPL-3.0-only

// What the staff's screens share: their strings in the reader's locale, how a moment is written (the reader's zone, a
// date and its time held together), a heading over what it heads, a notice that a screen's read loads or failed, the
// pages of a list by the server's cursor, kept in the address with the way back along them (`useAddressPages`), and
// the status a press says what it did in.

import { sentenceHandleOf } from "@my/app-components";
import type { Account, PagedList } from "@my/app-data";
import {
  Box,
  CaptionLine,
  dateFormat,
  FeedNotice,
  FieldError,
  FieldLabel,
  FormAlert,
  GroupHeading,
  ListTitle,
  ShowMore,
  Stack,
  TextButton,
  useLocale,
  useTimeZone,
  type AddedFocus,
  type PagerProps,
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

/** Writes a moment's date alone in the reader's zone, held together: a list's lines carry the day, not the time. */
export function useMomentDay(): (iso: string) => string {
  const { locale } = useLocale();
  const zone = useTimeZone();
  const format = dateFormat(locale, zone, { dateStyle: "medium" });
  return (iso) => format.format(new Date(iso)).replace(/ /g, "\u00a0");
}

/** The handle a sentence names an account by; the words for an account that is gone without one. */
export function handleIn(t: T, account: Account | undefined): string {
  return account === undefined ? t("staff.account_gone") : sentenceHandleOf(account);
}

/**
 * A section of a screen that holds fields or what is decided on (the form of a new action, the decision of an appeal),
 * under a title of the list's size: never smaller than the labels of the fields under it.
 */
export function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <Stack gap="entry-room">
      <ListTitle>{title}</ListTitle>
      {children}
    </Stack>
  );
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

/**
 * The names a list keeps its page under in the address's query: its cursor's, and its trail's (the cursors of the pages
 * before, the way back by Previous). A screen's first list keeps them as `cursor` and `trail`; a second list on the same
 * screen under names of its own, so that each keeps its place.
 */
export type PageNames = { readonly cursor: string; readonly trail: string };

/** The names of a screen's first list. */
const firstList: PageNames = { cursor: "cursor", trail: "trail" };

/** The cursor of the page an address's query asks for, of the list of `of`; the first page without one. */
export const cursorOf = (search: StaffSearch, of: PageNames = firstList): string | undefined =>
  search[of.cursor] === undefined || search[of.cursor] === "" ? undefined : search[of.cursor];

/**
 * The cursors of the pages before the one shown, the first page's as empty (so that the second page's trail is the
 * empty word): the way back by Previous.
 */
const trailOf = (search: StaffSearch, of: PageNames): string[] => {
  const trail = search[of.trail];
  return trail === undefined ? [] : trail.split(",").map((c) => decodeURIComponent(c));
};

/** `search` at the page of `cursor`, after the `trail` of pages before it, under the names `of` the list. */
function atPage(search: StaffSearch, cursor: string | undefined, trail: readonly string[], of: PageNames): StaffSearch {
  const rest = Object.fromEntries(Object.entries(search).filter(([k]) => k !== of.cursor && k !== of.trail));
  return {
    ...rest,
    ...(cursor === undefined ? {} : { [of.cursor]: cursor }),
    ...(trail.length === 0 ? {} : { [of.trail]: trail.map((c) => encodeURIComponent(c)).join(",") }),
  };
}

/** `search` with `change` made and at the first page of its first list, as a change of the filters starts. */
export function filtered(search: StaffSearch, change: StaffSearch): StaffSearch {
  return atPage({ ...search, ...change }, undefined, [], firstList);
}

/** Where a list of a screen is along its pages, kept in the address (`useAddressPages`). */
export type AddressPages = {
  /** The cursor of the page shown, to read it by; undefined on the first page. */
  readonly cursor: string | undefined;
  /** Which page is shown, from 1: the trail's length and one. */
  readonly position: number;
  /**
   * The pager's props under the page shown, Next to the page of `next` (the page's `next_cursor`; none on the last
   * page): each press shows the screen at the other page as a new entry of the history, so that Back, a reload and a
   * shared address land on the same page.
   */
  readonly pager: (next: string | undefined) => PagerProps;
};

/**
 * The one place a list that is a screen's subject keeps its cursor: in the address of `address`, under the names `of`
 * the list (a screen's first list where left out), with the trail of the pages before it for Previous.
 */
export function useAddressPages(
  address: Pick<StaffAddress, "search" | "onSearch">,
  of: PageNames = firstList,
): AddressPages {
  const { search, onSearch } = address;
  const cursor = cursorOf(search, of);
  const trail = trailOf(search, of);
  const previous = trail.at(-1);
  // A page after the first whose trail the address lost (an address cut short) is at least the second.
  const position = cursor === undefined ? 1 : Math.max(trail.length, 1) + 1;
  return {
    cursor,
    position,
    pager: (next) => ({
      previous:
        cursor === undefined
          ? undefined
          : {
              onPress: () => {
                onSearch(atPage(search, previous === "" ? undefined : previous, trail.slice(0, -1), of));
              },
            },
      next:
        next === undefined
          ? undefined
          : {
              onPress: () => {
                onSearch(atPage(search, next, [...trail, cursor ?? ""], of));
              },
            },
      position,
    }),
  };
}

/**
 * Show more under a list inside a detail that grows in place (`list`, read by pages of app-data), busy while its next
 * page comes and Try again after it failed; `added` (`useAddedFocus` of the items shown) moves the focus to the first
 * item a press added.
 */
export function ListMore({ list, added }: { list: PagedList<unknown>; added: AddedFocus }) {
  return (
    <ShowMore
      more={list.hasNextPage}
      busy={list.isFetchingNextPage}
      failed={list.failedNextPage}
      onPress={added.press(list.fetchNextPage)}
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

/** A payload as its lines are best read (a job's, a webhook's): a JSON document laid out, anything else as it is. */
export function shownPayload(payload: string): string {
  try {
    return JSON.stringify(JSON.parse(payload), null, 2);
  } catch {
    return payload;
  }
}
