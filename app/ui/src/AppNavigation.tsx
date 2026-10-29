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
// Signed out, the navigation is a bar along the top (`placement="top"`): the brand, then the items in a row from lg;
// below lg the items are behind a menu button (a disclosure), and only the call to action stays in the bar.
import {
  createContext,
  type FocusEvent,
  type KeyboardEvent,
  type MouseEvent,
  type ReactNode,
  use,
  useEffect,
  useId,
  useRef,
} from "react";
import { tv } from "tailwind-variants";
import { Button } from "./Button";
import { Badge } from "./Badge";
import { useBlockSizeProperty } from "./blockSizeProperty";
import { Sheet, DialogTrigger } from "./Dialog";
import { CloseIcon, MenuIcon } from "./Icons";
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
      // One row at every width: the brand takes the room left and its name wraps (never the items). Below lg the
      // items are in the menu (`topMenu`), from lg they follow the brand in a row.
      top: {
        base: "sticky top-under-bar flex items-center gap-2 border-b border-border px-4 py-2 lg:gap-4",
        // The server's name is cut to one line (G7) rather than broken inside a word: its link keeps the whole name.
        header:
          "mx-0 block min-w-0 flex-1 p-0 [&_a]:block [&_a]:truncate [&_a]:leading-6 pointer-coarse:[&_a]:leading-11 [&_p]:min-w-0",
        list: "items-center gap-1",
      },
    },
    // The top bar's items behind a menu button below lg (a disclosure, WAI-ARIA APG): a panel under the bar while
    // it is open, one item per row across the screen; from lg the row of the bar, the button gone.
    topMenu: {
      true: {
        list: [
          "absolute inset-x-0 top-full hidden flex-col items-stretch border-b border-border bg-surface-raised p-2 shadow-md data-open:flex",
          "px-[max(0.5rem,env(safe-area-inset-left),env(safe-area-inset-right))]",
          "lg:static lg:flex lg:flex-row lg:items-center lg:border-0 lg:bg-transparent lg:p-0 lg:shadow-none",
        ],
      },
    },
  },
  defaultVariants: { placement: "side" },
});

const appNavTop = tv({
  slots: {
    // The bar's call to action and its menu button: phones only.
    action: "flex-none lg:hidden",
    menuButton: "min-h-11 min-w-11 flex-none lg:hidden [&_svg]:size-6",
  },
});

/** The menu button of the top bar below lg, which shows and hides its items (`AppNav` `topMenu`). */
export type AppNavTopMenu = {
  /** The button's name ("Menu"). */
  label: string;
  isOpen: boolean;
  /** Called to open or close the items: the button, Escape, a link of the items, and focus or a press leaving the bar close them. */
  onOpenChange: (isOpen: boolean) => void;
};

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
  /**
   * The top bar only: below lg, the items go behind this menu button (a disclosure, WAI-ARIA APG) and the bar keeps
   * the header, `topAction` and the button. Without it every item stays in the bar's row.
   */
  topMenu?: AppNavTopMenu;
  /**
   * The top bar only, below lg: the call to action that stays in the bar beside the menu button (`AppNavTopAction`).
   * Its item in the list shows from lg only (`shows="rail"`), so that it is not in the menu twice.
   */
  topAction?: ReactNode;
};

const PlacementContext = createContext<"side" | "top">("side");

/**
 * The `nav` landmark and its list. The header and the footer are outside the list, so the list's links stay the same
 * and in the same order at every width.
 *
 * With `topMenu`, the top bar's list is the content of a disclosure below lg: the button (`aria-expanded`,
 * `aria-controls`) comes right before the list, so that Tab goes from the button into the items. Nothing is trapped:
 * the items close on Escape (focus back to the button), on following one of their links, and when focus or a press
 * leaves the bar.
 */
