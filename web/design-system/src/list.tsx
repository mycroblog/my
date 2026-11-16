// SPDX-License-Identifier: AGPL-3.0-only

// The parts of a screen over and among the entries of its feed (the notifications, a profile's posts, a thread): the
// screen's parts stacked a feed entry's room apart, the tabs over a list, a heading of a group of a list, a fold of
// what the list does not show, the unread mark, the line of a notice's kind, an excerpt, an account on a row of its
// own, rows of one kind as a list, one kind of what a screen found under its heading, what a field found, a refusal
// beside its confirmation, and the row that opens what waits elsewhere. Everything sits on the page's one surface,
// apart by space; a raised surface marks only a control (the current tab) and what a field found.

import { useId, type ReactNode, type Ref } from "react";
import { Button, Link, Tab, TabList, TabPanel, Tabs } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { Byline, Stamp } from "./entry";
import { Icon, type IconName } from "./icons";
import { useReader } from "./locale";

const screen = tv({ base: "grid grid-cols-[minmax(0,1fr)] gap-entry-room" });

/**
 * A screen's parts one under another (a head, the tabs, a row, the feed), apart by the room a feed entry's ring needs;
 * `busy` while what it holds loads, for assistive technology.
 */
export function ScreenStack({ busy = false, children }: { busy?: boolean | undefined; children: ReactNode }) {
  return (
    <div className={screen()} aria-busy={busy || undefined}>
      {children}
    </div>
  );
}

const tabs = tv({
  slots: {
    root: "grid grid-cols-[minmax(0,1fr)] gap-entry-room",
    row: "flex items-center gap-screen-tabs-gap",
    // Its first tab's text on the column's edge, as the heading and the back link above it, its surface reaching out.
    list: "-ms-control-timeline-inline flex min-w-0 flex-1 gap-screen-tab-gap overflow-x-auto overflow-y-hidden",
    // A tab: muted text on the page; the current one on the raised surface, in the text's color, as a tonal control.
    tab: [
      "box-border flex h-control-timeline cursor-pointer items-center rounded-control px-control-timeline-inline",
      "text-body font-label whitespace-nowrap text-muted-text outline-hidden",
      "data-hovered:bg-surface-hover data-hovered:text-text data-focus-visible:shadow-focus",
      "data-selected:bg-surface-raised data-selected:text-text",
    ],
    end: "ms-auto inline-flex flex-none",
    panel: "outline-hidden",
  },
});

export type TabRowProps<T extends string> = {
  /** What the tabs choose between, for assistive technology. */
  label: string;
  tabs: readonly { id: T; name: string }[];
  /** The current tab. */
  current: T;
  onChange: (id: T) => void;
  /** What sits at the row's end, after the tabs (icon buttons). */
  end?: ReactNode;
  /** What the current tab shows. */
  children: ReactNode;
};

/** Tabs over a list, with what ends their row, and the list of the current tab under them. */
export function TabRow<T extends string>({ label, tabs: items, current, onChange, end, children }: TabRowProps<T>) {
  const s = tabs();
  const { direction } = useReader(undefined);
  return (
    <Tabs
      className={s.root()}
      selectedKey={current}
      onSelectionChange={(key) => {
        const tab = items.find((t) => t.id === key);
        if (tab) onChange(tab.id);
      }}
    >
      <div className={s.row()} dir={direction}>
        <TabList aria-label={label} className={s.list()}>
          {items.map((tab) => (
            <Tab key={tab.id} id={tab.id} className={s.tab()}>
              {tab.name}
            </Tab>
          ))}
        </TabList>
        {end !== undefined && <span className={s.end()}>{end}</span>}
      </div>
      {items.map((tab) => (
        <TabPanel key={tab.id} id={tab.id} className={s.panel()}>
          {children}
        </TabPanel>
      ))}
    </Tabs>
  );
}

const group = tv({
  base: [
    "m-0 text-caption font-normal text-muted-text",
    // In a feed, which owns its articles alone, it stands in the first entry of the group it heads, above the post,
    // with the group's space above it.
    "[[data-entry]>&]:mt-screen-group-gap",
  ],
});

/** A heading of a group of a list (the replies of a post), muted and small, as the column beside the main one heads its sections. */
export function GroupHeading({ children }: { children: string }) {
  return <h2 className={group()}>{children}</h2>;
}

const fold = tv({ base: "m-0 text-caption" });

