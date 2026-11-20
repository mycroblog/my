// SPDX-License-Identifier: AGPL-3.0-only

// The parts of a screen over and among the entries of its feed (the notifications, a profile's posts, a thread): the
// screen's parts stacked a feed entry's room apart, the tabs over a list, a heading of a group of a list, a fold of
// what the list does not show, the unread mark, the line of a notice's kind, an excerpt, an account on a row of its
// own, rows of one kind as a list, one kind of what a screen found under its heading, what a field found, a refusal
// beside its confirmation, and the row that opens what waits elsewhere. Everything sits on the page's one surface,
// apart by space; a raised surface marks only a control (the current tab) and what a field found.

import { createContext, use, useId, type ReactElement, type ReactNode, type Ref } from "react";
import { Button, Link, Tab, TabList, TabPanel, Tabs } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { Byline, Stamp } from "./entry";
import { Icon, type IconName } from "./icons";
import { Box, Row, Stack, type Gap } from "./layout";
import { useReader } from "./locale";

/**
 * A screen's parts one under another (a head, the tabs, a row, the feed), apart by the room a feed entry's ring needs;
 * `busy` while what it holds loads, for assistive technology.
 */
export function ScreenStack({ busy = false, children }: { busy?: boolean | undefined; children: ReactNode }) {
  return (
    <Stack gap="entry-room" aria-busy={busy || undefined}>
      {children}
    </Stack>
  );
}

const tabs = tv({
  slots: {
    // A tab: muted text on the page; the current one on the raised surface, in the text's color, as a tonal control.
    tab: [
      "box-border h-control-timeline cursor-pointer rounded-control px-control-timeline-inline",
      "text-body font-label whitespace-nowrap text-muted-text outline-hidden",
      "data-hovered:bg-surface-hover data-hovered:text-text data-focus-visible:shadow-focus",
      "data-selected:bg-surface-raised data-selected:text-text",
    ],
    panel: "outline-hidden",
    // How many things wait under a tab, as the navigation's and the menu's badges draw it.
    badge: [
      "box-border h-shell-badge min-w-shell-badge rounded-[calc(var(--shell-badge)/2)]",
      "bg-text px-shell-badge-inline text-[0.6875rem] leading-none font-label text-surface tabular-nums",
    ],
  },
});

export type TabRowProps<T extends string> = {
  /** What the tabs choose between, for assistive technology. */
  label: string;
  /**
   * The tabs: each its name, and how many things wait under it (a badge, as the navigation's), with what the count is
   * for assistive technology ("3 pending"); none or zero shows no badge.
   */
  tabs: readonly { id: T; name: string; count?: number | undefined; countLabel?: string | undefined }[];
  /** The current tab. */
  current: T;
  onChange: (id: T) => void;
  /** What sits at the row's end, after the tabs (icon buttons). */
  end?: ReactNode;
  /** What the current tab shows. */
  children: ReactNode;
};

/** The badge of a tab's count, in the reader's numbers; nothing for none or zero. */
function TabCount({
  count,
  label,
  className,
}: {
  count: number | undefined;
  label: string | undefined;
  className: string;
}) {
  const { lang } = useReader(undefined);
  if (count === undefined || count <= 0) return null;
  return (
    <>
      <Row
        as="span"
        inline
        rigid
        align="center"
        justify="center"
        className={className}
        aria-hidden={label === undefined ? undefined : "true"}
      >
        {new Intl.NumberFormat(lang).format(count)}
      </Row>
      {label !== undefined && <span className="sr-only">{label}</span>}
    </>
  );
}

/** Tabs over a list, with what ends their row, and the list of the current tab under them. */
export function TabRow<T extends string>({ label, tabs: items, current, onChange, end, children }: TabRowProps<T>) {
  const s = tabs();
  const { direction } = useReader(undefined);
  return (
    <Stack
      gap="entry-room"
      as={
        <Tabs
          selectedKey={current}
          onSelectionChange={(key) => {
            const tab = items.find((t) => t.id === key);
            if (tab) onChange(tab.id);
          }}
        />
      }
    >
      <Row align="center" gap="screen-tabs-gap" dir={direction}>
        {/* The first tab's text on the column's edge, as the heading and the back link above it, its surface reaching
            out; the tabs that do not fit scroll sideways. */}
        <Row
          grow
          gap="screen-tab-gap"
          bleedStart="control-timeline-inline"
          scroll="x"
          clip="y"
          as={<TabList aria-label={label} />}
        >
          {items.map((tab) => (
            <Row key={tab.id} rigid align="center" gap="screen-tab-gap" as={<Tab id={tab.id} className={s.tab()} />}>
              {tab.name}
              <TabCount count={tab.count} label={tab.countLabel} className={s.badge()} />
            </Row>
          ))}
        </Row>
        {end !== undefined && (
          <Row as="span" inline rigid push="end">
            {end}
          </Row>
        )}
      </Row>
      {items.map((tab) => (
        <TabPanel key={tab.id} id={tab.id} className={s.panel()}>
          {children}
        </TabPanel>
      ))}
    </Stack>
  );
}

