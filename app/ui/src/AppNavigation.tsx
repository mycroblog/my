// SPDX-License-Identifier: AGPL-3.0-only
// The app's main navigation, one landmark for every width (web-client/README.md D1, D2, ADR-0039): a tab bar on the bottom
// edge of phones, a side rail of icons with short labels from md, and a sidebar with full labels from lg. Like
// AppShell, it answers to the viewport (the shell is the one place for viewport breakpoints, D2).
//
// The items are one list in one order at every width; each item hides where it does not belong (`shows`, with
// display: none, so it leaves the accessibility tree too). A count shows as a badge on the icon's corner: its digits
// are hidden from assistive technology and the link is named with the count instead ("Notifications, 3 unread"), a
// name that starts with the visible label (WCAG 2.5.3). The tab bar's last item can be a menu (`AppNavMenu`), a
// button that opens a sheet with the items that only the rail shows.
// A line (`separated` on the first of them) sets the staff's areas apart from the user's items, in the rail and in the sheet.
import { createContext, type ReactNode, use, useId } from "react";
import { tv } from "tailwind-variants";
import { Button } from "./Button";
import { Badge } from "./Badge";
import { Sheet, DialogTrigger } from "./Dialog";
import { MenuIcon } from "./Icons";
import { Link } from "./Link";
import { PostCardAuthor } from "./PostCard";
import { Wordmark } from "./Wordmark";
import { RailItem, type RailItemBadge } from "./RailItem";

const appNav = tv({
  slots: {
    base: ["z-10 bg-surface-raised px-[max(env(safe-area-inset-left),env(safe-area-inset-right))]"],
    header: "mx-2 hidden px-3 pt-4 pb-2 lg:block",
    list: "flex",
    // The rail's main action (from md); below md the page floats its own button (`AppShell` `floating`).
    action: "hidden p-2 md:block",
  },
  variants: {
    // `side`: the tab bar below md, the rail from md and the sidebar from lg. `top`: the signed-out top bar.
    placement: {
      side: {
        base: [
          "fixed inset-x-0 bottom-0 border-t border-border pb-[env(safe-area-inset-bottom)]",
          "md:sticky md:top-under-bar md:bottom-auto md:flex md:h-svh-under-bar md:flex-col md:overflow-y-auto md:border-t-0 md:border-e md:bg-surface md:px-0 md:pb-0 lg:w-60",
        ],
        list: "md:flex-col md:gap-1 md:p-2",
      },
      top: {
        base: "sticky top-under-bar flex flex-wrap items-center gap-x-4 gap-y-1 border-b border-border px-4 py-2",
        header: "mx-0 block p-0",
        list: "flex-wrap items-center gap-1",
      },
    },
  },
  defaultVariants: { placement: "side" },
});

export type AppNavProps = {
  /** The landmark's name ("Main"). */
  label: string;
  /** Above the list in the sidebar (lg) and at the start of the top bar: the wordmark and the server's name. */
  header?: ReactNode;
  /** The items: `AppNavItem`s and, last, an `AppNavMenu`. */
  children: ReactNode;
  /** After the list from md, before the account: the rail's one main action (`AppNavAction`). */
  action?: ReactNode;
  /** Under the list in the sidebar (lg) only: the signed-in account (`AppNavAccount`). */
  footer?: ReactNode;
  /** `side` (default): tab bar, rail and sidebar. `top`: a light bar along the top for signed-out pages (`AppShell` `navigationPlacement="top"`). */
  placement?: "side" | "top";
};

const PlacementContext = createContext<"side" | "top">("side");

/**
 * The `nav` landmark and its list. The header and the footer are outside the list, so the list's links stay the same
 * and in the same order at every width.
 */
export function AppNav({ label, header, children, action, footer, placement = "side" }: AppNavProps) {
  const slots = appNav({ placement });
  return (
    <PlacementContext value={placement}>
      <nav aria-label={label} className={slots.base()}>
        {header !== undefined && <div className={slots.header()}>{header}</div>}
        <ul className={slots.list({ className: placement === "top" ? "ms-auto" : undefined })}>{children}</ul>
        {action !== undefined && <div className={slots.action()}>{action}</div>}
        {footer}
      </nav>
    </PlacementContext>
  );
}

export type AppNavBrandProps = {
  /** The server's name (`ServerNameLink`), which names the brand: the wordmark beside it is decorative. */
  children: ReactNode;
};

/** The wordmark with the server's name beside it: the start of the sidebar and of the signed-out top bar. */
export function AppNavBrand({ children }: AppNavBrandProps) {
  return (
    <div className="flex min-w-0 items-center gap-2">
      <Wordmark name="my" decorative size="sm" />
      {children}
    </div>
  );
}

