// SPDX-License-Identifier: AGPL-3.0-only

// The settings (ADR-0110 §5): the menu of their screens, a screen of them, and what a section holds.
//
// The menu lists the screens in groups, each a raised surface under its heading (ADR-0110 §12): an entry is its glyph,
// its name over a line of what it holds, how many things wait there for the reader (a badge knocked out of the text's
// color, as the navigation's), and a chevron; the whole row leads to its screen. What ends the menu stands apart,
// after a gap (signing out, which leaves the app: no chevron). Beside a screen, in the column beside the main one
// (`compact`), the menu sits on the page as that column's sections do: an entry is its glyph, its name and the badge
// alone, and the one the reader is at stands on the raised surface, strong.
//
// A screen is its sections one under another, each a raised surface under its heading, and what ends it (the submit
// bar of a form saved as a whole). Its head, the entry's glyph and name under the way back to the menu, shows where the
// menu is not beside the screen; a screen under an entry (`sub`: changing the password, one filter) keeps its head at
// every width, the way back leading to its entry. A section holds rows: a setting (its name over a line about it, the
// control at the row's end), a list of what the reader keeps there with its one add action in the section's head, or
// what stands in the list's place while it is empty, which leads to adding. The status summary heads a screen: a
// state as a glyph and a badge, what it means, and the next thing to do.
//
// A neutral control on a raised surface takes the page's surface (`raisedSurface`); the accent stays the one primary
// action's of a screen, and nothing here takes another hue.

import { createContext, use, useId, type ReactElement, type ReactNode } from "react";
import { Link } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { raisedSurface } from "./button";
import { Badge } from "./chip";
import { Icon, type IconName } from "./icons";
import { Box, Layers, Row, Stack } from "./layout";
import { ScreenTitle } from "./list";
import { useReader } from "./locale";
import { FrameFoot, FrameHead, FramePanel } from "./shell";

/** Whether the menu stands beside a screen, on the page, its entries their glyph and name alone. */
const CompactContext = createContext(false);

const menu = tv({
  slots: {
    root: "text-body",
    heading: "text-caption font-normal text-muted-text",
    list: "list-none p-0",
    // The whole row is the link: a surface on hover and press, the ring on focus.
    item: [
      "rounded-control px-screen-row-inline py-screen-row-block",
      "text-body text-text no-underline outline-hidden data-focus-visible:shadow-focus",
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
      // On the page: the entry the reader is at on the raised surface, strong.
      true: {
        item: [
          "data-hovered:bg-surface-hover data-pressed:bg-surface-pressed",
          "aria-[current=page]:bg-surface-raised aria-[current=page]:font-strong",
        ],
      },
      // On its group's raised surface.
      false: {
        item: "data-hovered:bg-surface-raised-hover data-pressed:bg-surface-raised-pressed",
        title: "font-label",
      },
    },
  },
});

/**
 * The menu of the settings' screens, a navigation named `label`: its groups (`SettingsGroup`) one under another, and
 * `end`, the entry that ends it apart (signing out). `compact`, beside a screen: on the page, an entry its glyph, its
 * name and its badge alone.
 */
export function SettingsMenu({
  label,
  compact = false,
  end,
  children,
}: {
  label: string;
  compact?: boolean;
  /** The entry that ends the menu, apart from its groups (a `SettingsItem`). */
  end?: ReactNode;
  children: ReactNode;
}) {
  const s = menu({ compact });
  return (
    <Stack as="nav" gap={compact ? "form-page-form-gap" : "shell-edge"} aria-label={label} className={s.root()}>
      <CompactContext value={compact}>
        {children}
        {end !== undefined &&
          (compact ? (
            <Stack as="ul" className={s.list()}>
              {end}
            </Stack>
          ) : (
            <FramePanel as={<ul className={s.list()} />} gap="screen-air">
              {end}
            </FramePanel>
          ))}
      </CompactContext>
    </Stack>
  );
}