const group = tv({ base: "text-caption font-normal text-muted-text" });

/**
 * Where the heading of a group stands: in a feed, which owns its articles alone, it is in the first entry of the group
 * it heads, above the post, and keeps the group's space above it there.
 */
function GroupPlace({ id, className, children }: { id?: string | undefined; className: string; children: string }) {
  return (
    <h2 id={id} className={`${className} [[data-entry]>&]:mt-screen-group-gap`}>
      {children}
    </h2>
  );
}

/** A heading of a group of a list (the replies of a post), muted and small, as the column beside the main one heads its sections. */
export function GroupHeading({ children }: { children: string }) {
  return <GroupPlace className={group()}>{children}</GroupPlace>;
}

const fold = tv({ base: "text-caption" });

/** A fold of what a list does not show yet: a `TextButton` that shows it, at the start of the column. */
export function Fold({ children }: { children: ReactNode }) {
  return <p className={fold()}>{children}</p>;
}

const unread = tv({ base: "size-screen-unread rounded-full bg-text" });

/** The mark of something unread: a dot in the text's color, a shape with no hue, and its label for assistive technology. */
export function UnreadMark({ label }: { label: string }) {
  return (
    <Box as="span" display="inline-block" rigid self="center" role="img" aria-label={label} className={unread()} />
  );
}

const notice = tv({
  slots: {
    // At the icon's width whatever it holds (a reaction's emoji too), so that every sentence starts in one column.
    glyph: "w-icon-action text-muted-text text-[length:var(--icon-action)] leading-none",
    // Held empty where no account acted (a notice of the server), so that every sentence starts in one column.
    avatar: "w-avatar-inline",
    // A sentence that wraps breaks between phrases, never inside a word of ja (求めています, お気に入り), its lines
    // kept even.
    text: "[text-wrap:pretty] [word-break:auto-phrase]",
    // The sentence as the link to what it tells of, in the line's own color, underlined while hovered.
    link: [
      "rounded-control text-current no-underline outline-hidden data-hovered:underline",
      "data-hovered:underline-offset-[0.15em] data-focus-visible:shadow-focus",
    ],
  },
  variants: {
    // Unread, the line is in the text's color with the mark at its end; read, it is muted.
    read: { true: { text: "text-muted-text" } },
  },
});

export type NoticeLineProps = {
  /** The glyph of its kind. */
  icon: IconName;
  /** An emoji in place of the glyph (a reaction's), as text or an image. */
  emoji?: ReactNode;
  /** The account's avatar, inline. */
  avatar?: ReactNode;
  /** When it happened. */
  date: Date | string;
  /** The id of the text, which names the entry. */
  id?: string | undefined;
  /** The unread mark's label while it is unread; undefined once read. */
  unread?: string | undefined;
  /** The sentence of its kind. */
  children: ReactNode;
  /** What it is about, under the sentence in its column (an `Excerpt` of the reader's post). */
  excerpt?: ReactNode;
  /** A control at the line's end, after the time (dismissing it). */
  action?: ReactNode;
  /** Where it leads: what it tells of (the post, the account, the follow requests). */
  href?: string | undefined;
  /** What runs as its link is followed (it is read). */
  onFollow?: (() => void) | undefined;
};

/**
 * The line of a notice's kind: its glyph, the account's avatar, the sentence, which wraps, the unread mark and the time
 * at its end, and what it is about under the sentence. With `href`, the whole line leads to what it tells of.
 */
