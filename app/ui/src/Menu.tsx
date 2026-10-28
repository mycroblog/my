// SPDX-License-Identifier: AGPL-3.0-only
// An action menu: React Aria's MenuTrigger + Popover + Menu (WAI-ARIA APG, menu button pattern).
// Arrow keys move through the items (Up / Down; Home / End; type-ahead), Esc closes, and focus returns
// to the trigger. Horizontal keys follow the reading direction, so submenus open towards the inline end.
import type { ReactNode, Ref } from "react";
import {
  Menu as AriaMenu,
  MenuItem as AriaMenuItem,
  MenuTrigger,
  Popover,
  SubmenuTrigger,
  Separator,
  type MenuItemProps as AriaMenuItemProps,
  type MenuProps as AriaMenuProps,
  type PopoverProps,
} from "react-aria-components";
import { tv } from "tailwind-variants";

/** MenuTrigger wraps the trigger button and its Menu; SubmenuTrigger wraps a MenuItem and the Menu it opens. */
export { MenuTrigger, SubmenuTrigger };
export type { MenuTriggerProps, SubmenuTriggerProps } from "react-aria-components";

const menu = tv({
  slots: {
    popover: [
      "min-w-(--trigger-width) rounded-lg border border-border bg-surface-raised text-fg shadow-lg outline-none",
      "motion-safe:transition motion-safe:duration-fast data-entering:opacity-0 data-exiting:opacity-0",
    ],
    // overscroll-contain keeps the scroll from chaining to the page on iOS WebKit (web-client/09-app-shell.md §9).
    list: "max-h-[inherit] min-w-48 overflow-y-auto overscroll-contain p-1.5 outline-none",
    separator: "mx-2 my-1 border-t border-border",
  },
});

const menuItem = tv({
  base: [
    "flex cursor-default items-center gap-2 rounded-md px-3 py-2 text-sm outline-none select-none pointer-coarse:min-h-11",
    "data-focused:bg-surface-sunken data-focus-visible:focus-ring data-disabled:text-fg-muted",
  ],
  variants: {
    intent: { neutral: "text-fg", danger: "text-danger" },
  },
  defaultVariants: { intent: "neutral" },
});

export type MenuProps<T extends object> = Omit<AriaMenuProps<T>, "className" | "style"> &
  Pick<PopoverProps, "placement"> & {
    /** The menu's accessible name; usually the trigger's label. */
    "aria-label": string;
    /** Classes merged onto the popover with tailwind-merge. */
    className?: string;
    ref?: Ref<HTMLDivElement>;
  };

/**
 * The menu of a `MenuTrigger` or `SubmenuTrigger`, shown in a popover. Without `placement` it opens below
 * the trigger at the inline start, or beside its item at the inline end for a submenu.
 */
export function Menu<T extends object>({ placement, className, ...props }: MenuProps<T>) {
  const styles = menu();
  return (
    <Popover {...(placement === undefined ? {} : { placement })} className={styles.popover({ class: className })}>
      <AriaMenu {...props} className={styles.list()} />
    </Popover>
  );
}

export type MenuItemProps<T extends object = object> = Omit<
  AriaMenuItemProps<T>,
  "className" | "style" | "children"
> & {
  children: ReactNode;
  /** `danger` for destructive actions (delete, block). The label must still say what happens. */
  intent?: "neutral" | "danger";
  /** Classes merged with tailwind-merge. */
  className?: string;
  ref?: Ref<HTMLDivElement>;
};

/** One action of a Menu. Use `onAction` for actions and `href` for links. */
export function MenuItem<T extends object = object>({ intent, className, children, ...props }: MenuItemProps<T>) {
  const textValue = props.textValue ?? (typeof children === "string" ? children : undefined);
  return (
    <AriaMenuItem
      {...props}
      {...(textValue === undefined ? {} : { textValue })}
      className={menuItem({ intent, class: className })}
    >
      {children}
    </AriaMenuItem>
  );
}

/** A divider between groups of menu items. */
export function MenuSeparator() {
  return <Separator className={menu().separator()} />;
}