/** A fold of what a list does not show yet: a `TextButton` that shows it, at the start of the column. */
export function Fold({ children }: { children: ReactNode }) {
  return <p className={fold()}>{children}</p>;
}

const unread = tv({ base: "inline-block size-screen-unread flex-none self-center rounded-full bg-text" });

/** The mark of something unread: a dot in the text's color, a shape with no hue, and its label for assistive technology. */
export function UnreadMark({ label }: { label: string }) {
  return <span role="img" aria-label={label} className={unread()} />;
}

const notice = tv({
  slots: {
    // The glyph, the avatar's place and the body, which holds the sentence and what it is about under it. The link, when
    // there is one, reaches over the whole line from the sentence. Its parts stand close, so that a short sentence
    // keeps to one line beside the time at 320 px.
    root: "relative flex items-start gap-entry-caption",
    // The glyph and the avatar stand on the text's first line, so that a sentence that wraps keeps them at its start.
    glyph: [
      "inline-flex h-[1lh] min-w-icon-action flex-none items-center justify-center text-muted-text",
      "text-[length:var(--icon-action)] leading-none",
    ],
    // Held empty where no account acted (a notice of the server), so that every sentence starts in one column.
    avatar: "inline-flex h-[1lh] w-avatar-inline flex-none items-center",
    body: "grid min-w-0 flex-1 gap-entry-line",
    // A sentence that wraps breaks between phrases, never inside a word of ja (求めています, お気に入り), its lines
    // kept even; the time floats at its first line's end, and the lines under it run to the column's end.
    text: "m-0 [text-wrap:pretty] [word-break:auto-phrase]",
    end: "float-end ms-entry-caption inline-flex h-[1lh] items-center gap-entry-caption",
    // The sentence as the link to what it tells of, in the line's own color, underlined while hovered; it reaches over
    // the whole line, so that a press anywhere on it follows the link.
    link: [
      "rounded-control text-current no-underline outline-hidden data-hovered:underline",
      "data-hovered:underline-offset-[0.15em] data-focus-visible:shadow-focus",
      "after:absolute after:inset-0 after:content-['']",
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
  href,
  onFollow,
}: NoticeLineProps) {
  const s = notice({ read: mark === undefined });
  return (
    <div className={s.root()}>
      <span className={s.glyph()}>{emoji ?? <Icon name={icon} place="action" />}</span>
      <span className={s.avatar()}>{avatar}</span>
      <div className={s.body()}>
        <p className={s.text()}>
          <span className={s.end()}>
            {mark !== undefined && <UnreadMark label={mark} />}
            <Stamp date={date} />
          </span>
          {/* The sentence alone names the entry, not the time before it. */}
          <span id={id}>
            {href === undefined ? (
              children
            ) : (
              <Link href={href} className={s.link()} {...(onFollow === undefined ? {} : { onPress: onFollow })}>
                {children}
              </Link>
            )}
          </span>
        </p>
        {excerpt}
      </div>
    </div>
  );
}

const noticeCaption = tv({
  slots: {
    root: "flex items-center gap-entry-avatar text-caption",
    text: "m-0 flex min-w-0 flex-1 items-start gap-entry-caption [word-break:auto-phrase]",
    glyph: "inline-flex h-[1lh] flex-none items-center text-muted-text",
  },
  variants: { read: { true: { text: "text-muted-text" }, false: { text: "text-text" } } },
});

/**
 * A notice's kind over the entry it is about, inside it (a post that mentions the reader): a caption's glyph and
 * sentence, in the text's color with the unread mark at the line's end while unread, muted once read.
 */
export function NoticeCaption({
  icon,
  unread: mark,
  children,
}: {
  icon: IconName;
  unread?: string | undefined;
  children: ReactNode;
}) {
  const s = noticeCaption({ read: mark === undefined });
  return (
    <div className={s.root()}>
      <p className={s.text()}>
        <span className={s.glyph()}>
          <Icon name={icon} place="caption" />
        </span>
        <span>{children}</span>
      </p>
      {mark !== undefined && <UnreadMark label={mark} />}
    </div>
  );
}

const name = tv({ base: "font-strong" });

/** A name inside a sentence: strong and isolated. */
export function SentenceName({ children }: { children: ReactNode }) {
  return <bdi className={name()}>{children}</bdi>;
}

const excerpt = tv({ base: "line-clamp-2 text-muted-text [&_p]:inline [&_p+p]:before:content-['_']" });

/**
 * The first words of something a line is about (the reader's post a notice is about): muted, its paragraphs run on,
 * on two lines at most.
 */
export function Excerpt({ lang, children }: { lang?: string | undefined; children: ReactNode }) {
  return (
    <div className={excerpt()} lang={lang}>
      {children}
    </div>
  );
}

const state = tv({
  base: "inline-flex h-control-timeline items-center text-caption text-muted-text",
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
    <span ref={ref} className={state({ focusable })} tabIndex={focusable ? -1 : undefined}>
      {children}
    </span>
  );
}

const pair = tv({ base: "flex flex-none items-center justify-end gap-screen-tabs-gap" });

/** A refusal beside its confirmation, the confirmation at the end; or one action alone at the end. */
export function ButtonPair({ children }: { children: ReactNode }) {
  return <div className={pair()}>{children}</div>;
}

const found = tv({ base: "grid gap-entry-room rounded-overlay bg-surface-raised p-[0.75rem]" });

/**
 * What a field found, under it, on the raised surface in the rhythm of the rows (the accounts a list's members may
 * take), apart from the rows below; `label` names the group.
 */
export function FoundRows({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div role="group" aria-label={label} className={found()}>
      {children}
    </div>
  );
}

const rows = tv({ base: "m-0 grid list-none gap-entry-room p-0" });

/** Rows of one kind (a list's members) as a list, named by `label`, in the rhythm of the rows: each a `RowItem`. */
export function RowList({ label, children }: { label: string; children: ReactNode }) {
  return (
    <ul aria-label={label} className={rows()}>
      {children}
    </ul>
  );
}

const result = tv({
  base: [
    // Room above the caption, which sits tight on its rows; the same caption-to-row space for every kind of row, a row
    // that opens (an `OpenRow`, padded for its hover) drawn up by its padding.
    "m-0 mt-screen-group-gap grid grid-cols-[minmax(0,1fr)] gap-screen-air",
    "[&>ul>li:first-child>a]:-mt-screen-row-block",
  ],
});

/**
 * One kind of what a screen found (the people of a search), a region named by its heading, muted and small as a
 * list's group's: its rows, or the line that stands in their place, under it.
 */
export function ResultGroup({ title, children }: { title: string; children: ReactNode }) {
  const id = useId();
  return (
    <section aria-labelledby={id} className={result()}>
      <h2 id={id} className={group()}>
        {title}
      </h2>
      {children}
    </section>
  );
}

/** A row of a `RowList` (an `AccountRow`, an `OpenRow`). */
export function RowItem({ children }: { children: ReactNode }) {
  return <li>{children}</li>;
}

const account = tv({
  slots: { root: "flex items-center gap-screen-row-gap", who: "min-w-0 flex-1" },
});

/** An account on a row of its own: its avatar, its name over its handle, and what the reader may do at the end. */
export function AccountRow({
  avatar,
  name: accountName,
  handle,
  href,
  end,
}: {
  avatar: ReactNode;
  name: ReactNode;
  handle: string;
  /** Where the name leads: the account's profile. */
  href?: string | undefined;
  end?: ReactNode;
}) {
  const s = account();
  return (
    <div className={s.root()}>
      <div className={s.who()}>
        <Byline avatar={avatar} name={accountName} handle={handle} href={href} />
      </div>
      {end !== undefined && <ButtonPair>{end}</ButtonPair>}
    </div>
  );
}

const row = tv({
  slots: {
    root: [
      "-mx-screen-row-inline flex items-center gap-screen-row-gap rounded-control px-screen-row-inline py-screen-row-block",
      "text-text no-underline outline-hidden data-hovered:bg-surface-hover data-focus-visible:shadow-focus",
    ],
    glyph: "inline-flex flex-none",
    lines: "grid min-w-0 flex-1",
    title: "text-body font-label",
    hint: "text-caption text-muted-text",
    chevron: "inline-flex flex-none text-muted-text",
  },
});

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
    <Link href={href} className={s.root()}>
      <span className={s.glyph()}>
        <Icon name={icon} place="action" />
      </span>
      <span className={s.lines()}>
        <span className={s.title()}>{title}</span>
        {hint !== undefined && <span className={s.hint()}>{hint}</span>}
      </span>
      <span className={s.chevron()}>
        <Icon name="onward" place="action" />
      </span>
    </Link>
  );
}