export function NoticeLine({
  icon,
  emoji,
  avatar,
  date,
  id,
  unread: mark,
  children,
  excerpt,
  action,
  href,
  onFollow,
}: NoticeLineProps) {
  const s = notice({ read: mark === undefined });
  return (
    // The glyph, the avatar's place and the body, which holds the sentence and what it is about under it. Its parts
    // stand close, so that a short sentence keeps to one line beside the time at 320 px. The glyph and the avatar
    // stand on the text's first line, so that a sentence that wraps keeps them at its start.
    <Row anchor align="start" gap="entry-caption">
      <Row as="span" inline line rigid align="center" justify="center" className={s.glyph()}>
        {emoji ?? <Icon name={icon} place="action" />}
      </Row>
      <Row as="span" inline line rigid align="center" className={s.avatar()}>
        {avatar}
      </Row>
      <Stack grow gap="entry-line">
        <p className={s.text()}>
          {/* The unread mark, the time and the control stand close, so that the first line keeps more of its measure:
              they float at its end, and the lines under it run to the column's end. */}
          <Row as="span" inline line align="center" gap="chip-gap" float="end" spaceBefore="entry-caption">
            {mark !== undefined && <UnreadMark label={mark} />}
            <Stamp date={date} />
            {action !== undefined && (
              // Over the line's link, at a timeline control's height on the first line's.
              <Row as="span" inline anchor layer={1} bleedBlock="control-timeline-line" bleedEnd="icon-button-inline">
                {action}
              </Row>
            )}
          </Row>
          {/* The sentence alone names the entry, not the time before it. */}
          <span id={id}>
            {href === undefined ? (
              children
            ) : (
              // The link reaches over the whole line, so that a press anywhere on it follows it.
              <Box
                reach
                as={
                  <Link href={href} className={s.link()} {...(onFollow === undefined ? {} : { onPress: onFollow })} />
                }
              >
                {children}
              </Box>
            )}
          </span>
        </p>
        {excerpt}
      </Stack>
    </Row>
  );
}

const noticeCaption = tv({
  slots: {
    root: "text-caption",
    text: "[word-break:auto-phrase]",
    glyph: "text-muted-text",
  },
  variants: { read: { true: { text: "text-muted-text" }, false: { text: "text-text" } } },
});

/**
 * A notice's kind over the entry it is about, inside it (a post that mentions the reader): a caption's glyph and
 * sentence, in the text's color with the unread mark at the line's end while unread, muted once read, and a control
 * after the mark.
 */
export function NoticeCaption({
  icon,
  unread: mark,
  action,
  children,
}: {
  icon: IconName;
  unread?: string | undefined;
  /** A control at the line's end (dismissing it). */
  action?: ReactNode;
  children: ReactNode;
}) {
  const s = noticeCaption({ read: mark === undefined });
  return (
    <Row align="center" gap="entry-avatar" className={s.root()}>
      <Row as="p" grow align="start" gap="entry-caption" className={s.text()}>
        <Row as="span" inline line rigid align="center" className={s.glyph()}>
          <Icon name={icon} place="caption" />
        </Row>
        <span>{children}</span>
      </Row>
      {mark !== undefined && <UnreadMark label={mark} />}
      {action !== undefined && (
        // Its whole height in the line, so that it keeps clear of the post's byline under it.
        <Row as="span" inline rigid bleedEnd="icon-button-inline">
          {action}
        </Row>
      )}
    </Row>
  );
}

const name = tv({ base: "font-strong" });

/** A name inside a sentence: strong and isolated. */
export function SentenceName({ children }: { children: ReactNode }) {
  return <bdi className={name()}>{children}</bdi>;
}

// Wrapping between phrases, never inside a word of ja, as the sentence over it does.
const excerpt = tv({
  base: "[&_p+p]:before:content-['_']",
  variants: {
    // A post's body (`ContentBody`) runs on in the excerpt's tone, not in the text colour of its own.
    body: { true: "[&_div]:text-[inherit]", false: "[word-break:auto-phrase]" },
  },
});

type ExcerptProps = {
  lang?: string | undefined;
  /** How many lines show before it ends in an ellipsis. */
  lines?: 1 | 2 | 3 | undefined;
  /** Its colour: muted, or the text's own for what is unread. */
  tone?: "muted" | "text" | undefined;
  /** What it holds is a post's body (`ContentBody`), whose blocks all run on as one text, in the excerpt's tone. */
  body?: boolean | undefined;
  children: ReactNode;
};

/**
 * The first words of something a line is about (the reader's post a notice is about): its paragraphs run on, on two
 * lines at most, muted unless its tone says the text's own.
 */
