// SPDX-License-Identifier: AGPL-3.0-only

// The one menu of the app (#3129), for every menu-like surface (a post's more and boost, a profile's, the composer's
// audience, language and drafts, a poll's duration, New post's, the Menu of the navigation), on React Aria Components'
// Menu: its trigger opens it, on a phone as a sheet from the bottom edge over the page dimmed and blurred as under every
// modal (the design system's modal, which traps the focus), headed by its name and close, above the software keyboard
// where one is open; from `tablet` in a popover by its trigger, standing off the page by a shadow, with no head. The app
// says which (`MenuSheet`), as it alone reads the viewport. The pointer and the keyboard move through its items (arrows,
// Home and End, typing a label's start), Enter or a press runs one and closes it, Escape, a press outside or close
// closes it, and focus returns to the trigger. A submenu takes its menu's place, headed by back and its name. A small
// form that is filled in and sent (the schedule's, `FormPopup`) opens on the same surface. Each item is an 18 px glyph
// and its label, in rows of a form control's height; the value chosen among a group's has a check at its end, a toggle
// its glyph on. Its groups stand apart by space; a group of actions on another account goes under a caption naming it;
// destructive items sit last and apart, with no color of their own.

import { createContext, use, useEffect, useId, useRef, useState, type ReactElement, type ReactNode } from "react";
import {
  Dialog,
  DialogTrigger,
  Header,
  Menu,
  MenuItem as AriaMenuItem,
  MenuSection,
  MenuTrigger,
  Popover,
  type Selection,
} from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { IconButton } from "./icon-button";
import { Icon, type IconName } from "./icons";
import { stringsIn, useReader } from "./locale";
import { ModalLayer, ModalPresence } from "./modal";
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
      "px-control-timeline-inline text-body whitespace-nowrap no-underline outline-hidden [&[href]]:text-inherit",
    ],
    glyph: "inline-flex text-muted-text",
    // An item's label over its quiet line, which wraps between phrases within a menu's width.
    text: "grid gap-[0.125rem]",
    about: "max-w-[16rem] text-caption whitespace-normal text-muted-text [word-break:auto-phrase]",
    // What sits at the item's end: the chosen value's check, or what a submenu holds now and its ›.
    end: "ms-auto inline-flex items-center gap-menu-icon-gap ps-menu-icon-gap text-caption text-muted-text",
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
    // What cannot be done now (add to a list an account the reader does not follow): muted, its line saying why.
    disabled: { true: { item: "text-muted-text" } },
    // On a phone, a sheet from the bottom edge: its head stays while its items scroll under it, and its bottom keeps
    // the room of its top and the device's safe area, so that no row sits at the edge.
    sheet: {
      true: {
        panel: [
          "flex max-h-[85dvh] min-h-0 w-full flex-col rounded-b-none pt-[var(--dialog-padding)]",
          "pb-[calc(var(--dialog-padding)+env(safe-area-inset-bottom))]",
        ],
        // As wide as the viewport and no wider: an item's label and what it holds wrap, never scrolling sideways.
        // A row grows with a label that wraps, never running over the next row or into the gap before a group.
        list: [
          "min-h-0 overflow-y-auto [&_[role^=menuitem]]:whitespace-normal",
          "[&_.h-control-form]:h-auto [&_.h-control-form]:min-h-control-form [&_.h-control-form]:py-[0.25rem]",
        ],
      },
    },
  },
});

const sheet = tv({
  slots: {
    // Over the viewport less the software keyboard where one is open, so that the sheet's bottom stays above it; room
    // above it shows the page under the backdrop.
    overlay: [
      "fixed inset-0 z-50 box-border flex flex-col justify-end pt-shell-sheet-room",
      "pb-[var(--keyboard-inset,env(keyboard-inset-height,0px))]",
    ],
    // Positioned, so that it paints over the fixed backdrop before it in the overlay.
    panel: "relative flex min-h-0 w-full flex-col",
  },
});

