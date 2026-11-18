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
// what stands in the list's place while it is empty, which leads to adding; a sentence that is no setting, with the
// links to where the reader acts on it; what they copy and keep (recovery codes, a key), with the action that copies
// it; or a QR code for a camera, dark on light in either scheme. The status summary heads a screen while
// something there is to do: the state as a glyph and a sentence, what it means, and the next thing to do. A state is
// always words and a glyph, on no surface of its own: a tile is what can be pressed.
//
// A neutral control on a raised surface takes the page's surface (`raisedSurface`); the accent stays the one primary
// action's of a screen, and nothing here takes another hue.
//
// For assistive technology: the menu is a navigation, each group a region named by its heading, the entry the reader
// is at the current page, a count read by its label. A section is a region named by its heading, one level under the
// screen's own. A setting's row names its control by its name and describes it by the line about it and by its save
// state, whose place is a polite live region from the start. A save state and what came of sending a form are read
// as they come, and the focus stays where it is. What the rules of ADR-0110 §6 ask of a rendered screen, and the lint
// cannot count in the source, `settingsFindings` (test/settings.ts) reads from the marks the parts leave
// (`data-settings-…`).

import { createContext, Fragment, use, useId, type ReactElement, type ReactNode } from "react";
import { Link } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { raisedSurface, TextLink } from "./button";
import { FieldContext } from "./field";
import { Icon, type IconName } from "./icons";
import { Box, Grid, Layers, Row, Stack } from "./layout";
import { ScreenTitle } from "./list";
import { useReader } from "./locale";
import { SaveControlContext } from "./save-state";
import { actionJoint } from "./sentence";
import { FrameFoot, FrameHead, FramePanel } from "./shell";

/** Whether the menu stands beside a screen, on the page, its entries their glyph and name alone. */
const CompactContext = createContext(false);

/**
 * The level of a section's heading: one under the screen's own name. The head of an entry's screen is the screen's
 * heading again, hidden where the menu beside it marks the entry, so its sections stand right under the screen's
 * heading; a screen under an entry keeps its head, and its sections stand under that.
 */