export function Excerpt({ lang, lines = 2, tone = "muted", body = false, children }: ExcerptProps) {
  return (
    <Box lines={lines} run={body ? "all" : true} tone={tone} className={excerpt({ body })} lang={lang}>
      {children}
    </Box>
  );
}

const state = tv({
  base: "h-control-timeline text-caption text-muted-text",
  // One that takes the focus of the button it replaces keeps that button's box (a timeline button's inline padding),
  // so that its ring has room around the text and the row keeps its footprint.
  variants: {
    focusable: { true: "rounded-control px-control-timeline-inline outline-hidden focus-visible:shadow-focus" },
  },
});

/**
 * What the reader did, or what is so, in place of the button that did it (a request accepted, an account followed).
 * Given a `ref`, it takes the focus the pressed button leaves (the screen moves it there), so that a screen reader
 * reads what the press did and the reader goes on from where they were.
 */
export function StateText({ children, ref }: { children: string; ref?: Ref<HTMLSpanElement> | undefined }) {
  const focusable = ref !== undefined;
  return (
    <Row
      inline
      align="center"
      as={<span ref={ref} className={state({ focusable })} tabIndex={focusable ? -1 : undefined} />}
    >
      {children}
    </Row>
  );
}

/** A refusal beside its confirmation, the confirmation at the end; or one action alone at the end. */
export function ButtonPair({ children }: { children: ReactNode }) {
  return (
    <Row rigid align="center" justify="end" gap="screen-tabs-gap">
      {children}
    </Row>
  );
}

const found = tv({ base: "rounded-overlay bg-surface-raised p-screen-found-padding" });

/**
 * What a field found, under it, on the raised surface in the rhythm of the rows (the accounts a list's members may
 * take), apart from the rows below; `label` names the group.
 */
export function FoundRows({ label, children }: { label: string; children: ReactNode }) {
  return (
    <Stack gap="entry-room" role="group" aria-label={label} className={found()}>
      {children}
    </Stack>
  );
}

const rows = tv({ base: "list-none p-0" });

/** Rows of one kind (a list's members) as a list, named by `label`, in the rhythm of the rows: each a `RowItem`. */
export function RowList({ label, children }: { label: string; children: ReactNode }) {
  return (
    <Stack as="ul" gap="entry-room" aria-label={label} className={rows()}>
      {children}
    </Stack>
  );
}

/**
 * Where one kind of what a screen found stands: room above its caption, which sits tight on its rows, with the same
 * caption-to-row space for every kind of row, a row that opens (an `OpenRow`, padded for its hover) drawn up by its
 * padding.
 */
function ResultPlace({ labelledBy, children }: { labelledBy: string; children: ReactNode }) {
  return (
    <Stack
      as="section"
      gap="screen-air"
      spaceAbove="screen-group-gap"
      aria-labelledby={labelledBy}
      className="[&>ul>li:first-child>a]:-mt-screen-row-block"
    >
      {children}
    </Stack>
  );
}

/**
 * One kind of what a screen found (the people of a search), a region named by its heading, muted and small as a
 * list's group's: its rows, or the line that stands in their place, under it.
 */
export function ResultGroup({ title, children }: { title: string; children: ReactNode }) {
  const id = useId();
  return (
    <ResultPlace labelledBy={id}>
      <GroupPlace id={id} className={group()}>
        {title}
      </GroupPlace>
      {children}
    </ResultPlace>
  );
}

/**
 * A row of a `RowList` (an `AccountRow`, an `OpenRow`); `itemRef` the ref of the row, for a list that grows under Show
 * more (`useAddedFocus`).
 */
export function RowItem({ itemRef, children }: { itemRef?: Ref<HTMLLIElement> | undefined; children: ReactNode }) {
  return <li ref={itemRef}>{children}</li>;
}

// A line under the byline at the name's start (how many notifications an account's request holds).
const account = tv({ slots: { note: "ps-entry-name-start text-caption text-muted-text" } });

/**
 * An account on a row of its own: its avatar, its name over its handle, a note under them, and what the reader may do
 * at the end.
 */
