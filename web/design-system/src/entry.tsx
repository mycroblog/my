// SPDX-License-Identifier: AGPL-3.0-only

// The parts an entry of a feed (a post) is laid out with (ADR-0025 §4), so that @my/app-components composes a post
// without a class of its own. The entry is one column at the full width, its parts a gap apart, with no structural
// empty column such as an avatar gutter. At rest it has no edge and marks nothing but its space; hovered, or focused
// from the keyboard, it shows its extent with a surface that reaches past its content (so the content never moves),
// and keyboard focus adds the ring. The byline is the avatar, then the name with what ends its line (the visibility
// and the time) over the handle; a name and a handle are isolated with <bdi>, and the handle reads left-to-right and
// truncates at its end in either direction. Every part lays out in the reader's direction (locale.ts).

import { motion } from "@my/design-tokens";
import { useId, useRef, useState, type ReactNode } from "react";
import { Button as AriaButton, Link } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { markedSurface } from "./button";
import { useFeedPosition } from "./feed";
import { Icon, type IconName } from "./icons";
import { Box, Row, Stack as Rows } from "./layout";
import { useReader } from "./locale";
import { Preview, usePreviewState } from "./preview";
import { actionJoint } from "./sentence";
import { ActionScope, HeardShortcuts } from "./shortcuts";
import { RelativeTime } from "./time";

const entry = tv({
  base: [
    "box-border rounded-entry text-body text-text outline-hidden",
    "p-entry-reach scroll-mt-feed-scroll scroll-mb-entry-scroll",
    "data-surface-marked:bg-surface-hover data-focus-visible:shadow-focus",
  ],
});

export type EntryProps = {
  /** The language of what the entry holds (a post's), when it is known. */
  lang?: string | undefined;
  /** The id of what names the entry (its author's name), for assistive technology. */
  labelledBy?: string | undefined;
  children: ReactNode;
};

/**
 * An entry of a feed: a focusable article whose parts stand one under another. It marks its surface while hovered or
 * focused from the keyboard (the icon buttons inside step their hover a tone down), and it carries `data-entry`, by
 * which a feed moves the focus between its entries; inside a FeedItem it states its position in the feed and carries
 * the item's key as `data-scroll-key`, by which a screen restores its position. It is the `ActionScope` of the focus
 * actions its parts declare (a post's reply), which act while focus is inside it. A story's preview of hover or
 * focus applies to the entry alone, not to the buttons inside it.
 */
export function Entry({ lang, labelledBy, children }: EntryProps) {
  const position = useFeedPosition();
  const preview = usePreviewState();
  const { direction } = useReader(undefined);
  const [hovered, setHovered] = useState(false);
  const [focused, setFocused] = useState(false);
  const ref = useRef<HTMLElement>(null);
  const focus = focused || preview === "focus-visible";
  const marked = hovered || focus || preview === "hover";
  return (
    <Rows
      as={
        <article
          ref={ref}
          lang={lang}
          dir={direction}
          tabIndex={0}
          aria-labelledby={labelledBy}
          aria-posinset={position?.posinset}
          aria-setsize={position?.setsize}
          data-entry=""
          data-scroll-key={position?.key}
          data-focus-visible={focus || undefined}
          {...markedSurface(marked)}
          onPointerEnter={() => {
            setHovered(true);
          }}
          onPointerLeave={() => {
            setHovered(false);
          }}
          onFocus={(e) => {
            setFocused(e.target === e.currentTarget && e.currentTarget.matches(":focus-visible"));
          }}
          onBlur={() => {
            setFocused(false);
          }}
        />
      }
      gap="entry"
      bleedInline="entry-reach"
      bleedBlock="entry-reach"
      className={entry()}
    >
      <ActionScope target={ref}>
        <Preview state={undefined}>{children}</Preview>
      </ActionScope>
    </Rows>
  );
}

/** Parts one under another at the full width: an entry's gap apart (`part`), or a line's (`line`). */
export function Stack({
  gap = "part",
  lang,
  children,
}: {
  gap?: "part" | "line";
  lang?: string | undefined;
  children: ReactNode;
}) {
  return (
    <Rows gap={gap === "part" ? "entry" : "entry-line"} lang={lang}>
      {children}
    </Rows>
  );
}

