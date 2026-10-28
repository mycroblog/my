// SPDX-License-Identifier: AGPL-3.0-only
// The grouped menu of the settings and of the admin and moderation mode (web-client §21, ADR-0110 §5): the pages in
// named groups, each page a row with its icon, its label and an optional count. The groups are what tells the pages
// apart, so every group has a visible heading and every page an icon (G12: the label carries the meaning, the icon
// lets the eye find it); a group holds at most `navMenuGroupLimit` pages, and a longer one is split. The menu brings
// no landmark: the frame around it (`SettingsLayout`, `AdminShell`) does.
//
// A group is a `group` named by its heading, not a `section` with an `h2`: the page beside the menu owns the
// headings (its `h1` and the `h2` of its sections), and the menu must not come first in their order.
// `@my/ui/testing`'s `watchNavMenus` checks every menu a test of the app renders against the two rules.
import { type ReactElement, useId } from "react";
import { tv } from "tailwind-variants";
import { RailItem, type RailItemBadge } from "./RailItem";

/** The most pages a group of the menu holds (ADR-0110 §6); a longer group is split in two. */
export const navMenuGroupLimit = 7;

const navMenu = tv({
  slots: {
    base: "flex flex-col gap-4",
    group: "flex flex-col gap-1",
    heading: "px-3 pb-1 text-xs font-extrabold text-fg-muted",
    list: "flex flex-col gap-1",
  },
});

/** One page of the menu. */
export type NavMenuItem = {
  href: string;
  /** The page's icon, one of the kit's icons (`ReportIcon`); every page has one. */
  icon: ReactElement;
  /** The visible label, the link's name without a count. */
  label: string;
  /** Whether the page is the one the user is on: `aria-current="page"`. */
  current?: boolean;
  /** What awaits on the page (open reports), at the end of the row; none while nothing does. */
  badge?: RailItemBadge;
};

/** One named group of the menu's pages. */
export type NavMenuGroup = {
  /** The group's key among the groups. */
  id: string;
  /** The group's visible name, which also names it for assistive technology ("Security"). */
  heading: string;
  /** The group's pages, at most `navMenuGroupLimit`; a group without pages is not shown. */
  items: readonly NavMenuItem[];
};

export type NavMenuProps = {
  groups: readonly NavMenuGroup[];
};

function Group({ heading, items }: NavMenuGroup) {
  const slots = navMenu();
  const id = useId();
  return (
    <div role="group" aria-labelledby={id} data-nav-menu-group="" className={slots.group()}>
      <p id={id} className={slots.heading()}>
        {heading}
      </p>
      <ul className={slots.list()}>
        {items.map((item) => (
          <li key={item.href}>
            <RailItem
              form="section"
              href={item.href}
              icon={item.icon}
              label={item.label}
              current={item.current ?? false}
              {...(item.badge !== undefined && { badge: item.badge })}
            />
          </li>
        ))}
      </ul>
    </div>
  );
}

/** The pages of a menu in named groups, each with its icon and its count; groups without pages are left out. */
export function NavMenu({ groups }: NavMenuProps) {
  const slots = navMenu();
  return (
    <div className={slots.base()}>
      {groups
        .filter((group) => group.items.length > 0)
        .map((group) => (
          <Group key={group.id} {...group} />
        ))}
    </div>
  );
}
