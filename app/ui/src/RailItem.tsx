// SPDX-License-Identifier: AGPL-3.0-only
// One item of the app's navigation (brand kit `RailItem`): an icon, a label and an optional count. It is a link in the
// bottom tab bar (`form="tab"`: icon over a small label), in the side rail (`row`: icon, label and the count at the
// end), in the grouped menus of the settings and the admin mode (`section`) and in the signed-out top bar (`top`:
// text only). The current item is `aria-current="page"` and differs by shape, not by color alone (WCAG 1.4.1): in the tab
// bar a filled pill stands behind its icon, and in the rows the soft fill covers the whole row. It knows no
// viewport: AppNavigation chooses between the forms by width (web-client/README.md D2), through `className`.
import type { ReactNode } from "react";
import { tv } from "tailwind-variants";
import { Badge } from "./Badge";
import { Link } from "./Link";

/** The look of each form, shared with the tab bar's preview (`TabBarPreview`). */
export const railItem = tv({
  slots: {
    // The label is never cut: where the labels together are wider than the bar (a long translation), they wrap,
    // inside a word if they must, so that every label is read whole (ADR-0039). The icon is size-6 (G11).
    item: "group",
    // The pill around the icon, which carries the count on its corner.
    pill: "relative",
    badge: "",
    // The label under the icon (beside it from lg).
    label: "max-w-full text-center leading-tight text-balance wrap-anywhere",
  },
  variants: {
    form: {
      // Icon over a small label, in the bottom tab bar.
      tab: {
        item: [
          "flex min-h-16 w-full flex-col items-center justify-center gap-1 rounded-none bg-transparent px-0.5 text-xs font-bold text-fg-muted",
          "data-current:rounded-none data-current:bg-transparent data-current:text-accent data-hovered:bg-surface-sunken pointer-coarse:min-h-16",
        ],
        // The pill is drawn behind the icon of the current item only; its room is kept on every item, so that the labels
        // stay level.
        pill: "rounded-full px-4 py-0.5 motion-safe:transition-colors",
        badge: "absolute -top-1.5 -inset-e-1 ring-2 ring-surface-raised",
      },
      // A row of the rail: the icon, the label and the count at the end.
      row: {
        item: [
          "min-h-12 gap-4 rounded-lg bg-transparent px-4 text-base font-bold text-fg data-hovered:bg-surface-sunken",
          "data-current:rounded-lg data-current:bg-soft data-current:text-accent data-current:data-hovered:bg-soft",
        ],
        pill: "contents",
        badge: "order-last ms-auto",
        label: "text-start",
      },
      // A row of a grouped menu (NavMenu): the full width of the menu, the icon, the label and the count at the end.
      section: {
        item: [
          "min-h-11 w-full gap-3 rounded-lg bg-transparent px-3 text-sm font-bold text-fg data-hovered:bg-surface-sunken",
          "data-current:rounded-lg data-current:bg-soft data-current:text-accent data-current:data-hovered:bg-soft",
        ],
        pill: "contents",
        badge: "order-last ms-auto",
        label: "text-start",
      },
      // A text link of the signed-out top bar.
      top: {
        item: [
          "min-h-11 gap-2 rounded-lg bg-transparent px-3 text-sm font-bold text-fg data-hovered:bg-surface-sunken",
          "data-current:rounded-lg data-current:bg-soft data-current:text-accent data-current:data-hovered:bg-soft",
        ],
        pill: "hidden",
        label: "text-start",
      },
    },
    // The call to action of the top bar (create an account): the brand's coral pill.
    prominent: { true: {} },
    current: { true: {} },
  },
  compoundVariants: [
    // The current tab's shape: the filled pill behind its icon.
    { form: "tab", current: true, class: { pill: "bg-soft" } },
    {
      form: "top",
      prominent: true,
      class: {
        item: "rounded-full bg-accent-fill px-4 text-accent-fill-fg data-current:rounded-full data-current:bg-accent-fill data-current:text-accent-fill-fg data-hovered:bg-accent-fill-hover data-current:data-hovered:bg-accent-fill-hover",
      },
    },
  ],
  defaultVariants: { form: "tab" },
});

/** A count on an item: the number, what it means, and the name of the item while it shows it. */
export type RailItemBadge = {
  count: number;
  /** What the number means ("3 unread notifications"); `name` says it, and the drawn digits are not read. */
  label: string;
  /** The item's accessible name while it shows the count, starting with the visible label ("Notifications, 3 unread"). */
  name: string;
};

export type RailItemProps = {
  href: string;
  /** The item's icon (one of the kit's icons); the item sizes it. */
  icon: ReactNode;
  /** The visible label, the link's name without a count. */
  label: string;
  /**
   * Whether the link is the place the user is in: `aria-current="page"`, the accent label, and a shape the other items
   * lack: the filled pill behind the icon in the tab bar, the soft fill of the whole row elsewhere.
   */
  current?: boolean;
  /** The count on the icon's corner (at the end of the row from lg), or nothing for an item without a count now. */
  badge?: RailItemBadge;
  /** `tab` (default): icon over a small label; `row`: a row of the rail; `top`: a text link of the signed-out top bar; `section`: a row of a grouped menu (`NavMenu`). */
  form?: "tab" | "row" | "top" | "section";
  /** Adjusts the parts to the item's place (the navigation's responsive forms); overrides the form's classes. */
  classNames?: { item?: string; pill?: string; badge?: string; label?: string };
  /** In the top bar, the one call to action (a coral pill). */
  prominent?: boolean;
};

/** A navigation link: icon, label and an optional count; the current one is filled (its icon's pill or its row) with an accent label. */
export function RailItem({ href, icon, label, current = false, badge, form, prominent, classNames }: RailItemProps) {
  const slots = railItem({ form, prominent, current });
  return (
    <Link
      href={href}
      {...(badge !== undefined && { "aria-label": badge.name })}
      {...(current && { "aria-current": "page" as const })}
      variant="navigation"
      className={slots.item({ className: classNames?.item })}
    >
      <span className={slots.pill({ className: classNames?.pill })}>
        {icon}
        {badge !== undefined && (
          <Badge count={badge.count} decorative className={slots.badge({ className: classNames?.badge })} />
        )}
      </span>
      <span className={slots.label({ className: classNames?.label })}>{label}</span>
    </Link>
  );
}