const caption = tv({
  slots: {
    root: "text-caption text-muted-text",
    // The glyph stands on the first line, so that a caption that wraps keeps it at its start.
    glyph: "",
    // Wrapping between phrases, never inside a word of ja (表示されます); a word longer than the line (a handle of
    // another server, @user@domain) breaks where it must rather than push the line past its column.
    text: "[word-break:auto-phrase] [overflow-wrap:anywhere]",
    action: "whitespace-nowrap",
    // An action of its own at the line's end, on the first line, never broken inside.
    end: "whitespace-nowrap",
    // The glyph's place while what the line says is under way: a spinner of the glyph's size.
    spinner: [
      "box-border size-icon-caption rounded-full border-spinner border-solid",
      "border-spinner-track border-t-current animate-spin",
      motion.spin.reduced === "still" ? "motion-reduce:animate-none" : "",
    ],
  },
  variants: {
    // On one line: a `CaptionName` in it truncates, and the words around it keep their room and their spaces.
    oneLine: { true: { text: "whitespace-pre" } },
    // What went wrong with what the reader asked for: the text's own color over the muted caption, without a hue.
    alert: { true: { glyph: "text-text", text: "text-text" } },
    // What the reader did that waits for someone else (a reply until its author approves it): as plain as `alert`,
    // the text's own color without a hue, since it is no failure.
    waiting: { true: { glyph: "text-text", text: "text-text" } },
  },
});

/**
 * A part of an entry that assistive technology hears change (a translation's line, from Translate to its outcome): a
 * polite live region with no box of its own, which stays while what is in it changes.
 */
export function LivePart({ children }: { children: ReactNode }) {
  return (
    <div aria-live="polite" className="contents">
      {children}
    </div>
  );
}

// What a press of Enter or Space acts on. React Aria runs such a press of its controls itself, with no click.
const pressed = 'a[href], button, summary, [role="button"], [role="link"], [role^="menuitem"]';
const pressedBySpace = 'button, summary, [role="button"], [role^="menuitem"]';

/** Whether a press on `on` is one on the entry of `around`: see `HeardActions`. */
function acts(around: Element, on: Element): boolean {
  // A press in what the entry opened reaches it through React's tree, from outside its own elements.
  return around.contains(on) || on.closest('[role^="menuitem"]:not([aria-disabled="true"])') !== null;
}

/**
 * Around an entry, with no box of its own: hears every press of the reader on it (a notification is read once the
 * reader has shown that they saw it), by a pointer or by Enter or Space, on its words or on a control of it, the one
 * that opens a menu or a picker included, and on an item of a menu it opened. It hears the press before the control
 * that takes it, where a feed's `onEntryClick` hears only a press no control took. It hears as well each action
 * declared inside it that ran on its shortcut's key (`HeardShortcuts`): a post's reply, or more, which opens its menu;
 * not one declared around it (the feed's next entry). It may hear one press twice.
 */
export function HeardActions({ onAct, children }: { onAct: () => void; children: ReactNode }) {
  return (
    <div
      className="contents"
      onClickCapture={(e) => {
        if (acts(e.currentTarget, e.target as Element)) onAct();
      }}
      onKeyDownCapture={(e) => {
        const on = e.target as Element;
        const by = e.key === "Enter" ? pressed : e.key === " " ? pressedBySpace : undefined;
        if (by !== undefined && !e.repeat && on.matches(by) && acts(e.currentTarget, on)) onAct();
      }}
    >
      <HeardShortcuts onAct={onAct}>{children}</HeardShortcuts>
    </div>
  );
}

/**
 * A name inside a one-line caption (who boosted a post): isolated (<bdi>), and the part that truncates first, so the
 * line keeps the words around it, the verb at its end.
 */
export function CaptionName({ children }: { children: ReactNode }) {
  return (
    <Box as="bdi" shrink lines={1}>
      {children}
    </Box>
  );
}

export type CaptionLineProps = {
  icon: IconName;
  /** The language of its words, when it differs from the entry's. */
  lang?: string | undefined;
  /** The text. */
  children?: ReactNode;
  /**
   * An action after the text (a `TextButton`), apart from it by a middle dot that stays at the end of the text's line,
   * the action never broken inside.
   */
  action?: ReactNode;
  /**
   * An action at the line's end, apart from the text by space alone, which wraps under itself beside it (Change time);
   * in place of `action`.
   */
  end?: ReactNode;
  /** One line that never wraps: a `CaptionName` in it truncates instead. */
  oneLine?: boolean | undefined;
  /** What it says is under way (a translation): a spinner stands in the glyph's place. */
  busy?: boolean | undefined;
  /** It says what went wrong with what the reader asked for: its text in the text's color. */
  alert?: boolean | undefined;
  /** It says that what the reader did waits for someone else (an approval): its text in the text's color. */
  waiting?: boolean | undefined;
  /**
   * What assistive technology reads in place of the text, where the text changes more often than it should be heard (a
   * download's progress, heard by quarters).
   */
  spoken?: string | undefined;
};