/** A group of the menu under its heading: its entries (`SettingsItem`), seven at most. */
export function SettingsGroup({ heading, children }: { heading: string; children: ReactNode }) {
  const id = useId();
  const compact = use(CompactContext);
  const s = menu({ compact });
  const parts = (
    <>
      <h2 id={id} className={s.heading()}>
        {heading}
      </h2>
      <Stack as="ul" className={s.list()}>
        {children}
      </Stack>
    </>
  );
  return compact ? (
    <Stack as="section" gap="screen-air" aria-labelledby={id}>
      {parts}
    </Stack>
  ) : (
    <FramePanel as={<section aria-labelledby={id} />} gap="screen-air">
      {parts}
    </FramePanel>
  );
}

/** A row that is one link, as `as`: its parts in its middle, its surface reaching past what it holds. */
function LinkRow({ as, children }: { as: ReactElement<{ className?: unknown }>; children: ReactNode }) {
  return (
    <Row as={as} align="center" gap="screen-row-gap" bleedInline="screen-row-inline">
      {children}
    </Row>
  );
}

/** A count as its badge shows it, in the reader's numbers; none or zero shows no badge. */
function useCount(count: number | undefined): string | undefined {
  const { lang } = useReader(undefined);
  return count !== undefined && count > 0 ? new Intl.NumberFormat(lang).format(count) : undefined;
}

/**
 * An entry of the menu, a link to its screen: its glyph, its name over a line of what the screen holds, how many
 * things wait there for the reader, and a chevron. `current` marks the screen the reader is at, where the menu stands
 * beside it. `away`: it leaves the app (signing out), so it ends with no chevron.
 */
export function SettingsItem({
  icon,
  label,
  hint,
  count,
  countLabel,
  href,
  current = false,
  away = false,
}: {
  icon: IconName;
  label: string;
  /** What its screen holds, in a few words; left out beside a screen. */
  hint?: string | undefined;
  /** How many things wait there for the reader; none or zero shows no badge. */
  count: number | undefined;
  /** What the count is, for assistive technology ("1 unread"). */
  countLabel?: string | undefined;
  href: string;
  current?: boolean;
  away?: boolean;
}) {
  const compact = use(CompactContext);
  const s = menu({ compact });
  const shown = useCount(count);
  return (
    // The row fills the entry.
    <Stack as="li">
      <LinkRow as={<Link href={href} className={s.item()} {...(current ? { "aria-current": "page" as const } : {})} />}>
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
        {!away && !compact && (
          <Row as="span" inline rigid className={s.chevron()}>
            <Icon name="onward" place="action" />
          </Row>
        )}
      </LinkRow>
    </Stack>
  );
}

const screen = tv({ base: "text-body" });

/**
 * A screen of the settings: its sections (`SettingsSection`, a `StatusSummary` first) and nothing else, then `foot`,
 * what ends it (a `SubmitBar`). Its head is the entry's glyph and name under `back`, the way to the menu, where the
 * menu is not beside the screen. `sub`, a screen under an entry: its name under the way back to that entry, at every
 * width.
 */
export function SettingsScreen({
  icon,
  title,
  back,
  sub = false,
  foot,
  children,
}: {
  /** The entry's glyph; a screen under an entry has none. */
  icon?: IconName | undefined;
  title: string;
  /** The way back (a `BackButton`): to the menu, or to the entry of a screen under it. */
  back?: ReactNode;
  sub?: boolean;
  foot?: ReactNode;
  children: ReactNode;
}) {
  const head = (
    <ScreenTitle glyph={icon} back={back}>
      {title}
    </ScreenTitle>
  );
  return (
    <Stack gap="shell-edge" className={screen()}>
      {sub ? head : <FrameHead>{head}</FrameHead>}
      {children}
      {foot}
    </Stack>
  );
}

const section = tv({
  slots: {
    heading: "text-body font-strong",
    count: "font-normal text-muted-text tabular-nums",
    about: "text-caption text-muted-text [word-break:auto-phrase]",
  },
});

/**
 * A section of a settings screen, a raised surface under its heading, with how many it lists after it, muted, a line
 * that describes it, and `action` at the end of the heading's line (the one add action of its list): what it holds
 * one under another, seven actions at most.
 */
