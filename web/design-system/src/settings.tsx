// SPDX-License-Identifier: AGPL-3.0-only

// The settings (ADR-0110 §5): the menu of their screens, and a screen of them.
//
// The menu lists the screens in groups, each under its heading: an entry is its glyph, its name over a line of what it
// holds, how many things wait there for the reader (a badge knocked out of the text's color, as the navigation's), and
// a chevron; the whole row leads to its screen. Beside a screen (`compact`), an entry is its glyph, its name and the
// badge alone, and the one the reader is at stands on the raised surface, strong. An entry that does something in
// place of leading somewhere (signing out) is a button of the same row, with no chevron.
//
// A screen is its head (the entry's glyph and name, under the way back to the menu where the menu is not beside it)
// over its sections, each under its heading, which a line may describe. Everything sits on the page's one surface,
// apart by space: tight inside a group, room between groups.

import { createContext, use, useId, type ReactElement, type ReactNode } from "react";
import { Button, Link } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { Icon, type IconName } from "./icons";
import { Row, Stack } from "./layout";
import { ScreenTitle } from "./list";
import { useReader } from "./locale";

/** Whether the menu stands beside a screen, its entries their glyph and name alone. */
const CompactContext = createContext(false);

const menu = tv({
  slots: {
    root: "text-body",
    heading: "text-caption font-normal text-muted-text",
    list: "list-none p-0",
    // The whole row is the link or the button: a surface on hover and press, the ring on focus; the entry the reader
    // is at on the raised surface, strong.
    item: [
      "cursor-pointer rounded-control border-0 bg-transparent px-screen-row-inline py-screen-row-block",
      "text-start font-sans text-body text-text no-underline outline-hidden",
      "data-hovered:bg-surface-hover data-pressed:bg-surface-pressed data-focus-visible:shadow-focus",
      "aria-[current=page]:bg-surface-raised aria-[current=page]:font-strong",
    ],
    glyph: "text-muted-text",
    title: "[word-break:auto-phrase]",
    hint: "text-caption font-normal text-muted-text [word-break:auto-phrase]",
    badge: [
      "box-border h-shell-badge min-w-shell-badge rounded-[calc(var(--shell-badge)/2)]",
      "bg-text px-shell-badge-inline text-[0.6875rem] leading-none font-label text-surface tabular-nums",
    ],
    chevron: "text-muted-text",
  },
  variants: {
    compact: {
      true: { title: "font-[inherit]" },
      false: { title: "font-label" },
    },
  },
});

/**
 * The menu of the settings' screens, a navigation named `label`: its groups (`SettingsGroup`) one under another.
 * `compact`, beside a screen: an entry is its glyph, its name and its badge alone.
 */
export function SettingsMenu({
  label,
  compact = false,
  children,
}: {
  label: string;
  compact?: boolean;
  children: ReactNode;
}) {
  return (
    <Stack
      as="nav"
      gap={compact ? "form-page-form-gap" : "form-page-screen-gap"}
      aria-label={label}
      className={menu().root()}
    >
      <CompactContext value={compact}>{children}</CompactContext>
    </Stack>
  );
}

/** A group of the menu under its heading: its entries (`SettingsItem`), seven at most. */
export function SettingsGroup({ heading, children }: { heading: string; children: ReactNode }) {
  const id = useId();
  const s = menu();
  return (
    <Stack as="section" gap="screen-air" aria-labelledby={id}>
      <h2 id={id} className={s.heading()}>
        {heading}
      </h2>
      <Stack as="ul" className={s.list()}>
        {children}
      </Stack>
    </Stack>
  );
}

/** An entry's row, as `as` (its link, its button): its parts in its middle, its surface reaching past the column. */
function ItemRow({ as, children }: { as: ReactElement<{ className?: unknown }>; children: ReactNode }) {
  return (
    <Row as={as} align="center" gap="screen-row-gap" bleedInline="screen-row-inline">
      {children}
    </Row>
  );
}