/**
 * A caption line: a muted glyph and its text, which wraps between words under itself (who boosted a post, a
 * translation's note), and an optional action at its end.
 */
export function CaptionLine({
  icon,
  lang,
  children,
  action,
  end,
  oneLine = false,
  busy = false,
  alert = false,
  waiting = false,
  spoken,
}: CaptionLineProps) {
  const s = caption({ oneLine, alert, waiting });
  const hasText = children !== undefined && children !== null && children !== false;
  // On one line, the words around a name stand beside it, the name the one to give way.
  const Text = oneLine ? Row : Box;
  return (
    <Row as="p" align="start" gap="entry-caption" className={s.root()} lang={lang}>
      <Row as="span" line rigid align="center" className={s.glyph()}>
        {busy ? <span className={s.spinner()} aria-hidden="true" /> : <Icon name={icon} place="caption" />}
      </Row>
      <Text as="span" grow className={s.text()}>
        {spoken === undefined ? (
          children
        ) : (
          <>
            <span aria-hidden="true">{children}</span>
            <span className="sr-only">{spoken}</span>
          </>
        )}
        {/* A dot held to the last word after words; after a full sentence, none (sentence.ts). */}
        {hasText && action !== undefined && actionJoint(children)}
        {action !== undefined && <span className={s.action()}>{action}</span>}
      </Text>
      {end !== undefined && (
        <Box as="span" rigid className={s.end()}>
          {end}
        </Box>
      )}
    </Row>
  );
}

const byline = tv({
  slots: {
    line: "leading-entry",
    name: "font-strong",
    // A name that opens its account: in the text's color, underlined only while hovered.
    link: [
      "rounded-control text-current no-underline outline-hidden",
      "data-hovered:underline data-focus-visible:shadow-focus",
    ],
    // The handle's line, in the page's direction, so that the handle sits at the line's start in either; the handle
    // in it reads left-to-right and, as wide as it is up to the line, truncates at its own end.
    handleLine: "text-caption leading-entry text-muted-text",
    // The name as the heading of a screen of the account's things: the name's own look, in a heading's place.
    heading: "[font:inherit]",
  },
});

export type BylineProps = {
  /** The avatar, at the head of the byline. */
  avatar: ReactNode;
  /** The display name; it truncates before what ends its line gives up any room. */
  name: ReactNode;
  /** The id of the name, for what the name labels (the entry). */
  nameId?: string | undefined;
  /** The handle, as `@user@domain`. */
  handle: string;
  /** What ends the name's line: the visibility and the time (a `Stamp`). */
  end?: ReactNode;
  /** All on one line, beside an inline avatar. */
  compact?: boolean | undefined;
  /** Where the name leads: the account's profile. */
  href?: string | undefined;
  /**
   * The reader wrote it (in a conversation of direct messages): what says so ("You"), muted in the handle's place,
   * which the reader knows; the handle is then heard after it.
   */
  self?: string | undefined;
  /** The name is the heading of the screen it heads (an `h2`), as it shows. */
  heading?: boolean | undefined;
};

/** Who wrote an entry: the avatar, the name and what ends its line, and the handle. */
export function Byline({
  avatar,
  name,
  nameId,
  handle,
  end,
  compact = false,
  href,
  self,
  heading = false,
}: BylineProps) {
  const s = byline();
  const own = useId();
  const bdi = (
    <Box as="bdi" id={nameId ?? own} lines={1} className={s.name()}>
      {name}
    </Box>
  );
  const linked =
    href === undefined ? (
      bdi
    ) : (
      <Box as={<Link href={href} />} lines={1} className={s.link()}>
        {bdi}
      </Box>
    );
  const nameEl = heading ? (
    <Row as="h2" shrink className={s.heading()}>
      {linked}
    </Row>
  ) : (
    linked
  );
  const handleEl =
    self === undefined ? (
      <Box as="span" display="block" grow={compact} className={s.handleLine()}>
        <Box as="bdi" dir="ltr" display="inline-block" max="full" lines={1} baseline="top">
          {handle}
        </Box>
      </Box>
    ) : (
      <Box as="span" display="block" grow={compact} className={s.handleLine()}>
        <Box as="span" display="inline-block" max="full" lines={1} baseline="top">
          {self}
        </Box>
        <span className="sr-only"> {handle}</span>
      </Box>
    );
  const endEl = end !== undefined && (
    <Row as="span" push="end" rigid>
      {end}
    </Row>
  );
  return (
    <Row shrink align="center" gap={compact ? "entry-caption" : "entry-avatar"}>
      {avatar}
      {compact ? (
        <Row grow align="baseline" gap="entry-caption">
          {nameEl}
          {handleEl}
          {endEl}
        </Row>
      ) : (
        <Rows grow>
          <Row shrink align="baseline" gap="entry-name" className={s.line()}>
            {nameEl}
            {endEl}
          </Row>
          {handleEl}
        </Rows>
      )}
    </Row>
  );
}

