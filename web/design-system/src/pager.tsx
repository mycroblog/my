// SPDX-License-Identifier: AGPL-3.0-only

// The one pager of the web client's lists (#3279), by the kind of list: a list that is a screen's subject (a staff
// queue) pages with Previous and Next (`Pager`), the screen keeping its place in the address; a list inside a detail
// grows in place under Show more (`ShowMore`); a feed loads as the reader scrolls (feed.tsx), with neither.
//
// Both look alike: under the list, at its start, where its rows start, the timeline's words on the raised surface in a
// finger's target at every pointer, a mouse pressing them as often as a finger. Previous and Next sit close, Previous
// first, their chevrons pointing the way they go and mirrored in right-to-left, so that Next stays in its place page
// after page; after them, muted, which page is shown ("Page 3"), counted by the app from its trail, since the server
// pages by cursor and gives no total; on a line of its own where the line has no room for it. A list of one page has
// no pager, and a list read to its end no Show more.
//
// A button stays where it stood whatever happens, so that the focus it holds stays with it: a direction there is no
// page in is disabled, not gone; one on its way is busy, keeping its width; one that failed is the same button,
// named Try again, under the sentence that says what failed. Show more is gone once the list is read to its end, the
// focus having moved by then to the first item it added (`useAddedFocus`), where the reader goes on.
//
// Its few words (the pages, Previous, Next, Show more, Try again and what failed) are the design system's catalog's;
// a pager of another window than pages (the periods of the finance screens) names its two ways itself.

import { useRef, type ReactNode, type RefCallback } from "react";
import { Button, LinkButton } from "./button";
import { FeedNotice } from "./feed";
import { Icon, type IconName } from "./icons";
import { Box, Row, Stack } from "./layout";
import { stringsIn, useReader } from "./locale";

/** Where a page leads: an address, or what the screen does to show it. */
export type PageTarget = { href: string } | { onPress: () => void };

export type PagerProps = {
  /** The page before, if any (the first page has none): newer items, in a list of the latest first. */
  previous?: PageTarget | undefined;
  /** The page after, if any (the last page has none): older items, in a list of the latest first. */
  next?: PageTarget | undefined;
  /** The page asked for is on its way: its button busy (a target that is a press; an address loads as a screen). */
  busy?: "previous" | "next" | undefined;
  /**
   * The page asked for could not be read: the sentence that says so over the pager, and that direction's button named
   * Try again, its target asking for the page again.
   */
  failed?: "previous" | "next" | undefined;
  /**
   * Which page is shown, after the two buttons and said as it changes: a number, counted from 1 by the app's trail
   * ("Page 3"); words, for a window of something else than pages (the months shown). None, the place stays empty.
   */
  position?: number | string | undefined;
  /** The words of the two ways where they are not Previous and Next (Later months, Earlier months). */
  labels?: { previous: string; next: string } | undefined;
};

/** A page's button; where there is no such page, disabled, so that the focus it holds stays where it was. */
function PageButton({
  target,
  busy,
  icon,
  end,
  children,
}: {
  target: PageTarget | undefined;
  busy: boolean;
  icon: IconName;
  end: boolean;
  children: string;
}) {
  const label = (
    <Row as="span" inline align="center" gap="entry-caption">
      {!end && <Icon name={icon} place="caption" />}
      <span>{children}</span>
      {end && <Icon name={icon} place="caption" />}
    </Row>
  );
  if (target === undefined)
    return (
      <Button size="target" isDisabled>
        {label}
      </Button>
    );
  if ("href" in target)
    return (
      <LinkButton href={target.href} size="target">
        {label}
      </LinkButton>
    );
  return (
    <Button size="target" busy={busy} onPress={target.onPress}>
      {label}
    </Button>
  );
}

/**
 * Previous and Next under a list that is a screen's subject, paged by cursor, side by side at the list's start, either
 * disabled where there is no such page (the first page, the last), so that a press that reaches the last page leaves
 * the focus on its button; after them, which page is shown, a polite live region. A list of one page has none. A
 * navigation named for assistive technology.
 */