const SectionLevelContext = createContext<2 | 3>(3);

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
    <Stack as="section" gap="screen-air" aria-labelledby={id} data-settings-group="">
      {parts}
    </Stack>
  ) : (
    <FramePanel as={<section aria-labelledby={id} data-settings-group="" />} gap="screen-air">
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
  /** What the count is, for assistive technology ("1 unread"); without it, the number alone is read. */
  countLabel?: string | undefined;
  href: string;
  current?: boolean;
  away?: boolean;
}) {
  const compact = use(CompactContext);
  const s = menu({ compact });
  const shown = useCount(count);
  return (
    // The row fills the entry. Its parts are a space apart in its name ("Security Password, passkeys…", "Account and
    // data 1 unread"), which no box of the row shows.
    <Stack as="li">
      <LinkRow as={<Link href={href} className={s.item()} {...(current ? { "aria-current": "page" as const } : {})} />}>
        <Row as="span" inline rigid className={s.glyph()}>
          <Icon name={icon} place="action" on={compact && current} />
        </Row>
        <Stack as="span" grow>
          <span className={s.title()}>{label}</span>
          {hint !== undefined && !compact && (
            <>
              {" "}
              <span className={s.hint()}>{hint}</span>
            </>
          )}
        </Stack>
        {shown !== undefined && (
          <>
            {" "}
            <Row
              as="span"
              inline
              rigid
              align="center"
              justify="center"
              className={s.badge()}
              aria-hidden={countLabel === undefined ? undefined : "true"}
            >
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
 * width. While it has no section to show, `notice` stands in their place: that what it shows is being read, or could
 * not be, with the way to try again (a `FeedNotice`).
 */
export function SettingsScreen({
  icon,
  title,
  back,
  sub = false,
  notice,
  foot,
  children,
}: {
  /** The entry's glyph; a screen under an entry has none. */
  icon?: IconName | undefined;
  title: string;
  /** The way back (a `BackButton`): to the menu, or to the entry of a screen under it. */
  back?: ReactNode;
  sub?: boolean;
  /** What stands in the sections' place while there are none: that they load, or could not be read. */
  notice?: ReactNode;
  foot?: ReactNode;
  children?: ReactNode;
}) {
  const head = (
    <ScreenTitle glyph={icon} back={back}>
      {title}
    </ScreenTitle>
  );
  return (
    <Stack gap="shell-edge" className={screen()} data-settings-screen="">
      {sub ? <div data-frame-head="">{head}</div> : <FrameHead>{head}</FrameHead>}
      {notice}
      <SectionLevelContext value={sub ? 3 : 2}>{children}</SectionLevelContext>
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
 * A section of a settings screen, a raised surface under its heading, which names it as a region, with how many it
 * lists after it, muted, a line that describes it, and `action` at the end of the heading's line (the one add action
 * of its list). What it holds stands one under another, each one thing the reader may do there (a setting, a list,
 * what stands in an empty list's place): seven at most, `action` among them (ADR-0110 §6).
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
  const Heading = `h${use(SectionLevelContext)}` as const;
  return (
    <FramePanel as={<section aria-labelledby={id} data-settings-section="" {...raisedSurface} />} gap="screen-row-gap">
      <Stack gap="hair">
        <Row wrap align="center" justify="between" gap="screen-tabs-gap">
          <Heading id={id} className={s.heading()}>
            {heading}
            {count !== undefined && (
              <>
                {" "}
                <span className={s.count()}>{count}</span>
              </>
            )}
          </Heading>
          {action !== undefined && (
            <Row as="span" inline rigid push="end" data-settings-action="">
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

/** Whether a list stands under a setting as the values it holds (`values` of a `SettingRow`). */
const ValuesContext = createContext(false);

/**
 * The frame of a setting: its name (`name`) over the line under it (`under`) at the start, taking the room left on
 * their line, and its control (`control`) at the end, as wide as what it holds. Beside the control the name's block
 * stands at the middle of the control, with or without a line under it. Where the two cannot share a line, the
 * control fills a line of its own right under the block, as a field under its label, and the row's space stays above
 * the name. `reserve`, for a name with no line under it yet (a save state will stand there): on a line of its own the
 * name keeps that line's room above it, so that the state finds its room and the control under it does not move. The
 * block is on a line of its own when it is as wide as the frame, which the padding reads (a length that is the room
 * or nothing).
 */
function SettingFrame({
  className,
  reserve,
  name,
  under,
  control,
}: {
  className: string;
  reserve: boolean;
  name: ReactNode;
  under: ReactNode;
  control: ReactNode;
}) {
  const room =
    "pt-[clamp(0px,(100%_-_100cqw_+_1px)_*_9999,calc(var(--text-caption)_*_var(--text-caption--line-height)))]";
  return (
    <Row wrap align="center" className={`${className} @container gap-x-screen-row-gap gap-y-screen-air`}>
      <Stack grow="setting" pack="center">
        <Box className={reserve ? room : undefined}>{name}</Box>
        {under}
      </Stack>
      <Box grow="control">{control}</Box>
    </Row>
  );
}

const row = tv({
  slots: {
    // A finger's target tall, which holds the name and one line under it beside the control.
    root: "min-h-icon-button-touch",
    label: "[word-break:auto-phrase]",
    // The name of a setting that cannot be changed now, muted as every disabled label is.
    off: "text-muted-text",
    about: "text-caption text-muted-text [word-break:auto-phrase]",
    // The line about the setting while the save state stands in its place: it keeps its room.
    covered: "invisible",
  },
});

export type SettingRowProps = {
  /** The id of the name, which names the control. */
  id: string;
  label: string;
  about?: string | undefined;
  /** It cannot be changed now: its control is disabled, and `reason` says why, in place of the line about it. */
  isDisabled?: boolean | undefined;
  reason?: string | undefined;
  /**
   * What the setting holds, where it holds several and its control adds one (a `SettingsList` of the languages
   * chosen): close under the row, as the setting's own.
   */
  values?: ReactNode;
  children: ReactNode;
} & (
  | {
      /**
       * The save state of its control, which saves at once (a `SaveState`), in the place of the line about it;
       * `null` while it has nothing to say.
       */
      save: ReactNode | null;
      form?: undefined;
    }
  | {
      /** It is a field of a form saved as a whole: the form's submit bar says what came of sending. */
      form: true;
      save?: undefined;
    }
);

/**
 * A setting: its name over a line about it at the row's start, its control (a `Select`, a `NumberField`) at the
 * row's end, as wide as what it holds; where the two cannot share a line, the control fills a line of its own under
 * the name, as a field under its label. Its control either saves at once, and the row takes its save state (`save`,
 * ADR-0110 §6), which stands in the place of the line about it, or, with no such line, under the name, in the room
 * the row's height holds beside the control (the name, at the middle of the control until then, gives way to it, and
 * the control stays where it is); or it is a field of a form saved as a whole (`form`), with no state of its own.
 * What the setting holds beyond its control's value (`values`) stands close under the row. The row names its control
 * (`id`, the control's `labelledBy`, which a `Select` and a `NumberField` take from the row by themselves), describes
 * it by the line about it and by the save state, and a press on the name puts the focus on it.
 */
export function SettingRow({
  id,
  label,
  about,
  isDisabled = false,
  reason,
  values,
  save,
  form,
  children,
}: SettingRowProps) {
  const s = row();
  const saves = form !== true;
  const saying = saves && save !== undefined && save !== null && save !== false;
  const below = isDisabled && reason !== undefined ? reason : about;
  // The control takes this id from the row where it has none of its own.
  const controlId = `${id}-control`;
  const aboutId = `${id}-about`;
  const stateId = `${id}-state`;
  const setting = (
    <SettingFrame
      className={s.root()}
      reserve={saves && below === undefined && !saying}
      name={
        <label id={id} htmlFor={controlId} className={isDisabled ? `${s.label()} ${s.off()}` : s.label()}>
          {label}
        </label>
      }
      under={
        (below !== undefined || saves) && (
          <Layers>
            {below !== undefined && (
              <span id={aboutId} className={saying ? `${s.about()} ${s.covered()}` : s.about()}>
                {below}
              </span>
            )}
            {/* The state's place, a live region that is there before the state is. */}
            {saves && (
              <span id={stateId} role="status">
                <SaveControlContext value={() => document.getElementById(controlId)?.focus()}>
                  {saying && save}
                </SaveControlContext>
              </span>
            )}
          </Layers>
        )
      }
      control={
        <FieldContext
          value={{
            id: controlId,
            labelledBy: id,
            describedBy: below === undefined ? undefined : aboutId,
            stateBy: saying ? stateId : undefined,
            disabled: isDisabled || undefined,
          }}
        >
          {children}
        </FieldContext>
      }
    />
  );
  if (values === undefined || values === null || values === false) return setting;
  return (
    <Stack gap="screen-air">
      {setting}
      <ValuesContext value>{values}</ValuesContext>
    </Stack>
  );
}

const summary = tv({
  slots: {
    title: "text-body font-strong [word-break:auto-phrase]",
    about: "text-caption text-muted-text [word-break:auto-phrase]",
  },
});

/**
 * The status summary that heads a screen while something there is to do (the security's, while two-factor
 * authentication is off): the state as its glyph and a sentence, what it means, and the next thing to do in a word or
 * two (a `Button`, the screen's primary action). It is the one place the screen says it and the one door to it: no
 * row under it repeats either. A state is words and a glyph, never a tile, which is what can be pressed.
 */
export function StatusSummary({
  icon,
  title,
  about,
  action,
}: {
  /** The state's glyph: `caution` while something is to do, `accepted` once all is well. */
  icon: IconName;
  /** The state, as a sentence ("Two-factor authentication is off"). */
  title: string;
  about: string;
  action?: ReactNode;
}) {
  const id = useId();
  const s = summary();
  const Heading = `h${use(SectionLevelContext)}` as const;
  return (
    <FramePanel as={<section aria-labelledby={id} data-settings-section="" {...raisedSurface} />} gap="screen-row-gap">
      <Row align="start" gap="screen-row-gap">
        <Row as="span" inline line rigid align="center">
          <Icon name={icon} place="action" />
        </Row>
        <Stack grow gap="hair">
          <Heading id={id} className={s.title()}>
            {title}
          </Heading>
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
    // The values of a setting, under its row: set in from the setting's start, each as tall as its button alone.
    values: "ps-screen-row-inline",
    value: "min-h-control-timeline",
    link: [
      "min-h-icon-button-touch rounded-control px-screen-row-inline py-screen-air text-text no-underline",
      "outline-hidden data-hovered:bg-surface-raised-hover data-pressed:bg-surface-raised-pressed",
      "data-focus-visible:shadow-focus",
    ],
    glyph: "text-muted-text",
    title: "[word-break:auto-phrase]",
    caption: "text-caption text-muted-text [word-break:auto-phrase]",
    // A fact is a few words around a date or an address: it breaks at its spaces alone, so that neither is cut.
    fact: "text-caption text-muted-text [word-break:keep-all]",
    // A state at the row's end: words alone, muted, with no surface, which is a button's.
    state: "text-caption text-muted-text",
    chevron: "text-muted-text",
    empty: "text-muted-text [word-break:auto-phrase]",
  },
});

/**
 * What a section lists (the passkeys, the signed-in browsers, the filters), named by `label`: its rows. As the
 * `values` of a `SettingRow` (the languages a setting holds), it is the setting's own: set in from its start, its
 * rows close to each other.
 */
export function SettingsList({ label, children }: { label: string; children: ReactNode }) {
  const s = list();
  const values = use(ValuesContext);
  return (
    <Stack
      as="ul"
      gap={values ? "screen-air" : undefined}
      aria-label={label}
      className={values ? `${s.root()} ${s.values()}` : s.root()}
    >
      {children}
    </Stack>
  );
}

/**
 * A row of a section's list: its glyph, a title over a caption or over the facts known of it, one a line (`facts`), a
 * state in a word or two at its end, in muted words with no surface (`state`), and what the reader may do with it (`end`: one or two buttons, a group named by the
 * row's title, so that one Remove is told from the next). With `href`, the whole row opens the screen under the
 * entry that holds it (changing the password, one filter), a chevron at its end in place of buttons.
 */
export function SettingsListRow({
  icon,
  title,
  caption,
  facts,
  state,
  href,
  end,
}: {
  icon?: IconName | undefined;
  title: string;
  caption?: string | undefined;
  /**
   * What is known of it, each fact short and on a line of its own (which browser this is, when it signed in, when it
   * was last used): a line too long for the row breaks between its words, never inside one, in ja too.
   */
  facts?: readonly string[] | undefined;
  state?: string | undefined;
  href?: string | undefined;
  end?: ReactNode;
}) {
  const s = list();
  const titleId = useId();
  const value = use(ValuesContext);
  // Its parts are a space apart in what assistive technology reads of the row, which no box of it shows.
  const parts = (
    <>
      {icon !== undefined && (
        <Row as="span" inline rigid className={s.glyph()}>
          <Icon name={icon} place="action" />
        </Row>
      )}
      <Stack as="span" grow>
        <bdi id={titleId} className={s.title()}>
          {title}
        </bdi>
        {caption !== undefined && (
          <>
            {" "}
            <span className={s.caption()}>{caption}</span>
          </>
        )}
        {facts?.map((fact) => (
          <Fragment key={fact}>
            {" "}
            <span className={s.fact()}>{fact}</span>
          </Fragment>
        ))}
      </Stack>
      {state !== undefined && (
        <>
          {" "}
          <Box as="span" words className={s.state()}>
            {state}
          </Box>
        </>
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
    <Row as="li" align="center" gap="screen-row-gap" className={value ? s.value() : s.row()}>
      {parts}
      {end !== undefined && end !== null && end !== false && (
        <Row as="span" inline rigid align="center" gap="screen-tabs-gap" role="group" aria-labelledby={titleId}>
          {end}
        </Row>
      )}
    </Row>
  );
}

/**
 * What stands in a list's place while it is empty (ADR-0110 §5): a sentence, and the action that adds the first one,
 * which is then the one place to add from: the section's head leaves its `action` out.
 */
export function SettingsEmpty({ action, children }: { action?: ReactNode; children: string }) {
  return (
    <Stack gap="screen-row-gap">
      <p className={list().empty()}>{children}</p>
      {action !== undefined && (
        <Row justify="start" data-settings-action="">
          {action}
        </Row>
      )}
    </Stack>
  );
}

const note = tv({
  slots: {
    root: "text-caption text-muted-text [word-break:auto-phrase]",
    // The values to copy, on the page's surface, in the code's face; a long one breaks anywhere.
    values: "list-none rounded-control bg-surface p-screen-row-inline font-mono text-body text-text select-all",
    value: "wrap-anywhere tabular-nums",
    result: "text-caption text-muted-text",
    // A QR code is dark on light whatever the scheme, or a camera does not read it.
    qr: "rounded-control bg-surface p-screen-air text-text [color-scheme:light]",
    code: "size-screen-qr fill-current",
  },
});

/**
 * A sentence in a section that is no setting and no state of a control: what a list's rows mean for the reader, what
 * an action there will do, why one cannot be done; then the links to where they act on it (the password's screen, the
 * apps), each a link of the text. Muted, at the caption's size.
 */
export function SettingsNote({
  links,
  children,
}: {
  /** Where the reader acts on what the sentence says, after it. */
  links?: readonly { href: string; label: string }[] | undefined;
  children: string;
}) {
  return (
    <p className={note().root()}>
      {children}
      {/* The first link meets the sentence as an action does; each next one stands a middle dot after the one before. */}
      {links?.map((link, i) => (
        <Fragment key={link.href}>
          {i === 0 ? actionJoint(children) : actionJoint(link.label)}
          <TextLink href={link.href}>{link.label}</TextLink>
        </Fragment>
      ))}
    </p>
  );
}

/**
 * What the reader copies and keeps (recovery codes, the key of an authenticator app, a secret shown once), named by
 * `label`: the values in the code's face on the page's surface, each on a line of its own, two side by side where
 * they fit, and under them the action that copies them all (`action`, a `Button`) with what came of it right after
 * (`result`, read as it comes). A press on the values selects them all, for a reader who copies by hand.
 */
export function CopyBlock({
  label,
  values,
  action,
  result,
}: {
  label: string;
  values: readonly string[];
  action: ReactNode;
  /** What came of copying, in a word or two (Copied). */
  result?: string | undefined;
}) {
  const s = note();
  return (
    <Stack gap="screen-row-gap">
      <Grid
        as="ul"
        columns={values.length > 1 ? 2 : 1}
        gap="screen-air"
        gapX="shell-edge"
        aria-label={label}
        className={s.values()}
      >
        {values.map((value) => (
          <li key={value} className={s.value()}>
            {value}
          </li>
        ))}
      </Grid>
      <Row wrap align="center" gap="screen-row-gap">
        <Row as="span" inline rigid>
          {action}
        </Row>
        <span role="status" className={s.result()}>
          {result}
        </span>
      </Row>
    </Stack>
  );
}

/**
 * A QR code for a camera to read (the key of an authenticator app), named by `label`: the dark modules of `path`,
 * one unit a module, in a square view box of `size` modules that holds the quiet zone, dark on light in either
 * scheme.
 */
export function QrFrame({ label, size, path }: { label: string; size: number; path: string }) {
  const s = note();
  return (
    <Row justify="start">
      <Box className={s.qr()}>
        <Box
          display="block"
          as={
            <svg
              role="img"
              aria-label={label}
              viewBox={`0 0 ${String(size)} ${String(size)}`}
              shapeRendering="crispEdges"
              className={s.code()}
            />
          }
        >
          <path d={path} />
        </Box>
      </Box>
    </Row>
  );
}

/**
 * The submit bar of a form saved as a whole: its button (`children`, the screen's primary action) at the end, and
 * what came of sending right before it, in the same place every time (`result`: a `SaveState` once saved, a
 * `FormAlert` when refused). It ends the screen and stays at the bottom of the viewport while the form is longer,
 * the page keeping what takes the focus clear of it. The result's place is a polite live region from the start, so
 * that assistive technology hears what came of sending (a `FormAlert` at once, as an alert).
 */
export function SubmitBar({ result, children }: { result?: ReactNode; children: ReactNode }) {
  return (
    <FrameFoot>
      <Row wrap align="center" justify="end" gap="screen-row-gap">
        <Box role="status">{result}</Box>
        <Row as="span" inline rigid>
          {children}
        </Row>
      </Row>
    </FrameFoot>
  );
}
