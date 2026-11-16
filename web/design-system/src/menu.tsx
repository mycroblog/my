// SPDX-License-Identifier: AGPL-3.0-only

// The menu, for a post's more and boost actions, on React Aria Components' Menu: an icon button opens it in a popover
// under the button, at its end or its start, on every screen; the pointer and the keyboard move through its items (arrows, Home and End,
// typing a label's start), Enter or a press runs one and closes it, Escape or a press outside closes it and focus
// returns to the button. It stands off the page by a shadow on the page's surface; each item is an 18 px glyph and
// its label, in rows of a form control's height. Its groups stand apart by space; a group of actions on another
// account goes under a caption naming it; destructive items sit last and apart, with no color of their own.

import { useId, type ReactNode } from "react";
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
      "box-border flex cursor-default gap-menu-icon-gap rounded-control",
      "px-control-timeline-inline text-body whitespace-nowrap outline-hidden",
    ],
    glyph: "inline-flex text-muted-text",
    // An item's label over its quiet line, which wraps between phrases within a menu's width.
    text: "grid gap-[0.125rem]",
    about: "max-w-[16rem] text-caption whitespace-normal text-muted-text [word-break:auto-phrase]",
  },
  variants: {
    // An item is a form control's height; one with a line under its label grows with it (never clipped), padded so
    // that the line stands apart from the next item's label, its glyph on the label's line. The height is set here
    // alone: the variants' classes are not merged, so a fixed height beside a grown one would leave either.
    described: {
      false: { item: "h-control-form items-center" },
      true: { item: "min-h-control-form items-start py-[0.5rem]", glyph: "h-[1lh] items-center" },
    },
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
  own = false,
  children,
}: {
  /** The menu's accessible name. */
  label: string;
  /** `label` names the menu even inside a trigger whose button has another name (New post's menu, of New article). */
  own?: boolean | undefined;
  children: ReactNode;
}) {
  const s = menu();
  const id = useId();
  return (
    <div className={s.panel()}>
      {/* A trigger that names the menu otherwise (React Aria names it by its button) gives way to `label`. */}
      {own && (
        <span id={id} hidden>
          {label}
        </span>
      )}
      <Menu aria-label={label} {...(own ? { "aria-labelledby": id } : {})} className={s.list()}>
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
 * in "…". `about` is a quiet line under the label, read with it as the item's name (what the action will do: wait
 * for an approval).
 */
export function MenuItem({
  id,
  icon,
  on = false,
  onAction,
  about,
  children,
}: {
  id: string;
  icon: IconName;
  on?: boolean;
  onAction?: (() => void) | undefined;
  about?: string | undefined;
  children: string;
}) {
  const preview = usePreviewItemState(id);
  const described = about !== undefined;
  const s = menu({ described });
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
          described,
        }).item()
      }
    >
      <span className={s.glyph()}>
        <Icon name={icon} place="action" on={on} />
      </span>
      {described ? (
        <span className={s.text()}>
          {/* Both read as the item's name: a name without the line it shows is a mismatch for voice control. */}
          <span>{children}</span>
          <span className={s.about()}>{about}</span>
        </span>
      ) : (
        <span>{children}</span>
      )}
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
