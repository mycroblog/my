// SPDX-License-Identifier: AGPL-3.0-only
// The pages of one screen (web-client §14): a row of links under the screen's `h1`, one per child route
// (`/admin/finance`, `/admin/finance/costs`), the current one `aria-current="page"`. They are links, not ARIA tabs:
// each page is a route of its own, with its own address, loader and primary action, and the menu of the admin mode
// keeps one item for the whole screen. The current page differs by shape, not by color alone (WCAG 1.4.1): a bar under
// its label. The links wrap onto more lines rather than scroll, so that every page is in sight at 320 px.
import type { ReactNode } from "react";
import { Link } from "react-aria-components";
import { tv } from "tailwind-variants";

const localNav = tv({
  slots: {
    list: "flex flex-wrap gap-x-1 gap-y-2 border-b border-border",
    item: "-mb-px flex",
    link: [
      "inline-flex min-h-10 cursor-pointer items-center border-b-3 border-transparent px-3 text-sm font-bold text-fg-muted",
      "rounded-t-md outline-none data-hovered:text-fg data-current:border-accent data-current:text-accent",
      "data-focus-visible:outline-2 data-focus-visible:-outline-offset-2 data-focus-visible:outline-focus",
      "pointer-coarse:min-h-11 wrap-anywhere motion-safe:transition-colors gap-2",
    ],
  },
});

/** One page of a screen. */
export type LocalNavItem = {
  /** The page's route. */
  href: string;
  /** The page's name, the link's text ("Costs"). */
  label: string;
  /** Whether it is the page on screen: `aria-current="page"`. */
  current?: boolean;
  /**
   * What waits on the page, after the label inside the link: a decorative `Badge` with the count in words for
   * assistive technology beside it (`VisuallyHidden`), such as the requests awaiting review.
   */
  badge?: ReactNode;
};

export type LocalNavProps = {
  /** The name of the landmark, the screen's name ("Finance pages"); the page has other navigation landmarks. */
  label: string;
  /** The screen's pages in order, its first page (the screen's own route) first. */
  items: readonly LocalNavItem[];
};

/** A `nav` of a screen's pages, under its `h1`. Navigation comes from the app's React Aria `RouterProvider`. */
export function LocalNav({ label, items }: LocalNavProps) {
  const styles = localNav();
  return (
    <nav aria-label={label}>
      <ul className={styles.list()}>
        {items.map((item) => (
          <li key={item.href} className={styles.item()}>
            <Link
              href={item.href}
              className={styles.link()}
              {...(item.current === true && { "aria-current": "page" as const })}
            >
              {item.label}
              {item.badge}
            </Link>
          </li>
        ))}
      </ul>
    </nav>
  );
}