export function SettingsSection({
  heading,
  count,
  about,
  action,
  children,
}: {
  heading: string;
  /** How many it lists, as the reader's locale writes it. */
  count?: string | undefined;
  about?: string | undefined;
  /** What the reader may do with the section as a whole, at the end of the heading's line (adding to its list). */
  action?: ReactNode;
  children: ReactNode;
}) {
  const id = useId();
  const s = section();
  return (
    <FramePanel as={<section aria-labelledby={id} {...raisedSurface} />} gap="screen-row-gap">
      <Stack gap="hair">
        <Row wrap align="center" justify="between" gap="screen-tabs-gap">
          <h3 id={id} className={s.heading()}>
            {heading}
            {count !== undefined && (
              <>
                {" "}
                <span className={s.count()}>{count}</span>
              </>
            )}
          </h3>
          {action !== undefined && (
            <Row as="span" inline rigid push="end">
              {action}
            </Row>
          )}
        </Row>
        {about !== undefined && <p className={s.about()}>{about}</p>}
      </Stack>
      {children}
    </FramePanel>
  );
}

const row = tv({
  slots: {
    // A finger's target tall, which holds the name and one line under it, so that a save state moves nothing.
    root: "min-h-icon-button-touch",
    label: "[word-break:auto-phrase]",
    about: "text-caption text-muted-text [word-break:auto-phrase]",
    // The line about the setting while the save state stands in its place: it keeps its room.
    covered: "invisible",
  },
});

/**
 * A setting: its name over a line about it at the row's start, its control (a `Select`, a `NumberField`) at the
 * row's end, on a next line where they do not fit side by side. A control that saves at once gives its save state
 * (`save`, a `SaveState`), which stands in the place of the line about it while it has something to say. The control
 * is named by the row's name (`id`, its `labelledBy`).
 */
export function SettingRow({
  id,
  label,
  about,
  save,
  children,
}: {
  /** The id of the name, which names the control. */
  id: string;
  label: string;
  about?: string | undefined;
  save?: ReactNode | null | undefined;
  children: ReactNode;
}) {
  const s = row();
  const saving = save !== undefined && save !== null;
  return (
    // The name takes the room left, so that the control stands at the row's end beside it, and at the row's start
    // under it, as a field under its label, where they do not fit side by side.
    <Row wrap align="center" gap="screen-row-gap" className={s.root()}>
      <Stack grow="setting">
        <span id={id} className={s.label()}>
          {label}
        </span>
        {(about !== undefined || saving) && (
          <Layers>
            {about !== undefined && <span className={saving ? `${s.about()} ${s.covered()}` : s.about()}>{about}</span>}
            {saving && save}
          </Layers>
        )}
      </Stack>
      {children}
    </Row>
  );
}

const summary = tv({
  slots: {
    title: "text-body font-strong [word-break:auto-phrase]",
    about: "text-caption text-muted-text [word-break:auto-phrase]",
  },
});

/**
 * The status summary that heads a screen (the security's): a state as its glyph and a badge beside its name, what it
 * means in a sentence, and the next thing to do (a `Button`, the screen's primary action while something is to do).
 */
export function StatusSummary({
  icon,
  title,
  badge,
  about,
  action,
}: {
  /** The state's glyph: `caution` while something is to do, `accepted` once all is well. */
  icon: IconName;
  title: string;
  /** The state in a word or two ("Off"). */
  badge?: string | undefined;
  about: string;
  action?: ReactNode;
}) {
  const id = useId();
  const s = summary();
  return (
    <FramePanel as={<section aria-labelledby={id} {...raisedSurface} />} gap="screen-row-gap">
      <Row align="start" gap="screen-row-gap">
        <Row as="span" inline line rigid align="center">
          <Icon name={icon} place="action" />
        </Row>
        <Stack grow gap="hair">
          <Row wrap align="center" gap="screen-tabs-gap">
            <h3 id={id} className={s.title()}>
              {title}
            </h3>
            {badge !== undefined && <Badge>{badge}</Badge>}
          </Row>
          <p className={s.about()}>{about}</p>
        </Stack>
      </Row>
      {action !== undefined && <Row justify="start">{action}</Row>}
    </FramePanel>
  );
}