const faces = tv({
  slots: {
    // The whole row is the link: the surface on hover and the ring on focus, so its chevron reads as part of it. The
    // ring that cuts a face out takes the surface under it.
    root: [
      "-mx-screen-row-inline flex items-center gap-screen-row-gap rounded-control px-screen-row-inline py-screen-row-block",
      "text-text no-underline outline-hidden [--face-cut-color:var(--color-surface)]",
      "data-hovered:bg-surface-hover data-hovered:[--face-cut-color:var(--color-surface-hover)]",
      "data-focus-visible:shadow-focus",
    ],
    lead: "inline-flex min-w-screen-faces-lead flex-none items-center gap-chip-gap text-body",
    glyph: "inline-flex text-[length:var(--chip-glyph)] leading-none text-muted-text",
    count: "tabular-nums",
    // As many faces as fit: a face that does not wraps to a second line, which the strip's height hides, so that faces
    // drop from the end whole, never shrink or scroll. The padding keeps their cut rings inside it.
    strip: ["box-content flex h-avatar-post min-w-0 flex-1 flex-wrap overflow-hidden", "p-screen-face-cut"],
    face: "inline-flex flex-none rounded-full shadow-face-cut [&+&]:-ms-screen-face-overlap",
    chevron: "inline-flex flex-none text-muted-text",
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
    <Link href={href} className={s.root()} aria-label={label} {...{ title: label }}>
      <span className={s.lead()} aria-hidden="true">
        <span className={s.glyph()}>{glyph}</span>{" "}
        <bdi dir="ltr" lang={lang} className={s.count()}>
          {count}
        </bdi>
      </span>
      <span className={s.strip()} aria-hidden="true">
        {items.map((face, i) => (
          <span key={i} className={s.face()}>
            {face}
          </span>
        ))}
      </span>
      <span className={s.chevron()} aria-hidden="true">
        <Icon name="onward" place="action" />
      </span>
    </Link>
  );
}

const title = tv({
  slots: {
    root: "m-0 flex flex-wrap items-center gap-x-entry-caption text-title font-normal",
    glyph: "inline-flex text-muted-text",
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
    <h2 className={s.root()}>
      {glyph !== undefined && <span className={s.glyph()}>{glyph}</span>}
      <span>{children}</span>
      {count !== undefined && <span className={s.count()}>{count}</span>}
    </h2>
  );
}

const screenTitle = tv({
  slots: {
    // The back button close over the row, so that it reads as part of the head.
    root: "grid gap-screen-name-gap",
    // The title at the start, what the reader may do at the end, on the baseline of the title's first line.
    row: "flex items-baseline gap-screen-head-room",
    // One run of text, so that a long name wraps under its glyph and the count follows its last line.
    title: "m-0 min-w-0 flex-1 text-title font-normal [overflow-wrap:anywhere]",
    glyph: "me-entry-caption inline-flex align-[-0.125em] text-muted-text",
    count: "text-body whitespace-nowrap text-muted-text tabular-nums",
    actions: "flex flex-none items-center gap-screen-tab-gap",
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
};

/**
 * The head of a screen of one thing a timeline is of (a hashtag, a list): its back button, then one row of its glyph
 * and name, how many, muted, at the start, and what the reader may do with it at the end.
 */
export function ScreenTitle({ glyph, count, actions, back, children }: ScreenTitleProps) {
  const s = screenTitle();
  return (
    <div className={s.root()}>
      {back}
      <div className={s.row()}>
        <h2 className={s.title()}>
          {glyph !== undefined && (
            <span className={s.glyph()}>
              <Icon name={glyph} place="action" />
            </span>
          )}
          <bdi>{breakable(children)}</bdi>
          {/* A space apart from the name, read so too: "Bread and coffee 2 members", never "coffee2 members". */}
          {count !== undefined && (
            <>
              {" "}
              <span className={s.count()}>{count}</span>
            </>
          )}
        </h2>
        {actions !== undefined && actions !== false && actions !== null && <div className={s.actions()}>{actions}</div>}
      </div>
    </div>
  );
}

const back = tv({
  slots: {
    root: "m-0 text-caption",
    button: [
      "inline-flex cursor-pointer items-center gap-entry-caption rounded-control border-0 bg-transparent p-0",
      "text-caption text-muted-text outline-hidden [font-family:inherit]",
      "data-hovered:text-text data-focus-visible:shadow-focus",
    ],
  },
});

/** Back to where the reader came from, muted and small, at the start of the screen. */
export function BackButton({ onPress, children }: { onPress: () => void; children: string }) {
  const s = back();
  return (
    <p className={s.root()}>
      <Button className={s.button()} onPress={onPress}>
        <Icon name="back" place="caption" />
        {children}
      </Button>
    </p>
  );
}