export function AccountRow({
  avatar,
  name: accountName,
  handle,
  href,
  note,
  heading = false,
  end,
  itemRef,
}: {
  avatar: ReactNode;
  name: ReactNode;
  handle: string;
  /** Where the name leads: the account's profile, or what is of it elsewhere (its held-back notifications). */
  href?: string | undefined;
  /** A line under the name and the handle. */
  note?: ReactNode;
  /** The name is the heading of the screen the row heads (`ScreenHead`). */
  heading?: boolean | undefined;
  end?: ReactNode;
  /** The ref of the row, for a list that grows under Show more (`useAddedFocus`). */
  itemRef?: Ref<HTMLDivElement> | undefined;
}) {
  const s = account();
  return (
    // What the reader may do goes under the name, at the end, where beside it the name and the handle would keep less
    // than their least width (a 320 px row with two buttons), so that the handle stays readable.
    <Row as={<div ref={itemRef} />} wrap align="center" gap="screen-row-gap">
      <Stack grow="names" gap="entry-line">
        <Byline avatar={avatar} name={accountName} handle={handle} href={href} heading={heading} />
        {note !== undefined && <p className={s.note()}>{note}</p>}
      </Stack>
      {end !== undefined && (
        <Box push="end">
          <ButtonPair>{end}</ButtonPair>
        </Box>
      )}
    </Row>
  );
}

const row = tv({
  slots: {
    root: [
      "rounded-control px-screen-row-inline py-screen-row-block",
      "text-text no-underline outline-hidden data-hovered:bg-surface-hover data-focus-visible:shadow-focus",
    ],
    title: "text-body font-label",
    hint: "text-caption text-muted-text",
    chevron: "text-muted-text",
  },
});

/**
 * A row that is one link, as `as`: its parts in its middle, a row's gap apart; its surface reaches past the column by
 * its inline padding, so that what it holds lines up with the column's edges.
 */
function LinkRow({ as, children }: { as: ReactElement<{ className?: unknown }>; children: ReactNode }) {
  return (
    <Row as={as} align="center" gap="screen-row-gap" bleedInline="screen-row-inline">
      {children}
    </Row>
  );
}

/** A row that opens what waits elsewhere (the requests the notification policy holds back): its glyph, a title over a hint. */
export function OpenRow({
  href,
  icon,
  title,
  hint,
}: {
  href: string;
  icon: IconName;
  title: string;
  hint?: string | undefined;
}) {
  const s = row();
  return (
    <LinkRow as={<Link href={href} className={s.root()} />}>
      <Row as="span" inline rigid>
        <Icon name={icon} place="action" />
      </Row>
      <Stack as="span" grow>
        <span className={s.title()}>{title}</span>
        {hint !== undefined && <span className={s.hint()}>{hint}</span>}
      </Stack>
      <Row as="span" inline rigid className={s.chevron()}>
        <Icon name="onward" place="action" />
      </Row>
    </LinkRow>
  );
}

const faces = tv({
  slots: {
    // The whole row is the link: the surface on hover and the ring on focus, so its chevron reads as part of it. The
    // ring that cuts a face out takes the surface under it.
    root: [
      "rounded-control px-screen-row-inline py-screen-row-block",
      "text-text no-underline outline-hidden [--face-cut-color:var(--color-surface)]",
      "data-hovered:bg-surface-hover data-hovered:[--face-cut-color:var(--color-surface-hover)]",
      "data-focus-visible:shadow-focus",
    ],
    lead: "min-w-screen-faces-lead text-body",
    glyph: "text-[length:var(--chip-glyph)] leading-none text-muted-text",
    count: "tabular-nums",
    // The strip is as tall as one face, and its padding keeps the faces' cut rings inside it.
    strip: "box-content h-avatar-post p-screen-face-cut",
    face: "rounded-full shadow-face-cut",
    chevron: "text-muted-text",
  },
});

export type FacesRowProps = {
  /** Where it leads: the list of every account of its kind. */
  href: string;
  /** What it says to assistive technology, and on hover: how many, and of what kind ("6 reacted with :sparkle:"). */
  label: string;
  /** The kind's glyph: an icon, or an emoji as text or an image. */
  glyph: ReactNode;
  /** How many, as the reader's locale writes it. */
  count: string;
  /** The first accounts' avatars, at a post's size, as many as the server gave; those that do not fit are left out. */
  faces: readonly ReactNode[];
};

/**
 * A row of one kind of what was done to a post (a favourite, a boost, a reaction with one emoji): its glyph and count,
 * the first accounts' faces overlapping, each cut out of the one before, and a chevron; the whole row leads to the
 * list of every account of that kind.
 */
