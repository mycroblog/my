// SPDX-License-Identifier: AGPL-3.0-only
import { useLocale } from "react-aria-components";
import { tv } from "tailwind-variants";
import { Button } from "./Button";
import { NextIcon, PreviousIcon } from "./Icons";
import { Link } from "./Link";

// One look for every item: a link (`href`) and a button (`onPress`) are drawn the same. `Link`'s `standalone`
// variant is the base; the classes here replace its color and weight.
const item = tv({
  base: [
    "inline-flex min-h-9 min-w-9 items-center justify-center gap-1 rounded-md border border-border bg-surface-raised px-3 font-bold text-fg no-underline",
    "pointer-coarse:min-h-11 pointer-coarse:min-w-11 [&>svg]:size-4 [&>svg]:shrink-0",
    "data-hovered:bg-surface-sunken data-hovered:text-fg data-hovered:no-underline",
  ],
  variants: {
    current: {
      true: "border-transparent bg-accent-fill text-accent-fill-fg data-hovered:bg-accent-fill-hover data-hovered:text-accent-fill-fg",
    },
  },
});

const ELLIPSIS = "\u2026";

type ItemProps = {
  to: PaginationTarget | null;
  current?: boolean;
  label?: string | undefined;
  children: React.ReactNode;
};

function Item({ to, current, label, children }: ItemProps) {
  const common = {
    className: item({ current }),
    ...(label !== undefined && { "aria-label": label }),
    ...(current && { "aria-current": "page" as const }),
  };
  if (to?.href !== undefined) {
    return (
      <Link {...common} variant="standalone" href={to.href}>
        {children}
      </Link>
    );
  }
  return (
    <Button
      {...common}
      variant="ghost"
      size="md"
      isDisabled={to === null}
      {...(to?.onPress && { onPress: to.onPress })}
    >
      {children}
    </Button>
  );
}

/** Where an item leads: a link when it has `href`, a button otherwise. */
export type PaginationTarget = { href?: string; onPress?: () => void };

export type PaginationProps = {
  /** The accessible name of the landmark ("Pages", "Search results"). */
  label: string;
  /** The text of the previous end ("Previous"); the arrow is drawn beside it. */
  previousLabel: string;
  /** The text of the next end ("Next"). */
  nextLabel: string;
  /** The current page, counted from 1. Omit with `pageCount` for a list that pages by cursor. */
  page?: number;
  /** The number of pages. Omit it to show only the two ends. */
  pageCount?: number;
  /** The accessible name of a page's button or link ("Page 3"); the number is read when omitted. */
  pageLabel?: (page: number) => string;
  /** Makes each page and end a link to this address. */
  getHref?: (page: number) => string;
  /** Called with the page a button leads to, when `getHref` is not given. */
  onPageChange?: (page: number) => void;
  /** How many pages show on each side of the current page before they are folded into an ellipsis. */
  siblings?: number;
  /**
   * The previous end, when it is not simply `page - 1`: a cursor list gives the address or handler of
   * the previous slice, or `null` when there is none (the end is then disabled). Wins over `page`.
   */
  previous?: PaginationTarget | null;
  /** The next end; see `previous`. */
  next?: PaginationTarget | null;
};

type Entry = number | "start-ellipsis" | "end-ellipsis";

/** The pages to draw: the first, the last, the current one and its neighbors, with an ellipsis for each gap. */
export function pageEntries(page: number, pageCount: number, siblings: number): Entry[] {
  const total = siblings * 2 + 5;
  if (pageCount <= total) return Array.from({ length: pageCount }, (_, i) => i + 1);
  const left = Math.max(page - siblings, 1);
  const right = Math.min(page + siblings, pageCount);
  const showStart = left > 3;
  const showEnd = right < pageCount - 2;
  const run = (from: number, to: number) => Array.from({ length: to - from + 1 }, (_, i) => from + i);
  if (!showStart) return [...run(1, siblings * 2 + 3), "end-ellipsis", pageCount];
  if (!showEnd) return [1, "start-ellipsis", ...run(pageCount - (siblings * 2 + 2), pageCount)];
  return [1, "start-ellipsis", ...run(left, right), "end-ellipsis", pageCount];
}

/**
 * A page stepper for lists that page rather than scroll (search results, admin tables). The current page
 * carries `aria-current="page"`; an end with nowhere to go is disabled. Pages are links with `getHref`
 * and buttons with `onPageChange`; a list that pages by cursor passes `previous` and `next` and no `page`.
 */
export function Pagination({
  label,
  previousLabel,
  nextLabel,
  page,
  pageCount,
  pageLabel,
  getHref,
  onPageChange,
  siblings = 1,
  previous,
  next,
}: PaginationProps) {
  const { locale } = useLocale();
  const format = new Intl.NumberFormat(locale);

  const target = (to: number): PaginationTarget =>
    getHref ? { href: getHref(to) } : { onPress: () => onPageChange?.(to) };
  const hasPages = page !== undefined && pageCount !== undefined;
  const previousTarget = previous !== undefined ? previous : hasPages && page > 1 ? target(page - 1) : null;
  const nextTarget = next !== undefined ? next : hasPages && page < pageCount ? target(page + 1) : null;

  return (
    <nav aria-label={label}>
      <ul className="flex flex-wrap gap-1.5">
        <li>
          <Item to={previousTarget}>
            <PreviousIcon />
            {previousLabel}
          </Item>
        </li>
        {hasPages &&
          pageEntries(page, pageCount, siblings).map((entry) =>
            typeof entry === "string" ? (
              <li
                key={entry}
                aria-hidden="true"
                className="inline-flex min-h-9 min-w-9 items-center justify-center text-fg-muted"
              >
                {ELLIPSIS}
              </li>
            ) : (
              <li key={entry}>
                <Item to={target(entry)} current={entry === page} label={pageLabel?.(entry)}>
                  {format.format(entry)}
                </Item>
              </li>
            ),
          )}
        <li>
          <Item to={nextTarget}>
            {nextLabel}
            <NextIcon />
          </Item>
        </li>
      </ul>
    </nav>
  );
}
