// SPDX-License-Identifier: AGPL-3.0-only

// The parts of a screen over and among the entries of its feed (the notifications, a profile's posts, a thread): the
// screen's parts stacked a feed entry's room apart, the tabs over a list, a heading of a group of a list, a fold of
// what the list does not show, the unread mark, the line of a notice's kind, an excerpt, an account on a row of its
// own, a refusal beside its confirmation, and the row that opens what waits elsewhere. Everything sits on the page's
// one surface, apart by space; a raised surface marks only a control (the current tab).

import type { ReactNode } from "react";
import { Button, Link, Tab, TabList, TabPanel, Tabs } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { Byline, Stamp } from "./entry";
import { Icon, type IconName } from "./icons";
import { useReader } from "./locale";

const screen = tv({ base: "grid grid-cols-[minmax(0,1fr)] gap-entry-room" });

/** A screen's parts one under another (a head, the tabs, a row, the feed), apart by the room a feed entry's ring needs. */
export function ScreenStack({ children }: { children: ReactNode }) {
  return <div className={screen()}>{children}</div>;
}

const tabs = tv({
  slots: {
    root: "grid grid-cols-[minmax(0,1fr)] gap-entry-room",
    row: "flex items-center gap-screen-tabs-gap",
    list: "flex min-w-0 flex-1 gap-screen-tab-gap overflow-x-auto overflow-y-hidden",
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
    root: "flex items-start gap-entry-avatar",
    // The glyph and the avatar stand on the text's first line, so that a sentence that wraps keeps them at its start.
    glyph: [
      "inline-flex h-[1lh] min-w-icon-action flex-none items-center justify-center text-muted-text",
      "text-[length:var(--icon-action)] leading-none",
    ],
    avatar: "inline-flex h-[1lh] flex-none items-center",
    text: "m-0 min-w-0 flex-1",
    end: "inline-flex h-[1lh] flex-none items-center gap-entry-avatar",
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
};

/** The line of a notice's kind: its glyph, the account's avatar, the sentence, which wraps, the unread mark and the time. */
export function NoticeLine({ icon, emoji, avatar, date, id, unread: mark, children }: NoticeLineProps) {
  const s = notice({ read: mark === undefined });
  return (
    <div className={s.root()}>
      <span className={s.glyph()}>{emoji ?? <Icon name={icon} place="action" />}</span>
      {avatar !== undefined && <span className={s.avatar()}>{avatar}</span>}
      <p id={id} className={s.text()}>
        {children}
      </p>
      <span className={s.end()}>
        {mark !== undefined && <UnreadMark label={mark} />}
        <Stamp date={date} />
      </span>
    </div>
  );
}

const noticeCaption = tv({
  slots: {
    root: "flex items-center gap-entry-avatar text-caption",
    text: "m-0 flex min-w-0 flex-1 items-start gap-entry-caption",
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

const state = tv({ base: "inline-flex h-control-timeline items-center text-caption text-muted-text" });

/** What the reader did, or what is so, in place of the button that did it (a request accepted, an account followed). */
export function StateText({ children }: { children: string }) {
  return <span className={state()}>{children}</span>;
}

const pair = tv({ base: "flex flex-none items-center justify-end gap-screen-tabs-gap" });

/** A refusal beside its confirmation, the confirmation at the end; or one action alone at the end. */
export function ButtonPair({ children }: { children: ReactNode }) {
  return <div className={pair()}>{children}</div>;
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