export function FacesRow({ href, label, glyph, count, faces: items }: FacesRowProps) {
  const s = faces();
  const { lang } = useReader(undefined);
  return (
    // React Aria passes no title through: the link says its label on hover on the element itself.
    <LinkRow
      as={
        <Link
          href={href}
          className={s.root()}
          aria-label={label}
          render={(props: object) => <a {...props} title={label} />}
        />
      }
    >
      <Row as="span" inline rigid align="center" gap="chip-gap" className={s.lead()} aria-hidden="true">
        <Row as="span" inline className={s.glyph()}>
          {glyph}
        </Row>{" "}
        <bdi dir="ltr" lang={lang} className={s.count()}>
          {count}
        </bdi>
      </Row>
      {/* As many faces as fit: a face that does not wraps to a second line, which the strip's height hides, so that
          faces drop from the end whole, never shrink or scroll. */}
      <Row as="span" wrap grow clip overlap="screen-face-overlap" className={s.strip()} aria-hidden="true">
        {items.map((face, i) => (
          <Row key={i} as="span" inline rigid className={s.face()}>
            {face}
          </Row>
        ))}
      </Row>
      <Row as="span" inline rigid className={s.chevron()} aria-hidden="true">
        <Icon name="onward" place="action" />
      </Row>
    </LinkRow>
  );
}

const title = tv({
  slots: {
    root: "text-title font-normal",
    glyph: "text-muted-text",
    count: "text-body text-muted-text tabular-nums",
  },
});

/**
 * The title of a list of a screen of its own under its back button (who favourited a post): a glyph, the words, which
 * may draw an emoji, and how many, muted.
 */
export function ListTitle({ glyph, count, children }: { glyph?: ReactNode; count?: string; children: ReactNode }) {
  const s = title();
  return (
    <Row as="h2" wrap align="center" gapX="entry-caption" className={s.root()}>
      {glyph !== undefined && (
        <Row as="span" inline className={s.glyph()}>
          {glyph}
        </Row>
      )}
      <span>{children}</span>
      {count !== undefined && <span className={s.count()}>{count}</span>}
    </Row>
  );
}

/**
 * The head of a screen of one account's things (its held-back notifications): its back button close over the account's
 * row, whose name is the screen's heading (an `AccountRow` with `heading`), as a screen's title stands under it.
 */
export function ScreenHead({ back, children }: { back: ReactNode; children: ReactNode }) {
  return (
    // The back button close over the row, so that it reads as part of the head.
    <Stack gap="screen-name-gap">
      {back}
      {children}
    </Stack>
  );
}

const screenTitle = tv({
  slots: {
    // One run of text, so that a long name wraps under its glyph and the count follows its last line.
    title: "text-title font-normal [overflow-wrap:anywhere]",
    // One the screen moves the focus to shows the ring a keyboard's focus shows, and no outline of the browser's.
    focusable: "rounded-control outline-hidden focus-visible:shadow-focus",
    glyph: "text-muted-text",
    count: "text-body whitespace-nowrap text-muted-text tabular-nums",
  },
});

/**
 * A name with a chance to break before each capital or run of digits inside it (#SlowMorningsByTheSea), so that a long
 * hashtag wraps on whole words; its text is unchanged for copying and for assistive technology.
 */
function breakable(name: string): ReactNode {
  const parts = name.split(/(?<=\p{Ll})(?=[\p{Lu}\p{Nd}])|(?<=\p{Nd})(?=\p{L})/u);
  if (parts.length === 1) return name;
  return parts.flatMap((part, i) => (i === 0 ? [part] : [<wbr key={i} />, part]));
}

export type ScreenTitleProps = {
  /** The glyph of what the screen lists (a hashtag, a list). */
  glyph?: IconName | undefined;
  /** How many, as the reader's locale writes it with its word ("3 members"), muted after the name. */
  count?: string | undefined;
  /** What the reader may do with it (follow a hashtag); left out when there is nothing to do. */
  actions?: ReactNode;
  /** The back button over the title (`BackButton`). */
  back?: ReactNode;
  /** The name, as its author wrote it. */
  children: string;
  /**
   * Reaches the title, which can then take the focus: a view that takes another's place under the same address moves
   * it there, as a change of screen moves it to the screen's heading.
   */
  ref?: Ref<HTMLHeadingElement> | undefined;
};

/**
 * The head of a screen of one thing a timeline is of (a hashtag, a list): its back button, then one row of its glyph
 * and name, how many, muted, at the start, and what the reader may do with it at the end.
 */