const head = tv({
  slots: {
    // A group's space under it, as between groups; it does not scroll with the items.
    root: "mb-menu-group-gap flex shrink-0 items-center gap-menu-icon-gap",
    title: "m-0 min-w-0 flex-1 text-title",
    start: "ps-control-timeline-inline",
  },
});

/**
 * The head of a menu: on a phone, its name with close at its end; in a submenu, on a phone and in a popover alike, a
 * back control before the submenu's name, which no item repeats. A popover's own menu has none.
 */
export function MenuHead({
  title,
  back,
  close,
}: {
  title: string;
  /** In a submenu: back to the menu it opened from, named by `back.label`. */
  back?: { label: string; onPress?: (() => void) | undefined } | undefined;
  /** On a phone: close's name. */
  close?: { label: string; onPress?: (() => void) | undefined } | undefined;
}) {
  const s = head();
  return (
    <div className={s.root()}>
      {back !== undefined && (
        <IconButton icon="back" label={back.label} {...(back.onPress ? { onPress: back.onPress } : {})} />
      )}
      <h2 className={back === undefined ? `${s.title()} ${s.start()}` : s.title()}>{title}</h2>
      {close !== undefined && (
        <IconButton icon="close" label={close.label} {...(close.onPress ? { onPress: close.onPress } : {})} />
      )}
    </div>
  );
}

/**
 * The menu's surface and its items, open: in the sheet or the popover of a `MenuSurface`, or on its own, open at rest.
 */
