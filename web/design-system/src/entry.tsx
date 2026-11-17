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
import { useReader } from "./locale";
import { Preview, usePreviewState } from "./preview";
import { actionJoint } from "./sentence";
import { ActionScope } from "./shortcuts";
import { RelativeTime } from "./time";

const entry = tv({
  base: [
    "box-border grid grid-cols-[minmax(0,1fr)] gap-entry rounded-entry text-body text-text outline-hidden",
    "-m-entry-reach p-entry-reach scroll-mt-feed-scroll scroll-mb-entry-scroll",
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
      className={entry()}
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
    >
      <ActionScope target={ref}>
        <Preview state={undefined}>{children}</Preview>
      </ActionScope>
    </article>
  );
}

const stack = tv({
  base: "grid grid-cols-[minmax(0,1fr)]",
  variants: { gap: { part: "gap-entry", line: "gap-entry-line" } },
});

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
    <div className={stack({ gap })} lang={lang}>
      {children}
    </div>
  );
}

const caption = tv({
  slots: {
    root: "m-0 flex items-start gap-entry-caption text-caption text-muted-text",
    // The glyph stands on the first line, so that a caption that wraps keeps it at its start.
    glyph: "inline-flex h-[1lh] shrink-0 items-center",
    // Wrapping between phrases, never inside a word of ja (表示されます); a word longer than the line (a handle of
    // another server, @user@domain) breaks where it must rather than push the line past its column.
    text: "min-w-0 flex-1 [word-break:auto-phrase] [overflow-wrap:anywhere]",
    action: "whitespace-nowrap",
    // An action of its own at the line's end, on the first line, never broken inside.
    end: "shrink-0 whitespace-nowrap",
    // The glyph's place while what the line says is under way: a spinner of the glyph's size.
    spinner: [
      "box-border size-icon-caption rounded-full border-spinner border-solid",
      "border-spinner-track border-t-current animate-spin",
      motion.spin.reduced === "still" ? "motion-reduce:animate-none" : "",
    ],
  },
  variants: {
    // On one line: a `CaptionName` in it truncates, and the words around it keep their room and their spaces.
    oneLine: { true: { text: "flex whitespace-pre" } },
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
 * that takes it, where a feed's `onEntryClick` hears only a press no control took. It may hear one press twice. A
 * shortcut's key is no press: what it runs tells its owner itself.
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
      {children}
    </div>
  );
}

/**
 * A name inside a one-line caption (who boosted a post): isolated (<bdi>), and the part that truncates first, so the
 * line keeps the words around it, the verb at its end.
 */
export function CaptionName({ children }: { children: ReactNode }) {
  return <bdi className="min-w-0 truncate">{children}</bdi>;
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
  return (
    <p className={s.root()} lang={lang}>
      <span className={s.glyph()}>
        {busy ? <span className={s.spinner()} aria-hidden="true" /> : <Icon name={icon} place="caption" />}
      </span>
      <span className={s.text()}>
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
      </span>
      {end !== undefined && <span className={s.end()}>{end}</span>}
    </p>
  );
}

const byline = tv({
  slots: {
    root: "flex min-w-0 items-center",
    lines: "min-w-0 flex-1",
    line: "flex min-w-0 items-baseline gap-entry-name leading-entry",
    name: "min-w-0 shrink truncate font-strong",
    // A name that opens its account: in the text's color, underlined only while hovered.
    link: [
      "min-w-0 shrink truncate rounded-control text-current no-underline outline-hidden",
      "data-hovered:underline data-focus-visible:shadow-focus",
    ],
    end: "ms-auto flex flex-none",
    // The handle's line, in the page's direction, so that the handle sits at the line's start in either; the handle
    // in it reads left-to-right and, as wide as it is up to the line, truncates at its own end.
    handleLine: "block min-w-0 text-caption leading-entry text-muted-text",
    handle: "inline-block max-w-full truncate align-top",
    // The name as the heading of a screen of the account's things: the name's own look, in a heading's place.
    heading: "m-0 flex min-w-0 shrink [font:inherit]",
  },
  variants: {
    // The author over the handle beside the post's avatar, or all on one line beside an inline one (a quoted post).
    compact: {
      false: { root: "gap-entry-avatar", lines: "grid" },
      true: { root: "gap-entry-caption", lines: "flex items-baseline gap-entry-caption", handleLine: "flex-1" },
    },
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
  const s = byline({ compact });
  const own = useId();
  const bdi = (
    <bdi id={nameId ?? own} className={s.name()}>
      {name}
    </bdi>
  );
  const linked =
    href === undefined ? (
      bdi
    ) : (
      <Link href={href} className={s.link()}>
        {bdi}
      </Link>
    );
  const nameEl = heading ? <h2 className={s.heading()}>{linked}</h2> : linked;
  const handleEl =
    self === undefined ? (
      <span className={s.handleLine()}>
        <bdi dir="ltr" className={s.handle()}>
          {handle}
        </bdi>
      </span>
    ) : (
      <span className={s.handleLine()}>
        <span className={s.handle()}>{self}</span>
        <span className="sr-only"> {handle}</span>
      </span>
    );
  const endEl = end !== undefined && <span className={s.end()}>{end}</span>;
  return (
    <div className={s.root()}>
      {avatar}
      {compact ? (
        <div className={s.lines()}>
          {nameEl}
          {handleEl}
          {endEl}
        </div>
      ) : (
        <div className={s.lines()}>
          <div className={s.line()}>
            {nameEl}
            {endEl}
          </div>
          {handleEl}
        </div>
      )}
    </div>
  );
}

const stamp = tv({
  slots: {
    root: "flex flex-none items-center gap-entry-caption text-caption text-muted-text",
    glyph: "inline-flex flex-none",
    link: [
      "rounded-control text-current no-underline outline-hidden",
      "data-hovered:underline data-focus-visible:shadow-focus",
    ],
    // The time and the edited mark together: on a touch screen, one target of the touch height over both.
    when: "relative inline-flex items-baseline",
    // The edited mark: a word after the time, muted as the time is, apart from it by a middle dot; it opens the
    // history. On a touch screen its hit area covers the time too, at the touch height.
    edited: [
      "inline-flex cursor-pointer items-baseline rounded-control border-0 bg-transparent p-0 font-sans text-caption",
      "text-current outline-hidden data-hovered:underline data-focus-visible:shadow-focus",
      "pointer-coarse:before:absolute pointer-coarse:before:inset-x-0 pointer-coarse:before:top-1/2",
      "pointer-coarse:before:h-icon-button-touch pointer-coarse:before:-translate-y-1/2",
    ],
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
    <span className={s.root()}>
      {glyph !== undefined && (
        <span role="img" aria-label={glyph.label} className={s.glyph()}>
          <Icon name={glyph.icon} place="caption" />
        </span>
      )}
      {edited === undefined ? (
        linked
      ) : (
        <span className={s.when()}>
          {linked}
          <span aria-hidden="true">&nbsp;·&nbsp;</span>
          <AriaButton aria-label={edited.label} className={s.edited()} onPress={edited.onPress}>
            {edited.text}
          </AriaButton>
        </span>
      )}
    </span>
  );
}

const actions = tv({
  slots: {
    // The row reaches as far past the body as a tile's inline padding, so that the first glyph lines up with the body.
    root: "-mx-icon-button-inline flex gap-entry-action",
    end: "ms-auto inline-flex",
  },
});

/** A row of icon buttons close together at the start, and `end` (more) at the row's end. */
export function ActionRow({
  lang,
  children,
  end,
}: {
  lang?: string | undefined;
  children: ReactNode;
  end?: ReactNode;
}) {
  const s = actions();
  return (
    <div className={s.root()} lang={lang}>
      {children}
      {end !== undefined && <span className={s.end()}>{end}</span>}
    </div>
  );
}

const embed = tv({
  base: "grid grid-cols-[minmax(0,1fr)] gap-embed-gap rounded-control bg-surface-raised p-embed",
});

/** Something embedded in an entry, on the raised surface: a quoted post, which never outweighs the post that carries it. */
export function Embed({ lang, children }: { lang?: string | undefined; children: ReactNode }) {
  return (
    <div className={embed()} lang={lang}>
      {children}
    </div>
  );
}