export function ScreenTitle({ glyph, count, actions, back, children, ref }: ScreenTitleProps) {
  const s = screenTitle();
  return (
    // The back button close over the row, so that it reads as part of the head.
    <Stack gap="screen-name-gap">
      {back}
      {/* The title at the start, what the reader may do at the end, on the baseline of the title's first line. */}
      <Row align="baseline" gap="screen-head-room">
        <Box
          grow
          as={
            ref === undefined ? (
              <h2 className={s.title()} />
            ) : (
              <h2 ref={ref} tabIndex={-1} className={`${s.title()} ${s.focusable()}`} />
            )
          }
        >
          {glyph !== undefined && (
            <Row as="span" inline baseline="glyph" spaceAfter="entry-caption" className={s.glyph()}>
              <Icon name={glyph} place="action" />
            </Row>
          )}
          <bdi>{breakable(children)}</bdi>
          {/* A space apart from the name, read so too: "Bread and coffee 2 members", never "coffee2 members". */}
          {count !== undefined && (
            <>
              {" "}
              <span className={s.count()}>{count}</span>
            </>
          )}
        </Box>
        {actions !== undefined && actions !== false && actions !== null && (
          <Row rigid align="center" gap="screen-tab-gap">
            {actions}
          </Row>
        )}
      </Row>
    </Stack>
  );
}

const back = tv({
  slots: {
    root: "",
    button: [
      "cursor-pointer rounded-control border-0 bg-transparent p-0",
      "outline-hidden [font-family:inherit] data-focus-visible:shadow-focus",
    ],
  },
  variants: {
    lead: {
      // The reader's one way back: as the text reads, a finger's target tall.
      true: { root: "text-body", button: "min-h-icon-button-touch text-body text-text" },
      false: { root: "text-caption", button: "text-caption text-muted-text data-hovered:text-text" },
    },
  },
});

/**
 * Whether the way back a screen is headed by is the reader's only one (a screen of the settings, whose menu is not
 * beside it): a `BackButton` under it reads as the text does and is a finger's target tall.
 */
export const BackLead = createContext(false);

/**
 * Back to where the reader came from, muted and small, at the start of the screen; where it is the reader's only way
 * back (`BackLead`), in the text's size and color, a finger's target tall.
 */
export function BackButton({ onPress, children }: { onPress: () => void; children: string }) {
  const lead = use(BackLead);
  const s = back({ lead });
  return (
    <p className={s.root()}>
      <Row inline align="center" gap="entry-caption" as={<Button className={s.button()} onPress={onPress} />}>
        <Icon name="back" place={lead ? "action" : "caption"} />
        {children}
      </Row>
    </p>
  );
}

const press = tv({
  base: [
    "cursor-pointer rounded-control border-0 bg-transparent px-screen-row-inline py-screen-row-block text-start",
    "font-sans outline-hidden data-hovered:bg-surface-hover data-pressed:bg-surface-pressed",
    "data-focus-visible:shadow-focus",
  ],
});

/**
 * A row of a list that is pressed as a whole: a surface on hover and press, the ring on focus, its parts a `gap` apart,
 * one under another (`stack`) or side by side from the top (`row`). Its surface reaches past the column by its inline
 * padding, so that what it holds lines up with the column's edge: at its start (`start`, beside something that stands
 * at its end), or at both (`inline`). `grow` takes the room left in a row. Its text is the caption's or the body's,
 * the text's color or muted.
 */
export function PressRow({
  onPress,
  flow = "stack",
  gap,
  bleed,
  grow = false,
  text = "body",
  tone = "text",
  children,
}: {
  onPress?: (() => void) | undefined;
  flow?: "stack" | "row";
  gap: Gap;
  bleed: "start" | "inline";
  grow?: boolean;
  text?: "caption" | "body";
  tone?: "text" | "muted";
  children: ReactNode;
}) {
  const as = <Button className={press()} {...(onPress ? { onPress } : {})} />;
  const place = {
    as,
    grow,
    text,
    tone,
    bleedStart: bleed === "start" ? ("screen-row-inline" as const) : undefined,
    bleedInline: bleed === "inline" ? ("screen-row-inline" as const) : undefined,
  };
  return flow === "row" ? (
    <Row {...place} align="start" gap={gap}>
      {children}
    </Row>
  ) : (
    <Stack {...place} gap={gap}>
      {children}
    </Stack>
  );
}