/**
 * An entry of the menu: its glyph, its name over a line of what its screen holds, and how many things wait there for
 * the reader. With `href` it leads to its screen, a chevron at its end; with `onPress` it does something in place
 * (signing out). `current` marks the screen the reader is at, where the menu stands beside it.
 */
export function SettingsItem({
  icon,
  label,
  hint,
  count,
  countLabel,
  href,
  onPress,
  current = false,
}: {
  icon: IconName;
  label: string;
  /** What its screen holds, in a few words; left out beside a screen. */
  hint?: string | undefined;
  /** How many things wait there for the reader; none or zero shows no badge. */
  count: number | undefined;
  /** What the count is, for assistive technology ("1 unread"). */
  countLabel?: string | undefined;
  href?: string | undefined;
  onPress?: (() => void) | undefined;
  current?: boolean;
}) {
  const compact = use(CompactContext);
  const s = menu({ compact });
  const { lang } = useReader(undefined);
  const shown = count !== undefined && count > 0 ? new Intl.NumberFormat(lang).format(count) : undefined;
  const parts = (
    <>
      <Row as="span" inline rigid className={s.glyph()}>
        <Icon name={icon} place="action" on={compact && current} />
      </Row>
      <Stack as="span" grow>
        <span className={s.title()}>{label}</span>
        {hint !== undefined && !compact && <span className={s.hint()}>{hint}</span>}
      </Stack>
      {shown !== undefined && (
        <>
          <Row as="span" inline rigid align="center" justify="center" className={s.badge()} aria-hidden="true">
            {shown}
          </Row>
          {countLabel !== undefined && <span className="sr-only">{countLabel}</span>}
        </>
      )}
      {href !== undefined && !compact && (
        <Row as="span" inline rigid className={s.chevron()}>
          <Icon name="onward" place="action" />
        </Row>
      )}
    </>
  );
  return (
    // The row fills the entry, a link or a button alike.
    <Stack as="li">
      {href === undefined ? (
        <ItemRow as={<Button className={s.item()} {...(onPress ? { onPress } : {})} />}>{parts}</ItemRow>
      ) : (
        <ItemRow
          as={<Link href={href} className={s.item()} {...(current ? { "aria-current": "page" as const } : {})} />}
        >
          {parts}
        </ItemRow>
      )}
    </Stack>
  );
}

/**
 * A screen of the settings: its head, the entry's glyph and name under `back` (the way to the menu, where the menu is
 * not beside the screen), over its sections (`SettingsSection`) and nothing else.
 */
export function SettingsScreen({
  icon,
  title,
  back,
  children,
}: {
  icon: IconName;
  title: string;
  back?: ReactNode;
  children: ReactNode;
}) {
  return (
    <Stack gap="form-page-gap" className={section().screen()}>
      <ScreenTitle glyph={icon} back={back}>
        {title}
      </ScreenTitle>
      {children}
    </Stack>
  );
}

const section = tv({
  slots: {
    screen: "text-body",
    heading: "text-body font-strong",
    count: "font-normal text-muted-text tabular-nums",
    about: "text-caption text-muted-text [word-break:auto-phrase]",
  },
});

/**
 * A section of a settings screen under its heading, with how many it lists after it, muted, and a line that describes
 * it: what it holds one under another, seven actions at most.
 */
export function SettingsSection({
  heading,
  count,
  about,
  children,
}: {
  heading: string;
  /** How many it lists, as the reader's locale writes it. */
  count?: string | undefined;
  about?: string | undefined;
  children: ReactNode;
}) {
  const id = useId();
  const s = section();
  return (
    <Stack as="section" gap="screen-row-gap" aria-labelledby={id}>
      <Stack gap="hair">
        <h3 id={id} className={s.heading()}>
          {heading}
          {count !== undefined && (
            <>
              {" "}
              <span className={s.count()}>{count}</span>
            </>
          )}
        </h3>
        {about !== undefined && <p className={s.about()}>{about}</p>}
      </Stack>
      {children}
    </Stack>
  );
}