const stamp = tv({
  slots: {
    root: "text-caption text-muted-text",
    link: [
      "rounded-control text-current no-underline outline-hidden",
      "data-hovered:underline data-focus-visible:shadow-focus",
    ],
    // The edited mark: a word after the time, muted as the time is, apart from it by a middle dot; it opens the
    // history. On a touch screen its hit area covers the time too, at the touch height.
    edited:
      "cursor-pointer rounded-control border-0 bg-transparent p-0 font-sans text-caption text-current outline-hidden data-hovered:underline data-focus-visible:shadow-focus",
  },
});

export type StampProps = {
  /** The glyph before the time (the visibility), by its meaning, and what it alone says for assistive technology. */
  glyph?: { icon: IconName; label: string } | undefined;
  /** The moment, as a Date or an ISO 8601 string. */
  date: Date | string;
  /** Where the time leads: the entry on a page of its own. */
  href?: string | undefined;
  /**
   * The entry was edited: `text` (edited) after the time, named `label` for assistive technology, which opens its
   * history; on a touch screen a press on the time opens it too.
   */
  edited?: { text: string; label: string; onPress: () => void } | undefined;
};

/**
 * A glyph and the relative time, kept together at the end of a byline's line. The time is isolated (<bdi>), so that
 * it keeps its own order beside the glyph in either direction, with words of an RTL locale reading right-to-left.
 */
export function Stamp({ glyph, date, href, edited }: StampProps) {
  const s = stamp();
  const time = (
    <bdi>
      <RelativeTime date={date} />
    </bdi>
  );
  const linked =
    href === undefined ? (
      time
    ) : (
      <Link href={href} className={s.link()}>
        {time}
      </Link>
    );
  return (
    <Row as="span" rigid align="center" gap="entry-caption" className={s.root()}>
      {glyph !== undefined && (
        <Row as="span" rigid role="img" aria-label={glyph.label}>
          <Icon name={glyph.icon} place="caption" />
        </Row>
      )}
      {edited === undefined ? (
        linked
      ) : (
        // The time and the edited mark together: on a touch screen, one target of the touch height over both.
        <Row as="span" anchor align="baseline">
          {linked}
          <span aria-hidden="true">&nbsp;·&nbsp;</span>
          <Row
            as={<AriaButton aria-label={edited.label} onPress={edited.onPress} />}
            align="baseline"
            touch
            className={s.edited()}
          >
            {edited.text}
          </Row>
        </Row>
      )}
    </Row>
  );
}

/**
 * A row of icon buttons close together at the start, and `end` (more) at the row's end; the buttons that do not fit (large
 * counts, as a right-to-left locale writes them) go on a next line.
 */
export function ActionRow({
  lang,
  children,
  end,
}: {
  lang?: string | undefined;
  children: ReactNode;
  end?: ReactNode;
}) {
  // The row reaches as far past the body as a tile's inline padding, so that the first glyph lines up with the body.
  return (
    <Row wrap bleedInline="icon-button-inline" gap="entry-action" lang={lang}>
      {children}
      {end !== undefined && (
        <Row as="span" push="end" rigid>
          {end}
        </Row>
      )}
    </Row>
  );
}

const embed = tv({
  base: "rounded-control bg-surface-raised p-embed",
  variants: { clear: { true: "[&>:first-child]:pe-composer-remove-room" } },
});

/**
 * Something embedded in an entry, on the raised surface: a quoted post, which never outweighs the post that carries
 * it. `clear` keeps the end of its first line clear of a button pinned over its top end (its remove).
 */
export function Embed({
  lang,
  clear = false,
  children,
}: {
  lang?: string | undefined;
  clear?: boolean;
  children: ReactNode;
}) {
  return (
    <Rows gap="embed-gap" className={embed({ clear })} lang={lang}>
      {children}
    </Rows>
  );
}