const list = tv({
  slots: {
    root: "list-none p-0",
    // A row is a finger's target tall with a little air, a link or not, so that every list keeps one rhythm.
    row: "min-h-icon-button-touch py-screen-air",
    link: [
      "min-h-icon-button-touch rounded-control px-screen-row-inline py-screen-air text-text no-underline",
      "outline-hidden data-hovered:bg-surface-raised-hover data-pressed:bg-surface-raised-pressed",
      "data-focus-visible:shadow-focus",
    ],
    glyph: "text-muted-text",
    title: "[word-break:auto-phrase]",
    caption: "text-caption text-muted-text [word-break:auto-phrase]",
    chevron: "text-muted-text",
    empty: "text-muted-text [word-break:auto-phrase]",
  },
});

/** What a section lists (the passkeys, the signed-in browsers, the filters), named by `label`: its rows. */
export function SettingsList({ label, children }: { label: string; children: ReactNode }) {
  return (
    <Stack as="ul" aria-label={label} className={list().root()}>
      {children}
    </Stack>
  );
}

/**
 * A row of a section's list: its glyph, a title over a caption, a state in a word (`badge`), and at its end what the
 * reader may do with it (`end`: one or two buttons). With `href`, the whole row opens the screen under the entry
 * that holds it (changing the password, one filter), a chevron at its end in place of buttons.
 */
export function SettingsListRow({
  icon,
  title,
  caption,
  badge,
  href,
  end,
}: {
  icon?: IconName | undefined;
  title: string;
  caption?: string | undefined;
  badge?: string | undefined;
  href?: string | undefined;
  end?: ReactNode;
}) {
  const s = list();
  const parts = (
    <>
      {icon !== undefined && (
        <Row as="span" inline rigid className={s.glyph()}>
          <Icon name={icon} place="action" />
        </Row>
      )}
      <Stack as="span" grow>
        <bdi className={s.title()}>{title}</bdi>
        {caption !== undefined && <span className={s.caption()}>{caption}</span>}
      </Stack>
      {badge !== undefined && (
        <Row as="span" inline rigid>
          <Badge>{badge}</Badge>
        </Row>
      )}
    </>
  );
  if (href !== undefined)
    return (
      <Stack as="li">
        <LinkRow as={<Link href={href} className={s.link()} />}>
          {parts}
          <Row as="span" inline rigid className={s.chevron()}>
            <Icon name="onward" place="action" />
          </Row>
        </LinkRow>
      </Stack>
    );
  return (
    <Row as="li" align="center" gap="screen-row-gap" className={s.row()}>
      {parts}
      {end !== undefined && (
        <Row as="span" inline rigid align="center" gap="screen-tabs-gap">
          {end}
        </Row>
      )}
    </Row>
  );
}

/**
 * What stands in a list's place while it is empty: a sentence, and the action that adds the first one, which the
 * section's head then leaves out.
 */
export function SettingsEmpty({ action, children }: { action?: ReactNode; children: string }) {
  return (
    <Stack gap="screen-row-gap">
      <p className={list().empty()}>{children}</p>
      {action !== undefined && <Row justify="start">{action}</Row>}
    </Stack>
  );
}

/**
 * The submit bar of a form saved as a whole: its button (`children`, the screen's primary action) at the end, and
 * what came of sending right before it, in the same place every time (`result`: a `SaveState` once saved, a
 * `FormAlert` when refused). It ends the screen and stays at the bottom of the viewport while the form is longer.
 */
export function SubmitBar({ result, children }: { result?: ReactNode; children: ReactNode }) {
  return (
    <FrameFoot>
      <Row wrap align="center" justify="end" gap="screen-row-gap">
        {result !== undefined && <Box>{result}</Box>}
        <Row as="span" inline rigid>
          {children}
        </Row>
      </Row>
    </FrameFoot>
  );
}