export function Pager({ previous, next, busy, failed, position, labels }: PagerProps) {
  const { lang } = useReader(undefined);
  const { t } = stringsIn(lang);
  if (previous === undefined && next === undefined) return null;
  const retry = t("records.retry");
  return (
    <Stack gap="screen-row-gap">
      {failed !== undefined && (
        <FeedNotice alert start>
          {t("records.page_failed")}
        </FeedNotice>
      )}
      <Row as="nav" wrap align="center" gap="screen-tabs-gap" aria-label={t("records.pages")}>
        <Row wrap gap="entry-caption">
          <PageButton target={previous} busy={busy === "previous"} icon="previous" end={false}>
            {failed === "previous" ? retry : (labels?.previous ?? t("records.previous"))}
          </PageButton>
          <PageButton target={next} busy={busy === "next"} icon="onward" end>
            {failed === "next" ? retry : (labels?.next ?? t("records.next"))}
          </PageButton>
        </Row>
        <Box text="caption" tone="muted" numeric>
          <span role="status">
            {typeof position === "number" ? t("records.page", { page: position }) : (position ?? "")}
          </span>
        </Box>
      </Row>
    </Stack>
  );
}

export type ShowMoreProps = {
  /** The list has more to show; without, it is read to its end and there is no Show more. */
  more: boolean;
  /** Asks for the next items; `useAddedFocus().press` wraps it so that the focus goes to the first of them. */
  onPress: () => void;
  /** The next items are on their way: the button busy. */
  busy?: boolean | undefined;
  /** The next items could not be read: the sentence that says so, and the button named Try again, pressing the same. */
  failed?: boolean | undefined;
};

/**
 * Show more, under a list that grows in place (a list inside a detail), at the list's start: busy while the next items
 * are on their way; after they failed, the same button named Try again under the sentence that says so. Nothing once
 * the list is read to its end.
 */
export function ShowMore({ more, onPress, busy = false, failed = false }: ShowMoreProps) {
  const { lang } = useReader(undefined);
  const { t } = stringsIn(lang);
  if (!more) return null;
  return (
    <Stack gap="screen-row-gap">
      {failed && (
        <FeedNotice alert start>
          {t("records.more_failed")}
        </FeedNotice>
      )}
      <Row justify="start">
        <Button size="target" busy={busy} onPress={onPress}>
          {t(failed ? "records.retry" : "records.more")}
        </Button>
      </Row>
    </Stack>
  );
}

/** What `useAddedFocus` gives a list that grows under Show more. */
export type AddedFocus = {
  /** The press of Show more: it notes where the items to come start, then asks for them with `load`. */
  press: (load: () => void) => () => void;
  /**
   * The ref of the item at `index` of the list (from 0), for its focusable element (its link, or the row itself with
   * `tabIndex={-1}`): the first item a press added takes the focus once it is drawn, then no other.
   */
  item: (index: number) => RefCallback<HTMLElement> | undefined;
};

/**
 * The focus of a list that grows under Show more, `shown` the number of items it shows: after a press, the first item
 * added takes it, so that the reader goes on from there, and Show more may go once the list is read to its end. A
 * press that fails adds nothing and leaves the focus on its button, which Try again then holds.
 */
export function useAddedFocus(shown: number): AddedFocus {
  const from = useRef<number | undefined>(undefined);
  return {
    press: (load) => () => {
      from.current = shown;
      load();
    },
    item: (index) =>
      index === from.current
        ? (element) => {
            if (element === null || from.current !== index) return;
            from.current = undefined;
            element.focus();
          }
        : undefined,
  };
}

export type RecordPagesProps = Pick<PagerProps, "previous" | "next" | "busy">;

/** Previous and Next as a list paged by cursor wrote them before the pager: the pager, saying no position. */
export function RecordPages(props: RecordPagesProps): ReactNode {
  return <Pager {...props} />;
}

/** Show more as a list that grows in place wrote it before the pager, shown only while it has more: the same part. */
export function MoreRecords({ onPress, busy }: { onPress: () => void; busy?: boolean | undefined }): ReactNode {
  return <ShowMore more onPress={onPress} busy={busy} />;
}
