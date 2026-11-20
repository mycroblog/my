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
// Its few words (the pages, Previous, Next, Later and Earlier for a window of periods, Show more, Try again and what
// failed) are the design system's catalog's; the position of a window of periods is the app's, which formats it.

import { useLayoutEffect, useRef, type FocusEvent, type ReactNode, type RefCallback } from "react";
import { Button, LinkButton } from "./button";
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
   * ("Page 3"); else what the app shows, for a window of periods (the months shown, which it formats in the reader's
   * locale). None, the place stays empty.
   */
  position?: number | ReactNode;
  /**
   * A window of periods (the finance totals) in place of pages, by its periods' length: its two ways are Later (as
   * Previous, toward today) and Earlier (as Next), "Later months", "Earlier months".
   */
  periods?: "week" | "month" | "year" | undefined;
};

/**
 * What failed, over the buttons: one sentence in the text's color, a caption's size, said as it comes; it goes once the
 * reader asks again and the answer comes.
 */
function Failed({ children }: { children: string }) {
  return (
    <Box as="p" role="alert" text="caption" tone="text" breaks="phrase">
      {children}
    </Box>
  );
}

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
 * Where a press leaves the focus once its direction has no page (Next reaching the last page, Later the window that
 * ends today): the button that held it is disabled, which a browser drops to the page's body, so it goes to the other
 * direction's button, or to the position where that one is disabled too; not where it went elsewhere meanwhile.
 * `held` is the direction whose button held the focus, kept while the focus left it for nothing.
 */
function keepFocus(
  row: HTMLElement | null,
  place: HTMLElement | null,
  held: 0 | 1 | undefined,
  previousGone: boolean,
  nextGone: boolean,
) {
  if (row === null || held === undefined || !(held === 0 ? previousGone : nextGone)) return;
  const lost = row.children[held];
  const active = document.activeElement;
  if (active !== null && active !== document.body && !(lost?.contains(active) ?? false)) return;
  const other = row.children[held === 0 ? 1 : 0];
  if (other instanceof HTMLElement && !(held === 0 ? nextGone : previousGone)) other.focus();
  else place?.focus();
}

/**
 * Previous and Next under a list that is a screen's subject, paged by cursor, side by side at the list's start, either
 * disabled where there is no such page (the first page, the last); a press that reaches it leaves the focus on the
 * other direction (`keepFocus`). After them, which page is shown, a polite live region. A list of one page has none.
 * A navigation named for assistive technology.
 */
export function Pager({ previous, next, busy, failed, position, periods }: PagerProps) {
  const { lang } = useReader(undefined);
  const { t } = stringsIn(lang);
  const buttons = useRef<HTMLDivElement>(null);
  const place = useRef<HTMLSpanElement>(null);
  const held = useRef<0 | 1 | undefined>(undefined);
  const previousGone = previous === undefined;
  const nextGone = next === undefined;
  useLayoutEffect(() => {
    keepFocus(buttons.current, place.current, held.current, previousGone, nextGone);
  }, [previousGone, nextGone]);
  const onFocus = (event: FocusEvent<HTMLDivElement>) => {
    const at = [...event.currentTarget.children].findIndex((b) => b.contains(event.target));
    held.current = at === 0 || at === 1 ? at : undefined;
  };
  const onBlur = (event: FocusEvent<HTMLDivElement>) => {
    if (event.relatedTarget !== null) held.current = undefined;
  };
  if (previous === undefined && next === undefined) return null;
  const retry = t("records.retry");
  return (
    <Stack gap="screen-row-gap">
      {failed !== undefined && (
        <Failed>
          {periods === undefined
            ? t("records.page_failed", { direction: failed })
            : t("records.periods_failed", { direction: failed, granularity: periods })}
        </Failed>
      )}
      <Row as="nav" wrap align="center" gap="screen-tabs-gap" aria-label={t("records.pages")} data-pager="">
        <Row as={<div ref={buttons} onFocus={onFocus} onBlur={onBlur} />} wrap gap="entry-caption">
          <PageButton target={previous} busy={busy === "previous"} icon="previous" end={false}>
            {failed === "previous"
              ? retry
              : periods === undefined
                ? t("records.previous")
                : t("records.later", { granularity: periods })}
          </PageButton>
          <PageButton target={next} busy={busy === "next"} icon="onward" end>
            {failed === "next"
              ? retry
              : periods === undefined
                ? t("records.next")
                : t("records.earlier", { granularity: periods })}
          </PageButton>
        </Row>
        <Box text="caption" tone="muted" numeric>
          <span ref={place} role="status" tabIndex={-1}>
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
      {failed && <Failed>{t("records.more_failed")}</Failed>}
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
   * The ref of the item at `index` of the list (from 0), for its link or the row itself: the first item a press added
   * takes the focus once it is drawn, then no other; a row that takes no focus of its own is given `tabindex="-1"`
   * for good, so that the focus stays on it through the renders after.
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
            // An element that takes no focus of its own (a row) is given it, and keeps it: the ref goes once it has
            // focused, and a tabindex that went with it would send the focus to the page's body on the next render.
            if (element.tabIndex < 0 && !element.hasAttribute("tabindex")) element.setAttribute("tabindex", "-1");
            element.focus();
          }
        : undefined,
  };
}