export function MenuPanel({
  label,
  own = false,
  sheet = false,
  head,
  children,
}: {
  /** The menu's accessible name. */
  label: string;
  /** `label` names the menu even inside a trigger whose button has another name (New post's menu, of New article). */
  own?: boolean | undefined;
  /** On a phone: a sheet from the bottom edge, as wide as the viewport, its rows reaching across. */
  sheet?: boolean | undefined;
  /** Its head (`MenuHead`): a sheet's name and close, or a submenu's back and name. */
  head?: ReactNode;
  children: ReactNode;
}) {
  const s = menu({ sheet });
  const id = useId();
  return (
    <div className={s.panel()}>
      {head}
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

/** The first key of a selection: what a group of one choice holds now. */
const firstKey = (keys: Selection): string | undefined => (keys === "all" ? undefined : [...keys].map(String).at(0));

/**
 * A group of items, apart from the others by space; `title` names it above its items (the account they act on, or what
 * its values set). With `chosen`, its items are the values of one setting (a visibility, a language): the one chosen
 * has a check at its end and is read as checked, and choosing another gives `onChoose` its id. With `checked`, they are
 * toggles (whether others may ask for approval): each one checked has its glyph on, and a press gives `onCheck` its id
 * and its new state. A choice closes the menu unless `stay` (a submenu whose settings go together).
 */
export function MenuGroup({
  title,
  chosen,
  onChoose,
  checked,
  onCheck,
  stay = false,
  children,
}: {
  title?: string | undefined;
  /** The id of the value chosen among its items. */
  chosen?: string | undefined;
  onChoose?: ((id: string) => void) | undefined;
  /** The ids of its toggles that are on. */
  checked?: readonly string[] | undefined;
  onCheck?: ((id: string, on: boolean) => void) | undefined;
  /** A choice keeps the menu open. */
  stay?: boolean | undefined;
  children: ReactNode;
}) {
  const s = menu();
  const selection =
    chosen !== undefined
      ? {
          selectionMode: "single" as const,
          selectedKeys: [chosen],
          disallowEmptySelection: true,
          onSelectionChange: (keys: Selection) => {
            const id = firstKey(keys);
            if (id !== undefined && id !== chosen) onChoose?.(id);
          },
        }
      : checked !== undefined
        ? {
            selectionMode: "multiple" as const,
            selectedKeys: checked,
            onSelectionChange: (keys: Selection) => {
              if (keys === "all") return;
              const now = new Set([...keys].map(String));
              for (const id of now) if (!checked.includes(id)) onCheck?.(id, true);
              for (const id of checked) if (!now.has(id)) onCheck?.(id, false);
            },
          }
        : {};
  return (
    <MenuSection className={s.group()} {...selection} {...(stay ? { shouldCloseOnSelect: false } : {})}>
      {title !== undefined && <Header className={s.header()}>{title}</Header>}
      {children}
    </MenuSection>
  );
}

/**
 * An item: a glyph and its label; `on` draws a toggle's glyph on. A label whose action asks for a confirmation ends
 * in "…". `about` is a quiet line under the label, read with it as the item's name (what the action will do: wait
 * for an approval). With `href` it is a link to a place (the Menu's), `current` where the reader is, its glyph on,
 * `badge` at its end (how many wait there). In a group of `chosen` values, the one chosen has a check at its end; in a
 * group of `checked` toggles, each one on has its glyph on.
 */
export function MenuItem({
  id,
  icon,
  on = false,
  onAction,
  about,
  disabled = false,
  chosen = false,
  opens: opensValue,
  href,
  current = false,
  badge,
  children,
}: {
  id: string;
  icon: IconName;
  on?: boolean;
  onAction?: (() => void) | undefined;
  about?: string | undefined;
  /** It cannot be done now; `about` says why. */
  disabled?: boolean | undefined;
  /** The value chosen, drawn at rest outside a group of `chosen` values (a story's): a check at the item's end. */
  chosen?: boolean | undefined;
  /**
   * It opens a submenu, which the menu's owner puts in its place (`MenuSurface`'s `sub`): what it holds now (muted, or
   * "" for nothing) and › at the item's end. It keeps the menu open.
   */
  opens?: string | undefined;
  /** A link: where it leads. */
  href?: string | undefined;
  /** The reader is at the place it leads to. */
  current?: boolean | undefined;
  /** What it counts at its end, with its words for assistive technology (a frame's badge). */
  badge?: ReactNode;
  children: string;
}) {
  const preview = usePreviewItemState(id);
  const described = about !== undefined;
  const s = menu({ described });
  return (
    <AriaMenuItem
      id={id}
      textValue={children}
      isDisabled={disabled}
      {...(onAction ? { onAction } : {})}
      {...(href === undefined ? {} : { href })}
      {...(current
        ? // React Aria passes no aria-current through: the place where the reader is says so on the element itself.
          {
            render: (props: object) => (href === undefined ? <div {...props} /> : <a {...props} aria-current="page" />),
          }
        : opensValue !== undefined && !disabled
          ? // An item given aria-haspopup is React Aria's trigger of a submenu of its own, which runs no action: the
            // item that opens a submenu says so on the element itself, where it shows ›.
            { render: (props: object) => <div {...props} aria-haspopup="menu" /> }
          : {})}
      {...(opensValue === undefined ? {} : { shouldCloseOnSelect: false })}
      className={({ isHovered, isFocused, isFocusVisible, isPressed }) =>
        menu({
          state: disabled
            ? "rest"
            : isPressed || preview === "pressed"
              ? "pressed"
              : isHovered || isFocused || preview !== undefined
                ? "hover"
                : "rest",
          ring: isFocusVisible || preview === "focus-visible",
          described,
          disabled,
        }).item()
      }
    >
      {({ isSelected, selectionMode }) => {
        const checked = (selectionMode === "single" && isSelected) || chosen;
        // A submenu that cannot be opened now shows neither what it holds nor ›: its line says why instead.
        const opens = disabled ? undefined : opensValue;
        return (
          <>
            <span className={s.glyph()}>
              <Icon name={icon} place="action" on={on || current || (selectionMode === "multiple" && isSelected)} />
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
            {(checked || opens !== undefined || badge !== undefined) && (
              <span className={s.end()}>
                {opens !== undefined && opens !== "" && <span>{opens}</span>}
                {badge}
                {(checked || opens !== undefined) && <Icon name={checked ? "chosen" : "onward"} place="action" />}
              </span>
            )}
          </>
        );
      }}
    </AriaMenuItem>
  );
}

const SheetContext = createContext(false);

// Whether a pointer is down anywhere on the page, followed from the first menu on.
let pressing = false;
let following = false;
function followPresses() {
  if (following) return;
  following = true;
  const up = () => {
    pressing = false;
  };
  document.addEventListener(
    "pointerdown",
    () => {
      pressing = true;
    },
    true,
  );
  document.addEventListener("pointerup", up, true);
  document.addEventListener("pointercancel", up, true);
}

/**
 * The sheet over the page. React Aria opens a menu as a mouse's press starts (and a long press while it lasts), and
 * runs the item a press ends on, to choose by dragging; a sheet rises over its trigger (the tab bar's Menu), so the
 * press that opened it would end on an item and run it. It takes no pointer until that press ends.
 */
function MenuSheetLayer({ onDismiss, children }: { onDismiss: () => void; children: ReactNode }) {
  const [armed, setArmed] = useState(() => !pressing);
  useEffect(() => {
    if (armed) return undefined;
    const arm = () => {
      setArmed(true);
    };
    document.addEventListener("pointerup", arm, true);
    document.addEventListener("pointercancel", arm, true);
    return () => {
      document.removeEventListener("pointerup", arm, true);
      document.removeEventListener("pointercancel", arm, true);
    };
  }, [armed]);
  const s = sheet();
  return (
    <ModalLayer
      onDismiss={onDismiss}
      overlay={armed ? s.overlay() : `${s.overlay()} pointer-events-none`}
      panel={s.panel()}
    >
      {children}
    </ModalLayer>
  );
}

/**
 * Whether the menus inside show as a sheet from the bottom edge (`sheet`, on a phone's viewport) or in a popover by
 * their trigger: the app's choice, as it alone reads the viewport.
 */
export function MenuSheet({ sheet, children }: { sheet: boolean; children: ReactNode }) {
  return <SheetContext value={sheet}>{children}</SheetContext>;
}

/** Where a popover sits against its trigger: under it at its start or end, over it, or beside it toward the content. */
export type MenuPlacement = "bottom start" | "bottom end" | "top start" | "top end" | "end bottom";

export type MenuSurfaceProps = {
  /** What opens it: a React Aria pressable (an icon button, a button, a frame's action). */
  trigger: ReactElement;
  /** The menu's name: its sheet's title, and what assistive technology reads it by, whatever its trigger is named. */
  label: string;
  /** Where its popover sits from `tablet`. */
  placement?: MenuPlacement | undefined;
  /** How far its popover stands from its trigger, in px. */
  offset?: number | undefined;
  /** The menu is open, when its owner opens it too (a shortcut), with `onOpenChange`; else its trigger alone does. */
  open?: boolean | undefined;
  onOpenChange?: ((open: boolean) => void) | undefined;
  /** A long press, not a press, opens it (the trigger's press runs its own action). */
  longPress?: boolean | undefined;
  /**
   * The submenu shown in its place, `children` its items: its head's name, and `onBack`, what puts its menu back. The
   * owner holds which one is shown, and opens one from an item with `opens`.
   */
  sub?: { title: string; onBack: () => void } | undefined;
  children: ReactNode;
};

/**
 * A menu of `children` opened by its trigger: on a phone a sheet from the bottom edge, from `tablet` a popover. The
 * focus goes to its first item (its chosen one) and back to the trigger when it closes.
 */
export function MenuSurface({
  trigger,
  label,
  placement = "bottom end",
  offset = 4,
  open,
  onOpenChange,
  longPress = false,
  sub,
  children,
}: MenuSurfaceProps) {
  const reader = useReader(undefined);
  const { t } = stringsIn(reader.lang);
  const [own, setOwn] = useState(false);
  const shown = open ?? own;
  // What held the focus as the menu opened (its trigger), which takes it back once a sheet has left.
  const opener = useRef<Element | null>(null);
  const change = (next: boolean) => {
    if (next) opener.current = document.activeElement;
    if (open === undefined) setOwn(next);
    onOpenChange?.(next);
  };
  const close = () => {
    change(false);
  };
  const title = sub?.title ?? label;
  const back = sub === undefined ? undefined : { label: t("menu.back"), onPress: sub.onBack };
  return (
    <MenuTrigger isOpen={shown} onOpenChange={change} {...(longPress ? { trigger: "longPress" as const } : {})}>
      {trigger}
      <SurfaceLayer
        shown={shown}
        onDismiss={close}
        onExited={() => {
          giveBack(opener.current);
        }}
        placement={placement}
        offset={offset}
      >
        {(inSheet) => (
          // A submenu is a menu of its own, which takes the focus to its first item as it replaces the one it came from.
          <MenuPanel
            key={title}
            label={title}
            own
            sheet={inSheet}
            head={
              inSheet ? (
                <MenuHead title={title} back={back} close={{ label: t("menu.close"), onPress: close }} />
              ) : back === undefined ? undefined : (
                <MenuHead title={title} back={back} />
              )
            }
          >
            {children}
          </MenuPanel>
        )}
      </SurfaceLayer>
    </MenuTrigger>
  );
}

/**
 * Gives the focus back to `opener` (a menu's trigger) once its sheet has left, where nothing else took it: the page,
 * or what holds the trigger (the composer's dialog, which takes the focus that the sheet's leaving dropped before the
 * sheet could give it back). A control or a dialog that took it on purpose (an item opened one) keeps it.
 */
function giveBack(opener: Element | null): void {
  if (!(opener instanceof HTMLElement) || !opener.isConnected) return;
  const active = document.activeElement;
  if (active === opener) return;
  if (active === null || active === document.body || active.contains(opener)) opener.focus({ preventScroll: true });
}

/**
 * What a menu or a small form shows on, open, by the trigger of the React Aria trigger around it: on a phone (as
 * `MenuSheet` says) a sheet from the bottom edge over the dimmed page, from `tablet` a popover by the trigger, which
 * flips to the other side of it where there is no room and stays inside the viewport. `children` draws what it holds,
 * told whether it is in the sheet.
 */
function SurfaceLayer({
  shown,
  onDismiss,
  onExited,
  placement,
  offset,
  children,
}: {
  shown: boolean;
  onDismiss: () => void;
  /** Runs once the sheet has left. */
  onExited?: (() => void) | undefined;
  placement: MenuPlacement;
  offset: number;
  children: (inSheet: boolean) => ReactNode;
}) {
  const asSheet = use(SheetContext);
  const reader = useReader(undefined);
  useEffect(followPresses, []);
  if (asSheet)
    return (
      <ModalPresence onExited={onExited}>
        {shown && (
          <MenuSheetLayer key="menu" onDismiss={onDismiss}>
            <div className={sheet().panel()} lang={reader.lang} dir={reader.direction}>
              {children(true)}
            </div>
          </MenuSheetLayer>
        )}
      </ModalPresence>
    );
  return (
    <Popover placement={placement} offset={offset} shouldFlip={placement !== "end bottom"} className="outline-hidden">
      {children(false)}
    </Popover>
  );
}

/** A form inside a sheet whose head names it already: it draws no caption of its own. */
const HeadedContext = createContext(false);

/** Whether the form is in a sheet whose head names it (`FormPopup`'s on a phone), so that it needs no caption. */
export const useHeaded = () => use(HeadedContext);

/**
 * A small form (the schedule's) opened by its trigger, a React Aria pressable, apart from the menus, which choose and
 * close, where this one is filled in and sent; on the one menu's surface: on a phone a sheet from the bottom edge,
 * headed by its name and close, from `tablet` a popover under the trigger at its start, or over it where there is no
 * room below, as wide as what it holds. Escape, a press outside or close closes it, and focus returns to the trigger.
 */
export function FormPopup({
  open,
  onOpenChange,
  trigger,
  label,
  children,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  trigger: ReactElement;
  /** The form's name: its sheet's title, and what assistive technology reads it by. */
  label: string;
  children: ReactNode;
}) {
  const reader = useReader(undefined);
  const { t } = stringsIn(reader.lang);
  // Open as it mounts (the composer opened on it, by Change time…), it opens a frame later: a modal it is in (the
  // composer's) makes all but itself inert as it mounts, after the effects of what it holds, this popover's among them.
  const [mounted, setMounted] = useState(false);
  useEffect(() => {
    setMounted(true);
  }, []);
  const shown = open && mounted;
  const close = () => {
    onOpenChange(false);
  };
  return (
    <DialogTrigger isOpen={shown} onOpenChange={onOpenChange}>
      {trigger}
      <SurfaceLayer shown={shown} onDismiss={close} placement="bottom start" offset={4}>
        {(inSheet) => (
          <Dialog aria-label={label} className={menu({ sheet: inSheet }).panel()} data-popup="">
            {inSheet && <MenuHead title={label} close={{ label: t("menu.close"), onPress: close }} />}
            <HeadedContext value={inSheet}>
              {/* In a sheet, what it holds lines up with the head's name, as a menu's items do, and scrolls under it. */}
              <div
                className={
                  inSheet
                    ? "min-h-0 overflow-y-auto px-[calc(var(--control-timeline-padding-inline)-0.25rem)]"
                    : undefined
                }
              >
                {children}
              </div>
            </HeadedContext>
          </Dialog>
        )}
      </SurfaceLayer>
    </DialogTrigger>
  );
}

/**
 * An icon button that opens a menu of `children`, from `tablet` in a popover under it, at its end or its start. Like
 * the icon button, it may show a count and an on state (boost, whose menu holds boost and quote).
 */
export function MenuButton({
  icon,
  label,
  menuLabel,
  count,
  pressed,
  align = "end",
  open,
  onOpenChange,
  sub,
  "aria-keyshortcuts": keyshortcuts,
  children,
}: {
  icon: IconName;
  /** The button's name, which also names the menu unless `menuLabel` does. */
  label: string;
  /** The menu's name, where the button's says what it holds now (the composer's audience). */
  menuLabel?: string | undefined;
  /** How many have done it, as the icon button shows it (boost); zero or none shows no count. */
  count?: number | undefined;
  /** The on state of what the menu toggles, as the icon button shows it (boost). */
  pressed?: boolean | undefined;
  /** Which edge of the button the menu lines up with: its end (more, at the row's end) or its start. */
  align?: "start" | "end";
  /** The menu is open, when its owner opens it too (a shortcut), with `onOpenChange`; else the button alone does. */
  open?: boolean | undefined;
  onOpenChange?: ((open: boolean) => void) | undefined;
  /** The submenu shown in its place (`MenuSurface`). */
  sub?: MenuSurfaceProps["sub"];
  /** The keys that open the menu (what `useAction` returns). */
  "aria-keyshortcuts"?: string | undefined;
  children: ReactNode;
}) {
  return (
    <MenuSurface
      trigger={
        <IconButton icon={icon} label={label} count={count} pressed={pressed} aria-keyshortcuts={keyshortcuts} />
      }
      label={menuLabel ?? label}
      placement={`bottom ${align}`}
      open={open}
      onOpenChange={onOpenChange}
      sub={sub}
    >
      {children}
    </MenuSurface>
  );
}