export function AppNav({
  label,
  header,
  children,
  action,
  footer,
  placement = "side",
  topMenu,
  topAction,
}: AppNavProps) {
  const top = placement === "top";
  const menu = top ? topMenu : undefined;
  const slots = appNav({ placement, topMenu: menu !== undefined });
  const topSlots = appNavTop();
  const listId = useId();
  const navRef = useRef<HTMLElement>(null);
  // The top bar's height, under which what sticks to the top of the page starts (`top-under-top-bar`).
  useBlockSizeProperty(navRef, top ? "--top-bar-block-size" : undefined);
  const isOpen = menu?.isOpen ?? false;
  const close = menu?.onOpenChange;
  // A press outside the bar closes the items, as focus leaving it does; the page under the panel stays usable.
  useEffect(() => {
    if (!isOpen || close === undefined) return;
    const onPointerDown = (event: PointerEvent) => {
      if (!(event.target instanceof Node) || navRef.current?.contains(event.target) !== true) close(false);
    };
    document.addEventListener("pointerdown", onPointerDown);
    return () => {
      document.removeEventListener("pointerdown", onPointerDown);
    };
  }, [isOpen, close]);
  return (
    <PlacementContext value={placement}>
      <nav
        ref={navRef}
        aria-label={label}
        className={slots.base()}
        {...(menu !== undefined && {
          onKeyDown: (event: KeyboardEvent<HTMLElement>) => {
            if (event.key !== "Escape" || !menu.isOpen) return;
            event.stopPropagation();
            menu.onOpenChange(false);
            navRef.current?.querySelector<HTMLElement>(`[aria-controls="${listId}"]`)?.focus();
          },
          onBlur: (event: FocusEvent<HTMLElement>) => {
            if (
              menu.isOpen &&
              !(event.relatedTarget instanceof Node && event.currentTarget.contains(event.relatedTarget))
            )
              menu.onOpenChange(false);
          },
        })}
      >
        {header !== undefined && <div className={slots.header()}>{header}</div>}
        {top && topAction !== undefined && <div className={topSlots.action()}>{topAction}</div>}
        {menu !== undefined && (
          <Button
            variant="ghost"
            size="icon"
            aria-label={menu.label}
            aria-expanded={menu.isOpen}
            aria-controls={listId}
            onPress={() => {
              menu.onOpenChange(!menu.isOpen);
            }}
            className={topSlots.menuButton()}
          >
            {menu.isOpen ? <CloseIcon /> : <MenuIcon />}
          </Button>
        )}
        <ul
          id={listId}
          data-open={isOpen ? "" : undefined}
          className={slots.list()}
          {...(menu !== undefined && {
            // Following a link of the items closes them (the page it opens takes focus).
            onClick: (event: MouseEvent<HTMLElement>) => {
              if (event.target instanceof Element && event.target.closest("a") !== null) menu.onOpenChange(false);
            },
          })}
        >
          {children}
        </ul>
        {action !== undefined && <div className={slots.action()}>{action}</div>}
        {footer}
      </nav>
    </PlacementContext>
  );
}

export type AppNavTopActionProps = {
  href: string;
  /** The visible label, the link's name ("Sign up"). */
  label: string;
};

/** The top bar's call to action below lg (`AppNav` `topAction`): the coral pill of the list's prominent item. */
export function AppNavTopAction({ href, label }: AppNavTopActionProps) {
  return <RailItem href={href} icon={null} label={label} form="top" prominent />;
}

export type AppNavBrandProps = {
  /** The server's name (`ServerNameLink`), which names the brand: the wordmark beside it is decorative. */
  children: ReactNode;
  /**
   * Whether `children` show the server's own logo (`ServerNameLink`'s `logo`), which includes its name and takes the
   * place of the built-in wordmark.
   */
  ownLogo?: boolean;
};

/**
 * The wordmark with the server's name beside it, or the server's own logo alone: the start of the sidebar and of the
 * signed-out top bar.
 */
export function AppNavBrand({ children, ownLogo = false }: AppNavBrandProps) {
  return (
    <div className="flex min-w-0 items-center gap-2">
      {!ownLogo && <Wordmark name="my" decorative size="sm" />}
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
    // The top bar's item: a full row with its icon in the menu below lg, a text link in the bar's row from lg.
    topItem: "w-full lg:w-auto",
    topPill: "contents lg:hidden",
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
    // In the top bar the items keep their width in the row (from lg) and fill the menu's rows (below lg).
    placement: { side: {}, top: { base: "flex-none" } },
    // The first item of the staff's areas, set apart from the user's items by a line in the rail and the sidebar.
    separated: { true: { base: "md:mt-2 md:border-t md:border-border md:pt-3" } },
  },
  // In the top bar, an item for the rail and the sidebar (`shows="rail"`) is the call to action, which the bar shows
  // outside the list below lg (`AppNav` `topAction`): it joins the row from lg. `md:hidden` overrides the rail's
  // `md:block`, so that the call to action is not in the menu between md and lg as well.
  compoundVariants: [{ placement: "top", shows: "rail", class: { base: "hidden md:hidden lg:block" } }],
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
  /** Where the item shows: everywhere, only in the tab bar (below md), or only in the rail and the sidebar (from md). In the top bar, `rail` is the call to action, outside the list below lg. */
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
        {...(side
          ? {
              classNames: {
                item: slots.railItem(),
                pill: slots.railPill(),
                badge: slots.railBadge(),
                label: slots.railLabel(),
              },
            }
          : { classNames: { item: slots.topItem(), pill: slots.topPill() } })}
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
            {badge !== undefined && <Badge count={badge.count} decorative className={slots.badge()} />}
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
        {badge !== undefined && <Badge count={badge.count} decorative />}
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