const appNavItem = tv({
  slots: {
    // In the tab bar each item is as wide as its label and the room left is shared out evenly (flex: 1 1 auto), so
    // that a long label ("Notifications") takes the room that short ones ("Menu") leave and fits a 320 px screen.
    base: "min-w-0",
    // The menu button wears the classes of a `RailItem` in the tab bar.
    item: [
      "group flex min-h-16 w-full flex-col items-center justify-center gap-1 rounded-none px-0.5 text-xs font-bold text-fg-muted",
      "data-hovered:bg-surface-sunken pointer-coarse:min-h-16 short:min-h-12 [&_svg]:size-6 [&_svg]:shrink-0",
    ],
    // What the tab bar's item becomes from md (a compact column of icon over label) and from lg (a row, the count at
    // the end), and on short screens.
    railItem: [
      "short:min-h-12 md:w-20 md:rounded-lg md:text-fg md:data-current:rounded-lg md:data-current:bg-soft md:data-current:text-accent md:data-current:data-hovered:bg-soft",
      "lg:min-h-12 lg:w-auto lg:flex-row lg:justify-start lg:gap-4 lg:px-4 lg:text-base",
    ],
    railPill: "lg:contents",
    railBadge: "md:ring-surface lg:static lg:order-last lg:ms-auto lg:ring-0",
    railLabel: "lg:text-start",
    pill: "relative",
    badge: "absolute -top-1.5 -inset-e-1 ring-2 ring-surface-raised",
    label: "max-w-full text-center leading-tight text-balance wrap-anywhere",
  },
  variants: {
    shows: {
      always: { base: "flex-auto md:flex-none" },
      "tab-bar": { base: "flex-auto md:hidden" },
      rail: { base: "hidden md:block" },
    },
    // In the top bar every item shows, at every width.
    placement: { side: {}, top: { base: "flex-none" } },
    // The first item of the staff's areas, set apart from the user's items by a line in the rail and the sidebar.
    separated: { true: { base: "md:mt-2 md:border-t md:border-border md:pt-3" } },
  },
  compoundVariants: [{ placement: "top", shows: "rail", class: { base: "block" } }],
});

/** A count on an item: the number, what it means, and the name of the item while it shows it. */
export type AppNavBadge = RailItemBadge;

export type AppNavItemProps = {
  href: string;
  /** The item's icon (one of the kit's icons); the item sizes it. */
  icon: ReactNode;
  /** The visible label, the link's name without a count. */
  label: string;
  /** Whether the link is the place the user is in: `aria-current="page"`, the soft fill and the accent label. */
  current?: boolean;
  /** The count on the icon's corner, or nothing for an item without a count now. */
  badge?: AppNavBadge;
  /** Where the item shows: everywhere, only in the tab bar (below md), or only in the rail and the sidebar (from md). In the top bar every item shows. */
  shows?: "always" | "tab-bar" | "rail";
  /** In the top bar, the call to action (a coral pill). */
  prominent?: boolean;
  /** Sets the item and those after it apart from the items before it with a line (the staff's areas after the user's items). */
  separated?: boolean;
};

/** One item of the navigation: a `RailItem` in a list item. */
export function AppNavItem({
  href,
  icon,
  label,
  current = false,
  badge,
  shows = "always",
  prominent,
  separated = false,
}: AppNavItemProps) {
  const placement = use(PlacementContext);
  const slots = appNavItem({ shows, placement, separated });
  const side = placement === "side";
  return (
    <li className={slots.base()}>
      <RailItem
        href={href}
        icon={icon}
        label={label}
        current={current}
        form={side ? "tab" : "top"}
        {...(side && {
          classNames: {
            item: slots.railItem(),
            pill: slots.railPill(),
            badge: slots.railBadge(),
            label: slots.railLabel(),
          },
        })}
        {...(badge !== undefined && { badge })}
        {...(prominent !== undefined && { prominent })}
      />
    </li>
  );
}

export type AppNavMenuProps = {
  /** The button's visible label ("Menu"). */
  label: string;
  /** The sum of the counts of the sheet's items, with the button's name that says it ("Menu, 3 pending in moderation"). */
  badge?: AppNavBadge;
  isOpen: boolean;
  onOpenChange: (isOpen: boolean) => void;
  /** The sheet's title, its accessible name. */
  title: string;
  /** The sheet's close button's label. */
  closeLabel: string;
  /** The sheet's content: an `AppNavMenuList` of the rail's items, then `AppNavMenuSection`s. */
  children: ReactNode;
};

/**
 * The tab bar's last item (below md only): a button with the items' look that opens a sheet (a modal dialog, WAI-ARIA
 * APG). The button is not a place, so it never has `aria-current`; the sheet marks the current item.
 */
