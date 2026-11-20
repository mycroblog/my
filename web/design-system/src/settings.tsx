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
// what stands in the list's place while it is empty, which leads to adding; the facts it states, which are not
// pressed and stand closer than rows; a sentence that is no setting, with the
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

import {
  createContext,
  Fragment,
  use,
  useId,
  type ReactElement,
  type ReactNode,
  type Ref,
  type RefCallback,
} from "react";
import { Link } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { raisedSurface, TextLink } from "./button";
import { CodeLines } from "./code-text";
import { describedBy, FieldContext } from "./field";
import { FieldError } from "./form-page";
import { Icon, type IconName } from "./icons";
import { Box, Grid, Layers, Row, Stack } from "./layout";
import { BackLead, ScreenTitle } from "./list";
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
 * not be, with the way to try again (a `FeedNotice`). A screen that takes another's place under the same address
 * (the recovery codes, once two-factor authentication is on) moves the focus to its title, which `titleRef` reaches.
 */
export function SettingsScreen({
  icon,
  title,
  titleRef,
  back,
  sub = false,
  notice,
  foot,
  children,
}: {
  /** The entry's glyph; a screen under an entry has none. */
  icon?: IconName | undefined;
  title: string;
  /** Reaches the title, which then can take the focus. */
  titleRef?: Ref<HTMLHeadingElement> | undefined;
  /** The way back (a `BackButton`): to the menu, or to the entry of a screen under it. */
  back?: ReactNode;
  sub?: boolean;
  /** What stands in the sections' place while there are none: that they load, or could not be read. */
  notice?: ReactNode;
  foot?: ReactNode;
  children?: ReactNode;
}) {
  // The way back is the reader's only one where it shows: as the text reads, a finger's target tall.
  const head = (
    <ScreenTitle glyph={icon} back={back === undefined ? undefined : <BackLead value>{back}</BackLead>} ref={titleRef}>
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
 * The frame of a setting: its name (`name`), its control (`control`) and the line under the name (`under`), in a
 * frame that knows its own width.
 *
 * - In a wide frame (a tablet's column and wider): the name over the line under it at the start, the control at the
 *   end, at the middle of both.
 * - In a narrow one (a phone): the name and the control share the first line, the control at its end, and the line
 *   under the name runs the frame's whole width under both, where it reads in fewer lines. Where the two cannot share
 *   a line, the control fills a line of its own right under the name, as a field under its label, and the line about
 *   it comes under the control.
 *
 * `state` is the place of a save state for a name with no line under it (with one, the state stands in that line's
 * place): under the name beside the control, in the room the row's height holds, and beside the name when the name
 * has a line to itself, so that it moves neither the control nor the rows. The name has a line to itself when its
 * block is as wide as the frame, which the state's width reads (a length that is the whole line or nothing).
 */
function SettingFrame({
  className,
  name,
  state,
  under,
  control,
  across,
}: {
  className: string;
  name: ReactNode;
  state: ReactNode;
  under: ReactNode;
  control: ReactNode;
  /** The control fills a line of its own at every width. */
  across: boolean;
}) {
  const lined = under !== undefined && under !== null && under !== false;
  // In a wide frame: two rows, the name over the line under it, and the control beside both at their middle; a name
  // with no line under it takes both rows, at the control's middle. A control that goes across stands in a third
  // row, the frame's width, a little under the two.
  const wide = {
    root: "@min-[28rem]:grid @min-[28rem]:grid-cols-[minmax(0,1fr)_auto] @min-[28rem]:content-center @min-[28rem]:gap-y-0",
    name: "@min-[28rem]:col-start-1 @min-[28rem]:row-start-1",
    alone: "@min-[28rem]:row-span-2 @min-[28rem]:self-center",
    control: "@min-[28rem]:col-start-2 @min-[28rem]:row-span-2 @min-[28rem]:row-start-1 @min-[28rem]:self-center",
    across: "@min-[28rem]:col-[1/-1] @min-[28rem]:row-start-3 @min-[28rem]:mt-screen-air",
    under: "@min-[28rem]:col-start-1 @min-[28rem]:row-start-2 @min-[28rem]:mt-0 @min-[28rem]:mb-0",
  };
  // In a narrow frame the name stands on its control's line, at the control's middle, as a name with no line under
  // it does, and the line about it under the two, a little air from the control and nearer to them than to the next
  // setting by the room after it: it reads as its own, and no control stands above its name's line.
  const narrow = { under: "mb-screen-field-gap" };
  const named = `flex min-w-0 flex-[9999_1_6rem] flex-wrap items-center gap-x-screen-row-gap ${wide.name}`;
  return (
    <div className="@container">
      <div className={`${className} flex flex-wrap items-center gap-x-screen-row-gap gap-y-screen-air ${wide.root}`}>
        <div className={lined || across ? named : `${named} ${wide.alone}`}>
          <div className="max-w-full min-w-0 wrap-break-word">{name}</div>
          {state !== undefined && state !== null && state !== false && (
            <div className="min-w-0 grow basis-[clamp(0px,(100cqw_-_1px_-_100%)_*_9999,100%)] wrap-break-word">
              {state}
            </div>
          )}
        </div>
        {/* In a narrow frame a control that goes across is a dropped one at every width: the line comes under it. */}
        <div className={`min-w-0 ${across ? `basis-full ${wide.across}` : `flex-[1_1_auto] ${wide.control}`}`}>
          {control}
        </div>
        {lined && <div className={`min-w-0 basis-full wrap-break-word ${narrow.under} ${wide.under}`}>{under}</div>}
      </div>
    </div>
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
  /** Its control fills a line of its own under the name at every width: one that grows with what it holds (tags). */
  wide?: boolean | undefined;
  children: ReactNode;
} & (
  | {
      /**
       * The save state of its control, which saves at once (a `SaveState`), in the place of the line about it;
       * `null` while it has nothing to say.
       */
      save: ReactNode | null;
      form?: undefined;
      error?: undefined;
    }
  | {
      /** It is a field of a form saved as a whole: the form's submit bar says what came of sending. */
      form: true;
      save?: undefined;
      /**
       * What is wrong with the value the form sent (one the server no longer allows), right under the row: the
       * control is marked and described by it, and its id, `<id>-control`, is what the screen moves the focus to.
       */
      error?: string | undefined;
    }
);

/**
 * A setting: its name over a line about it at the row's start, its control (a `Select`, a `NumberField`) at the
 * row's end, as wide as what it holds. In a narrow section (a phone) the line about it runs the row's width under the
 * name and the control, and where the two cannot share a line, the control fills a line of its own under the name, as
 * a field under its label (`SettingFrame`); a `wide` control (one that grows with what it holds: tags) has a line of
 * its own at every width, the row's whole width. Its control either saves at once, and the row takes its save state
 * (`save`, ADR-0110 §6), which stands in the place of the line about it, or, with no such line, with the name: under
 * it beside the control, in the room the row's height holds (the name, at the middle of the control until then, gives
 * way to it), and beside it when the name has a line to itself; the control stays where it is. Or it is a field of a
 * form saved as a whole (`form`), with no state of its own, and what is wrong with the value it sent (`error`) right
 * under the line about it, the control marked invalid and described by it. What the setting holds beyond its control's value
 * (`values`) stands close under the row. The row names its control (`id`, the control's `labelledBy`, which a
 * `Select` and a `NumberField` take from the row by themselves), describes it by the line about it and by the save
 * state, and a press on the name puts the focus on it.
 */
export function SettingRow({
  id,
  label,
  about,
  isDisabled = false,
  reason,
  values,
  wide = false,
  save,
  form,
  error,
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
  const errorId = `${id}-error`;
  const wrong = error !== undefined && error !== "";
  // The state's place, a live region that is there before the state is.
  const state = saves && (
    <span id={stateId} role="status">
      <SaveControlContext value={() => document.getElementById(controlId)?.focus()}>
        {saying && save}
      </SaveControlContext>
    </span>
  );
  const setting = (
    <SettingFrame
      className={s.root()}
      across={wide}
      name={
        <label id={id} htmlFor={controlId} className={isDisabled ? `${s.label()} ${s.off()}` : s.label()}>
          {label}
        </label>
      }
      state={below === undefined && state}
      under={
        (below !== undefined || wrong) && (
          <Stack gap="hair">
            {below !== undefined && (
              <Layers>
                <span id={aboutId} className={saying ? `${s.about()} ${s.covered()}` : s.about()}>
                  {below}
                </span>
                {state}
              </Layers>
            )}
            {wrong && <FieldError id={errorId}>{error}</FieldError>}
          </Stack>
        )
      }
      control={
        <FieldContext
          value={{
            id: controlId,
            labelledBy: id,
            describedBy: describedBy(below !== undefined && aboutId, wrong && errorId),
            stateBy: saying ? stateId : undefined,
            disabled: isDisabled || undefined,
            invalid: wrong || undefined,
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
    // Where the screen moves the focus to it, the ring a keyboard's focus shows.
    title: "rounded-control text-body font-strong outline-hidden [word-break:auto-phrase] focus-visible:shadow-focus",
    about: "text-caption text-muted-text [word-break:auto-phrase]",
  },
});

/**
 * The status summary that heads a screen while something there is to do (the security's, while two-factor
 * authentication is off): the state as its glyph and a sentence, what it means, and the next thing to do in a word or
 * two (a `Button`, the screen's primary action). It is the one place the screen says it and the one door to it: no
 * row under it repeats either. A state is words and a glyph, never a tile, which is what can be pressed. An action
 * that is done in place (it sends an email) says what came of it beside itself (`result`, a `SaveState`), in a polite
 * live region that is there from the start.
 * A view that takes another's place to say what came of it (an invite created) moves the focus to its title, which
 * `titleRef` reaches, so that a screen reader reads what happened first.
 */
export function StatusSummary({
  icon,
  title,
  titleRef,
  about,
  action,
  result,
}: {
  /** The state's glyph: `caution` while something is to do, `accepted` once all is well. */
  icon: IconName;
  /** The state, as a sentence ("Two-factor authentication is off"). */
  title: string;
  /** Reaches the title, which then can take the focus. */
  titleRef?: Ref<HTMLHeadingElement> | undefined;
  about: string;
  action?: ReactNode;
  /** What came of the action done in place; `null` while it has nothing to say. Only with an `action`. */
  result?: ReactNode | null;
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
          <Heading id={id} ref={titleRef} tabIndex={titleRef === undefined ? undefined : -1} className={s.title()}>
            {title}
          </Heading>
          <p className={s.about()}>{about}</p>
        </Stack>
      </Row>
      {action !== undefined && (
        <Row wrap align="center" justify="start" gap="screen-row-gap">
          <Row as="span" inline rigid>
            {action}
          </Row>
          {result !== undefined && <Box role="status">{result}</Box>}
        </Row>
      )}
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
    // A line about a stated fact is a sentence: it wraps between phrases, never inside a word of ja.
    about: "text-caption text-muted-text [word-break:auto-phrase]",
    // A row of several lines (a title over its facts): its title strong, as the menu's entries, and the rows further
    // apart than its own lines are, so that each reads as one.
    named: "font-label",
    spaced: "py-screen-row-gap",
    // What came of the reader's action on the row, in place of what it says of itself: in the text's color.
    result: "text-caption text-text outline-hidden [word-break:auto-phrase]",
    // What the reader may do with an account, at the middle of its avatar's height: level with the avatar and the
    // name, however many lines are under them.
    end: "min-h-avatar-post",
    // An account's line under its name, set in to the name's start.
    note: "ps-entry-name-start text-caption text-muted-text [word-break:auto-phrase]",
    // Where it stands: the one line of the row in the text's color under its title, which the reader comes back for.
    standing: "text-caption text-text [word-break:auto-phrase]",
    // A state at the row's end: words alone, muted, with no surface, which is a button's.
    state: "text-caption text-muted-text",
    chevron: "text-muted-text",
    empty: "text-muted-text [word-break:auto-phrase]",
    // The name over a list of facts that stands apart from the ones before it.
    lead: "font-label [word-break:auto-phrase]",
  },
  variants: {
    // What the row names does not work now: its title muted too, with no hue and no surface.
    inactive: { true: { title: "text-muted-text" } },
  },
});

/**
 * What a section lists (the passkeys, the signed-in browsers, the filters), named by `label`: its rows. As the
 * `values` of a `SettingRow` (the languages a setting holds), it is the setting's own: set in from its start, its
 * rows close to each other. With `id`, the list of a form (the words of a filter) takes the focus when what holds it
 * gives it (no key leads to it), and `describedBy` names what is wrong with its rows as a whole, an error at
 * `<id>-error` right under it, so that a refusal of them all is read with the list and comes into view
 * (`focusRefusedField`).
 */
export function SettingsList({
  label,
  id,
  describedBy,
  children,
}: {
  label: string;
  id?: string | undefined;
  describedBy?: string | undefined;
  children: ReactNode;
}) {
  const s = list();
  const values = use(ValuesContext);
  return (
    <Stack
      as="ul"
      gap={values ? "screen-air" : undefined}
      id={id}
      tabIndex={id === undefined ? undefined : -1}
      aria-label={label}
      aria-describedby={describedBy}
      className={values ? `${s.root()} ${s.values()}` : s.root()}
    >
      {children}
    </Stack>
  );
}

/** A line of a row's facts: one fact, or several joined by a middle dot, the line breaking between them first. */
export type SettingsFactLine = string | readonly string[];

/**
 * What came of the reader's action on a row (Unblocked), in place of what the row says of itself, its actions gone:
 * `shown`, its words, and `spoken`, the sentence assistive technology reads in their place (You unblocked @kai.); its
 * place takes the focus by `id`, where the button the reader pressed was.
 */
export type SettingsRowResult = { readonly id: string; readonly shown: string; readonly spoken: string };

/**
 * What came of an action of a row that leaves the row as it was (a download that started, or failed): its words, each
 * a new element of the row's status (`n`, which counts what the row said), so that the same words are read again.
 */
export type SettingsRowSaid = { readonly text: string; readonly n: number };

/** A row's result, a place the focus can be. */
function RowResult({ result }: { result: SettingsRowResult }) {
  return (
    <span id={result.id} tabIndex={-1} className={list().result()}>
      <span aria-hidden="true">{result.shown}</span>
      <span className="sr-only">{result.spoken}</span>
    </span>
  );
}

/** A line of facts as one string: several joined, each a chunk that breaks inside only past the line's width. */
function FactLine({ line }: { line: SettingsFactLine }) {
  const s = list();
  if (typeof line === "string") return <span className={s.fact()}>{line}</span>;
  return (
    <span className={s.fact()}>
      {/* The dot ends the fact before it, so that a line that breaks there never starts with it. */}
      {line.map((fact, i) => (
        <Fragment key={fact}>
          {i > 0 && " "}
          <Box as="span" display="inline-block">
            {i < line.length - 1 ? `${fact} ·` : fact}
          </Box>
        </Fragment>
      ))}
    </span>
  );
}

/**
 * A row of a section's list: its glyph, a title over a caption or over the facts known of it, a line each (`facts`:
 * a fact, or several joined by a middle dot that the line breaks between first), then where it stands, the one line
 * in the text's color (`standing`), a state in a word or two (`state`,
 * muted words with no surface: at the row's end, or on the title's line where the row has facts, so that every
 * row's facts keep one column), and what the reader may do with it (`end`: one or two buttons, a group named by the
 * row's title, so that one Remove is told from the next). With `result`, what came of the reader's action stands in
 * the caption's place and `end` is gone: the row keeps its place. With `said`, what came of an action that leaves the
 * row as it was (a download) is its last line, in the text's color, and `end` stays: the place is a polite live
 * region from the start (`null` while it has nothing to say), so that it is read where it happened, the focus staying
 * on the button. With `href`, the whole row opens the screen under the entry that holds it (changing the password,
 * one filter), a chevron at its end in place of buttons. `inactive`, a thing that does not work now (a revoked
 * invite): its title in the muted color, so that the ones that work stand out in a list of both.
 */
export function SettingsListRow({
  icon,
  title,
  inactive = false,
  caption,
  facts,
  standing,
  state,
  mark,
  href,
  end,
  result,
  said,
  itemRef,
}: {
  icon?: IconName | undefined;
  title: string;
  /** Whether what it names does not work now. */
  inactive?: boolean | undefined;
  caption?: string | undefined;
  /**
   * What is known of it, a line each (which browser this is, when it signed in, when it was last used): a line too
   * long for the row breaks between its facts, then between its words, never inside one, in ja too.
   */
  facts?: readonly SettingsFactLine[] | undefined;
  /**
   * Where it stands, a line after its facts in the text's own color: what the reader comes back to the row for, and
   * what changes while the rest stays (an appeal that waits, then what was decided of it).
   */
  standing?: string | undefined;
  state?: string | undefined;
  /**
   * Where a record stands, as the staff's and the reader's lists of records mark it (a `StateMark`: pending, approved),
   * in the place of `state` and in its own look: one look for a state on every list.
   */
  mark?: ReactNode;
  href?: string | undefined;
  end?: ReactNode;
  result?: SettingsRowResult | undefined;
  /** What came of an action that leaves the row as it was; `null` while it has nothing to say. */
  said?: SettingsRowSaid | null | undefined;
  /**
   * The ref of its focusable element, the link of a row with `href`, else the row itself: a list that grows under
   * Show more gives the first row a press added the focus (`useAddedFocus`).
   */
  itemRef?: RefCallback<HTMLElement> | undefined;
}) {
  const s = list({ inactive });
  const titleId = useId();
  const value = use(ValuesContext);
  const named = (
    <bdi id={titleId} className={facts === undefined ? s.title() : `${s.title()} ${s.named()}`}>
      {title}
    </bdi>
  );
  const stated = (state !== undefined || mark !== undefined) && (
    <>
      {" "}
      {mark ?? (
        <Box as="span" words className={s.state()}>
          {state}
        </Box>
      )}
    </>
  );
  // Its parts are a space apart in what assistive technology reads of the row, which no box of it shows.
  const parts = (
    <>
      {icon !== undefined && (
        <Row as="span" inline rigid className={s.glyph()}>
          <Icon name={icon} place="action" />
        </Row>
      )}
      <Stack as="span" grow>
        {facts === undefined || stated === false ? (
          named
        ) : (
          <Row as="span" wrap align="baseline" justify="between" gap="screen-tabs-gap">
            {named}
            {stated}
          </Row>
        )}
        {result !== undefined ? (
          <>
            {" "}
            <RowResult result={result} />
          </>
        ) : (
          caption !== undefined && (
            <>
              {" "}
              <span className={s.caption()} dir="auto">
                {caption}
              </span>
            </>
          )
        )}
        {facts?.map((line) => (
          <Fragment key={typeof line === "string" ? line : line.join(" · ")}>
            {" "}
            <FactLine line={line} />
          </Fragment>
        ))}
        {standing !== undefined && (
          <>
            {" "}
            <span className={s.standing()}>{standing}</span>
          </>
        )}
        {said !== undefined && (
          <>
            {" "}
            <span role="status" className={s.result()}>
              {said !== null && <span key={said.n}>{said.text}</span>}
            </span>
          </>
        )}
      </Stack>
      {facts === undefined && stated}
    </>
  );
  if (href !== undefined)
    return (
      <Stack as="li">
        <LinkRow
          as={
            <Link ref={itemRef} href={href} className={facts === undefined ? s.link() : `${s.link()} ${s.spaced()}`} />
          }
        >
          {parts}
          <Row as="span" inline rigid className={s.chevron()}>
            <Icon name="onward" place="action" />
          </Row>
        </LinkRow>
      </Stack>
    );
  return (
    <Row
      as={<li ref={itemRef} />}
      align="center"
      gap="screen-row-gap"
      className={value ? s.value() : facts === undefined ? s.row() : `${s.row()} ${s.spaced()}`}
    >
      {parts}
      {result === undefined && end !== undefined && end !== null && end !== false && (
        <Row as="span" inline rigid align="center" gap="screen-tabs-gap" role="group" aria-labelledby={titleId}>
          {end}
        </Row>
      )}
    </Row>
  );
}

/**
 * An account's row of a section's list: the account as another part draws it (`account`: its avatar, its name and
 * handle, its profile's link), what the reader may do with it at the end, level with the name (`end`, a group named
 * `label`), and the line under the name (`note`: what a mute covers), set in to the name's start. Where the end
 * cannot share the name's line it stands right under the account and its line, closer than the next account is. With `result`, what came
 * of the reader's action stands in the note's place and `end` is gone: the row keeps its place.
 */
export function SettingsAccountItem({
  account,
  label,
  note,
  end,
  result,
  itemRef,
}: {
  account: ReactNode;
  /** The account's name for assistive technology, which names the group of `end`. */
  label: string;
  note?: ReactNode;
  end?: ReactNode;
  result?: SettingsRowResult | undefined;
  /** The ref of the row, for a list that grows under Show more (`useAddedFocus`). */
  itemRef?: RefCallback<HTMLElement> | undefined;
}) {
  const s = list();
  return (
    // Wrapped, the end stands right under the account (its own box a little taller than the button), the next account
    // a row's air and more away.
    <Row
      as={<li ref={itemRef} />}
      wrap
      align="start"
      gapX="form-page-gap"
      gapY="0"
      className={`${s.row()} ${s.spaced()}`}
    >
      <Stack grow="names" gap="hair">
        {account}
        {result !== undefined ? (
          <p className={s.note()}>
            <RowResult result={result} />
          </p>
        ) : (
          note !== undefined && note !== null && note !== false && <div className={s.note()}>{note}</div>
        )}
      </Stack>
      {result === undefined && end !== undefined && end !== null && end !== false && (
        <Row
          as="span"
          inline
          rigid
          push="end"
          align="center"
          gap="screen-tabs-gap"
          role="group"
          aria-label={label}
          className={s.end()}
        >
          {end}
        </Row>
      )}
    </Row>
  );
}

/**
 * A row of a section's list whose value is a field (a word of a filter): the field (`field`, its label for assistive
 * technology alone) and what goes with it (`children`: a checkbox, then Remove) on one line in a wide frame; in a
 * narrow one the field fills its line, and they stand under it, the first at the start and the last at the end.
 */
export function SettingsFieldRow({ field, children }: { field: ReactNode; children: ReactNode }) {
  return (
    <li className={list().row()}>
      <FieldLineFrame field={field}>{children}</FieldLineFrame>
    </li>
  );
}

/**
 * The frame of a field and what goes with it, in a frame that knows its own width: on one line from 32 rem, the field
 * taking the room left; under it, the field filling its line, and what goes with it on the next, the first at its
 * start and the last at its end.
 */
function FieldLineFrame({ field, children }: { field: ReactNode; children: ReactNode }) {
  return (
    <div className="@container">
      <div className="flex flex-wrap items-center gap-x-screen-row-gap @min-[32rem]:flex-nowrap">
        <div className="min-w-0 grow basis-full @min-[32rem]:basis-0">{field}</div>
        <div className="flex grow items-center justify-between gap-x-screen-row-gap @min-[32rem]:grow-0">
          {children}
        </div>
      </div>
    </div>
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

/**
 * What a section states and the reader cannot press (what an app was granted, where it comes from), named by `label`:
 * each fact a sentence over the muted lines about it, where it has some, the facts one under another at one distance
 * whether or not they have such lines. A list of facts, not of rows: nothing in it is a finger's target tall. `shown`:
 * its name stands over it, in a label's weight, for facts told from the ones before them by what they are (what an
 * import removes, after what it adds).
 */
export function SettingsFacts({
  label,
  shown = false,
  items,
}: {
  label: string;
  shown?: boolean | undefined;
  items: readonly { text: string; about?: readonly string[] | undefined }[];
}) {
  const s = list();
  const id = useId();
  const facts = (
    <Stack
      as="ul"
      gap="screen-row-gap"
      {...(shown ? { "aria-labelledby": id } : { "aria-label": label })}
      className={s.root()}
    >
      {items.map((item) => (
        <Stack as="li" key={item.text}>
          <bdi className={s.title()}>{item.text}</bdi>
          {item.about?.map((line) => (
            <Fragment key={line}>
              {" "}
              <span className={s.about()}>{line}</span>
            </Fragment>
          ))}
        </Stack>
      ))}
    </Stack>
  );
  if (!shown) return facts;
  return (
    <Stack gap="screen-air">
      <p id={id} className={s.lead()}>
        {label}
      </p>
      {facts}
    </Stack>
  );
}

const note = tv({
  slots: {
    root: "[word-break:auto-phrase]",
    link: "whitespace-nowrap",
    // The values to copy, in the code's face, on the raised surface, and on the page's in a raised section; a long one
    // breaks anywhere.
    values: [
      "list-none rounded-control bg-surface-raised p-screen-row-inline font-mono text-body text-text select-all",
      "[[data-surface-raised]_&]:bg-surface",
    ],
    value: "wrap-anywhere tabular-nums",
    // A block of code (a payload), whole: its lines and indents kept, a long line wrapped where code-text.tsx breaks it and
    // never scrolled sideways, at the caption's size.
    code: [
      "box-border rounded-control bg-surface-raised p-screen-row-inline",
      "font-mono text-caption text-text whitespace-pre-wrap",
      "[[data-surface-raised]_&]:bg-surface",
    ],
    result: "text-caption text-muted-text",
    // What a section says came of a change: where the screen moves the focus to it, the ring a keyboard's focus shows.
    said: "rounded-control outline-hidden focus-visible:shadow-focus",
    // A QR code is dark on light whatever the scheme, or a camera does not read it.
    qr: "rounded-control bg-surface p-screen-air text-text [color-scheme:light]",
    modules: "size-screen-qr fill-current",
  },
  variants: {
    instead: {
      // In an action's place: the text's own size and color, a row's gap more before it than the description has.
      true: { root: "pt-screen-row-gap text-body text-text" },
      false: { root: "text-caption text-muted-text" },
    },
  },
  defaultVariants: { instead: false },
});

/**
 * A sentence in a section that is no setting and no state of a control: what a list's rows mean for the reader, what
 * an action there will do, why one cannot be done; then the links to where they act on it (the password's screen, the
 * apps), each a link of the text. Muted, at the caption's size. `instead`, the sentence that stands in the place of
 * an action the reader cannot take now and says why (no Create invite at the cap): in the text's size and color, set
 * apart from the section's description, still words and no tile.
 */
export function SettingsNote({
  links,
  instead = false,
  children,
}: {
  /** Where the reader acts on what the sentence says, after it. */
  links?: readonly { href: string; label: string }[] | undefined;
  /** Whether it says why an action of the section is not there. */
  instead?: boolean | undefined;
  children: string;
}) {
  return (
    <p className={note({ instead }).root()}>
      {children}
      {/* The first link meets the sentence as an action does; each next one stands a middle dot after the one before. */}
      {links?.map((link, i) => (
        <Fragment key={link.href}>
          {i === 0 ? actionJoint(children) : actionJoint(link.label)}
          {/* A link's few words hold together: the line wraps before it, never inside it. */}
          <span className={note().link()}>
            <TextLink href={link.href}>{link.label}</TextLink>
          </span>
        </Fragment>
      ))}
    </p>
  );
}

/**
 * What came of something the reader did in a section, in a line of it (a `SaveState`: a passkey removed, a browser
 * signed out), read as it comes (`role="status"`). Given a `ref`, it takes the focus that the control which did it
 * left behind when that control went with what it changed (the row's own button): the screen moves the focus there
 * once the confirmation over the section has left, so that a keyboard stays in the section and a screen reader reads
 * what happened.
 */
export function SettingsResult({
  ref,
  children,
}: {
  ref?: Ref<HTMLParagraphElement> | undefined;
  children: ReactNode;
}) {
  return (
    <p ref={ref} role="status" tabIndex={ref === undefined ? undefined : -1} className={note().said()}>
      {children}
    </p>
  );
}

/**
 * What the reader copies and keeps (recovery codes, the key of an authenticator app, a secret shown once), named by
 * `label`: the values in the code's face on the raised surface (the page's in a raised section), each on a line of
 * its own, two side by side where they fit, and under them the action that copies them all (`action`, a `Button`)
 * with what came of it right after (`result`, read as it comes). A press on the values selects them all, for a reader
 * who copies by hand. A block of `code` in their place (a webhook's payload, the errors of an import) stands whole,
 * keeps its lines and indents, and wraps a long line under its own indent.
 */
export function CopyBlock({
  label,
  action,
  result,
  ...shown
}: {
  label: string;
  action: ReactNode;
  /** What came of copying, in a word or two (Copied). */
  result?: string | undefined;
} & (
  | { values: readonly string[]; code?: undefined }
  | {
      /** A block of code or a payload, as its lines are to be copied. */
      code: string;
      values?: undefined;
    }
)) {
  const s = note();
  return (
    <Stack gap="screen-row-gap">
      {shown.code === undefined ? (
        <Grid
          as="ul"
          columns={shown.values.length > 1 ? 2 : 1}
          gap="screen-air"
          gapX="shell-edge"
          aria-label={label}
          className={s.values()}
        >
          {shown.values.map((value) => (
            <li key={value} className={s.value()}>
              {value}
            </li>
          ))}
        </Grid>
      ) : (
        <pre role="group" aria-label={label} className={s.code()} dir="ltr">
          <code>
            <CodeLines code={shown.code} />
          </code>
        </pre>
      )}
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
              className={s.modules()}
            />
          }
        >
          <path d={path} />
        </Box>
      </Box>
    </Row>
  );
}

const bar = tv({
  slots: {
    // What sending does, read toward the button it stands before: its lines end where the button starts.
    about: "text-end [word-break:auto-phrase]",
  },
});

/**
 * The submit bar of a form saved as a whole: its button (`children`, the screen's primary action) at the end, and
 * what came of sending right before it, in the same place every time (`result`: a `SaveState` once saved, a
 * `FormAlert` when refused). It ends the screen and stays at the bottom of the viewport while the form is longer,
 * the page keeping what takes the focus clear of it. The result's place is a polite live region from the start, so
 * that assistive technology hears what came of sending (a `FormAlert` at once, as an alert).
 *
 * `about` is what sending does as the form stands, where that cannot be undone (how many posts a save deletes): one
 * short sentence beside the button, on its line at every width (a sentence too long for the room wraps beside the
 * button, which never drops under it), so that the reader decides with it in view. It comes and goes as the form
 * changes, often between the press of the button and its release (a field that tells its value as it loses the
 * focus): the button keeps its element and its place, at the foot of the bar, which grows upward. The button names
 * the sentence as its description (the `id` given here, in the button's `aria-describedby`); it is no live region,
 * since what it says is read where the form counts it.
 */
export function SubmitBar({
  result,
  about,
  children,
}: {
  result?: ReactNode;
  about?: { readonly id: string; readonly text: string } | undefined;
  children: ReactNode;
}) {
  const said = result !== undefined && result !== null && result !== false;
  return (
    <FrameFoot>
      {/* With nothing to say, the result's place is no line of its own over a sentence and a button that fill the bar. */}
      <Row wrap align="center" justify="end" gap="screen-row-gap" gapY={said ? undefined : "0"}>
        <Box role="status">{result}</Box>
        <Row align="center" justify="end" gap="screen-row-gap">
          {about !== undefined && (
            <p id={about.id} className={bar().about()}>
              {about.text}
            </p>
          )}
          <Row as="span" inline rigid self="end">
            {children}
          </Row>
        </Row>
      </Row>
    </FrameFoot>
  );
}
