// SPDX-License-Identifier: AGPL-3.0-only

// The menu, for a post's more and boost actions, on React Aria Components' Menu: an icon button opens it in a popover
// under the button, at its end or its start, on every screen; the pointer and the keyboard move through its items (arrows, Home and End,
// typing a label's start), Enter or a press runs one and closes it, Escape or a press outside closes it and focus
// returns to the button. It stands off the page by a shadow on the page's surface; each item is an 18 px glyph and
// its label, in rows of a form control's height. Its groups stand apart by space; a group of actions on another
// account goes under a caption naming it; destructive items sit last and apart, with no color of their own.

import type { ReactNode } from "react";
import { Header, Menu, MenuItem as AriaMenuItem, MenuSection, MenuTrigger, Popover } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { IconButton } from "./icon-button";
import { Icon, type IconName } from "./icons";
import { usePreviewItemState } from "./preview";

const menu = tv({
  slots: {
    panel: [
      "box-border min-w-[var(--menu-min-width)] rounded-overlay bg-surface p-menu-padding shadow-overlay",
      "font-sans text-text outline-hidden",
    ],
    list: "flex flex-col gap-menu-group-gap outline-hidden",
    group: "flex flex-col",
    header: "px-control-timeline-inline py-menu-padding text-caption text-muted-text",
    item: [
      "box-border flex h-control-form cursor-default items-center gap-menu-icon-gap rounded-control",
      "px-control-timeline-inline text-body whitespace-nowrap outline-hidden",
    ],
    glyph: "inline-flex text-muted-text",
  },
  variants: {
    // Hovered or focused, the item steps a surface up from the menu's; pressed, one more.
    state: {
      rest: {},
      hover: { item: "bg-surface-raised" },
      pressed: { item: "bg-surface-raised-hover" },
    },
    // Inside a menu, whose rows touch, the focus ring is drawn within the item.
    ring: { true: { item: "shadow-focus-inset" } },
  },
});

/** The menu's surface and its items, open: inside the popover of a `MenuButton`, or on its own, open at rest. */
export function MenuPanel({
  label,
  children,
}: {
  /** The menu's accessible name. */
  label: string;
  children: ReactNode;
}) {
  const s = menu();
  return (
    <div className={s.panel()}>
      <Menu aria-label={label} className={s.list()}>
        {children}
      </Menu>
    </div>
  );
}

/** A group of items, apart from the others by space; `title` names it above its items (the account they act on). */
export function MenuGroup({ title, children }: { title?: string | undefined; children: ReactNode }) {
  const s = menu();
  return (
    <MenuSection className={s.group()}>
      {title !== undefined && <Header className={s.header()}>{title}</Header>}
      {children}
    </MenuSection>
  );
}

/**
 * An item: a glyph and its label; `on` draws a toggle's glyph on. A label whose action asks for a confirmation ends
 * in "…".
 */
export function MenuItem({
  id,
  icon,
  on = false,
  onAction,
  children,
}: {
  id: string;
  icon: IconName;
  on?: boolean;
  onAction?: () => void;
  children: string;
}) {
  const preview = usePreviewItemState(id);
  return (
    <AriaMenuItem
      id={id}
      textValue={children}
      {...(onAction ? { onAction } : {})}
      className={({ isHovered, isFocused, isFocusVisible, isPressed }) =>
        menu({
          state:
            isPressed || preview === "pressed"
              ? "pressed"
              : isHovered || isFocused || preview !== undefined
                ? "hover"
                : "rest",
          ring: isFocusVisible || preview === "focus-visible",
        }).item()
      }
    >
      <span className={menu().glyph()}>
        <Icon name={icon} place="action" on={on} />
      </span>
      <span>{children}</span>
    </AriaMenuItem>
  );
}

/**
 * An icon button that opens a menu of `children` in a popover under it, at its end or its start. Like the icon
 * button, it may show a count and an on state (boost, whose menu holds boost and quote).
 */
export function MenuButton({
  icon,
  label,
  count,
  pressed,
  align = "end",
  open,
  onOpenChange,
  "aria-keyshortcuts": keyshortcuts,
  children,
}: {
  icon: IconName;
  /** The button's name, which also names the menu. */
  label: string;
  /** How many have done it, as the icon button shows it (boost); zero or none shows no count. */
  count?: number | undefined;
  /** The on state of what the menu toggles, as the icon button shows it (boost). */
  pressed?: boolean | undefined;
  /** Which edge of the button the menu lines up with: its end (more, at the row's end) or its start. */
  align?: "start" | "end";
  /** The menu is open, when its owner opens it too (a shortcut), with `onOpenChange`; else the button alone does. */
  open?: boolean | undefined;
  onOpenChange?: ((open: boolean) => void) | undefined;
  /** The keys that open the menu (what `useAction` returns). */
  "aria-keyshortcuts"?: string | undefined;
  children: ReactNode;
}) {
  return (
    <MenuTrigger {...(open === undefined ? {} : { isOpen: open })} {...(onOpenChange ? { onOpenChange } : {})}>
      <IconButton icon={icon} label={label} count={count} pressed={pressed} aria-keyshortcuts={keyshortcuts} />
      <Popover placement={`bottom ${align}`} offset={4} className="outline-hidden">
        <MenuPanel label={label}>{children}</MenuPanel>
      </Popover>
    </MenuTrigger>
  );
}