export function AppNavMenu({ label, badge, isOpen, onOpenChange, title, closeLabel, children }: AppNavMenuProps) {
  const slots = appNavItem({ shows: "tab-bar" });
  return (
    <li className={slots.base()}>
      <DialogTrigger isOpen={isOpen} onOpenChange={onOpenChange}>
        <Button variant="ghost" className={slots.item()} {...(badge !== undefined && { "aria-label": badge.name })}>
          <span className={slots.pill()}>
            <MenuIcon />
            {badge !== undefined && <Badge count={badge.count} label={badge.label} className={slots.badge()} />}
          </span>
          <span className={slots.label()}>{label}</span>
        </Button>
        <Sheet title={title} closeLabel={closeLabel}>
          {children}
        </Sheet>
      </DialogTrigger>
    </li>
  );
}

const appNavMenu = tv({
  slots: {
    list: "flex flex-col",
    item: "",
    link: [
      "flex min-h-12 w-full gap-3 rounded-md px-3 text-base font-normal text-fg data-hovered:bg-surface-sunken",
      "aria-[current=page]:font-semibold aria-[current=page]:text-accent",
    ],
  },
  variants: {
    // The first item of the staff's areas, set apart from the user's items by a line.
    separated: { true: { item: "mt-2 border-t border-border pt-2" } },
  },
});

export type AppNavMenuListProps = { children: ReactNode };

/** The list of the menu sheet's items (`AppNavMenuLink`). */
export function AppNavMenuList({ children }: AppNavMenuListProps) {
  return <ul className={appNavMenu().list()}>{children}</ul>;
}

export type AppNavMenuLinkProps = Omit<AppNavItemProps, "shows">;

/** An item of the menu sheet: a full-width row with the icon, the label and the count after it. */
export function AppNavMenuLink({ href, icon, label, current = false, badge, separated = false }: AppNavMenuLinkProps) {
  return (
    <li className={appNavMenu().item({ separated })}>
      <Link
        href={href}
        {...(badge !== undefined && { "aria-label": badge.name })}
        {...(current && { "aria-current": "page" as const })}
        variant="navigation"
        className={appNavMenu().link()}
      >
        {icon}
        {label}
        {badge !== undefined && <Badge count={badge.count} label={badge.label} />}
      </Link>
    </li>
  );
}

const appNavMenuSection = tv({
  base: "px-3",
  variants: {
    separator: {
      "3": "mt-3 border-t border-border pt-3",
      "4": "mt-4 border-t border-border pt-4",
    },
  },
});

export type AppNavMenuSectionProps = {
  /** A line above the section, with this spacing step on both of its sides; none for a section that follows on. */
  separator?: "3" | "4";
  children: ReactNode;
};

/** A block of the menu sheet under its items (the theme switch, the server's name and documents), indented as they are. */
export function AppNavMenuSection({ separator, children }: AppNavMenuSectionProps) {
  return <div className={appNavMenuSection({ separator })}>{children}</div>;
}

const appNavAction = tv({
  // Icon only in the compact rail (md), icon and label from lg; the width of the rail's items.
  slots: { base: "w-full min-h-12 lg:gap-3 [&>svg]:size-6", text: "hidden lg:inline" },
});

export type AppNavActionProps = {
  /** The action's words ("Write a post"): the button's accessible name at every width, shown from lg. */
  label: string;
  /** The button's icon (`PlusIcon`). */
  icon: ReactNode;
  onPress: () => void;
};

/** The rail's one main action, a `primary` button after the items: icon alone in the compact rail, with its label from lg. */
export function AppNavAction({ label, icon, onPress }: AppNavActionProps) {
  const slots = appNavAction();
  return (
    <Button variant="primary" aria-label={label} onPress={onPress} className={slots.base()}>
      {icon}
      <span aria-hidden="true" className={slots.text()}>
        {label}
      </span>
    </Button>
  );
}

const appNavAccount = tv({
  base: "mx-2 mt-auto mb-2 hidden min-h-12 min-w-0 gap-3 rounded-lg px-3 py-2 font-normal text-fg data-hovered:bg-surface-sunken lg:flex",
});

export type AppNavAccountProps = {
  /** The account's profile. */
  href: string;
  /** The account's `Avatar`, `size="sm"` and `decorative`: the name next to it says whose it is. */
  avatar: ReactNode;
  /** The display name (the user's text, `dir="auto"`). */
  name: ReactNode;
  /** The handle (`@user`). */
  handle: string;
  /** Describes the link ("Your profile"). */
  description: string;
};

/**
 * The sidebar's last item (lg only): the signed-in account, a link to its profile named by its display name and
 * handle, a line each and cut to one line (web-client/16-look-states-and-wording.md G7). The list's own profile item is the place, so this second
 * link to it has no `aria-current`.
 */
export function AppNavAccount({ href, avatar, name, handle, description }: AppNavAccountProps) {
  const descriptionId = useId();
  return (
    <>
      <Link href={href} aria-describedby={descriptionId} variant="navigation" className={appNavAccount()}>
        {avatar}
        <PostCardAuthor name={name} handle={handle} />
      </Link>
      {/* A description of a hidden element is still read (accname), and the element stays out of the tree. */}
      <span id={descriptionId} hidden>
        {description}
      </span>
    </>
  );
}
