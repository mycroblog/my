// SPDX-License-Identifier: AGPL-3.0-only

// The direct messages (FR-212, ADR-0154 §6): the reader's `direct` posts by conversation, each by its latest post,
// with the other accounts and whether it is unread; the head of a conversation; and a reply in place at its end, which
// keeps `direct` and its people. A conversation of the list is an entry of a feed, as a notice is (list.tsx), on the
// page's one surface: a line of the people's faces and names, the unread mark, the time and more at its end, over the
// latest post's first two lines at the full width (the pick conversation-list, line). The people's names lay out in
// the reader's direction, each name isolated, so that a list of names in two scripts keeps its order and its commas,
// and truncates at its end.

import type { ReactNode } from "react";
import { tv } from "tailwind-variants/lite";
import { Stamp } from "./entry";
import { Icon } from "./icons";
import { UnreadMark } from "./list";
import { useReader } from "./locale";

const faces = tv({
  slots: {
    // The ring that cuts a face out of the one before takes the surface under the entry, hovered or not.
    root: [
      "inline-flex flex-none items-center [--face-cut-color:var(--color-surface)]",
      "in-data-surface-marked:[--face-cut-color:var(--color-surface-hover)]",
    ],
    // Each tucks a little under the one before, so that every face stays recognisable.
    face: "inline-flex flex-none rounded-full shadow-face-cut [&+&]:-ms-screen-face-tuck",
  },
});

/** The first three of a conversation's people as faces in a row (avatars at a row's size), each a little over the next. */
export function ConversationFaces({ children }: { children: readonly ReactNode[] }) {
  const s = faces();
  return (
    <span className={s.root()} aria-hidden="true">
      {children.slice(0, 3).map((face, i) => (
        <span key={i} className={s.face()}>
          {face}
        </span>
      ))}
    </span>
  );
}

const line = tv({
  slots: {
    // Tight inside the conversation: its line and its excerpt a line's gap apart; the feed keeps the space between.
    root: "grid min-w-0 grid-cols-[minmax(0,1fr)] gap-entry-line",
    head: "flex min-w-0 items-center gap-entry-name",
    names: "min-w-0 flex-1 truncate leading-entry",
    // The time's link is a target a row's height tall, as the line is.
    end: "flex flex-none items-center gap-entry-caption [&_a]:inline-flex [&_a]:min-h-avatar-row [&_a]:items-center",
    more: "-my-icon-button-inline -me-icon-button-inline inline-flex flex-none",
    // A post's body (`ContentBody`) runs on after its author, in the excerpt's color.
    excerpt: [
      "line-clamp-2 min-w-0 [&_p]:inline [&_p+p]:before:content-['_']",
      "[&_div]:inline [&_div]:text-[inherit]",
    ],
  },
  variants: {
    // Unread, the names are strong and the excerpt in the text's color; read, the names regular and the excerpt muted.
    read: {
      true: { names: "font-normal", excerpt: "text-muted-text" },
      false: { names: "font-strong", excerpt: "text-text" },
    },
  },
});

export type ConversationLineProps = {
  /** The people's avatars at a row's size, the first of them first; the first three show. */
  faces: readonly ReactNode[];
  /**
   * The people's names, as the reader's locale lists them ("Mika, Ren and 2 others"), each name in a <bdi> of its own
   * and the words around them the locale's.
   */
  names: ReactNode;
  /** The id of the names, which name the entry. */
  namesId?: string | undefined;
  /** When the latest post was posted. */
  date: Date | string;
  /** Where the time leads: the conversation, as a post's time leads to its thread. */
  href?: string | undefined;
  /** The unread mark's label while it is unread; undefined once read. */
  unread?: string | undefined;
  /**
   * The latest post's text (a `ConversationText`), its author first where the names do not say it already, as the
   * reader's locale writes them ("You: …", a `ConversationAuthor` in place of "You").
   */
  children: ReactNode;
  /** What the reader may do with the conversation (a `MenuButton` of more: hide). */
  more?: ReactNode;
};

/**
 * A conversation of the list: the people's faces and names, the unread mark, the time and more at the end; under it,
 * at the full width, the latest post's first two lines, its author first. A press on the time or on more is theirs
 * alone, never the entry's (a feed's `onEntryClick`, which opens the conversation).
 */
export function ConversationLine({
  faces: people,
  names,
  namesId,
  date,
  href,
  unread,
  children,
  more,
}: ConversationLineProps) {
  const s = line({ read: unread === undefined });
  const { direction } = useReader(undefined);
  return (
    <div className={s.root()}>
      <div className={s.head()}>
        <ConversationFaces>{people}</ConversationFaces>
        <span id={namesId} dir={direction} className={s.names()}>
          {names}
        </span>
        <span
          className={s.end()}
          onClick={(e) => {
            e.stopPropagation();
          }}
        >
          {unread !== undefined && <UnreadMark label={unread} />}
          <Stamp date={date} href={href} />
          {more !== undefined && <span className={s.more()}>{more}</span>}
        </span>
      </div>
      <div className={s.excerpt()}>{children}</div>
    </div>
  );
}

const author = tv({ base: "font-strong" });

/** Who wrote a conversation's latest post, in its excerpt: strong, and isolated ("You", "みか"). */
export function ConversationAuthor({ children }: { children: ReactNode }) {
  return <bdi className={author()}>{children}</bdi>;
}

/** A conversation's latest post's text, in its excerpt, in its own language and direction. */
export function ConversationText({ lang, children }: { lang?: string | undefined; children: ReactNode }) {
  return (
    <span lang={lang} dir="auto">
      {children}
    </span>
  );
}

const head = tv({
  slots: {
    root: "grid gap-screen-name-gap",
    row: "flex min-w-0 items-center gap-entry-name",
    title: "m-0 min-w-0 flex-1 truncate text-title font-normal",
    // More's glyph on the column of the posts' more, as an action row reaches past the body.
    more: "-me-icon-button-inline inline-flex flex-none",
  },
});

/**
 * The head of a conversation under its back button: the people's faces and names, which truncate at their end as
 * the list's do, and more (hide) at the end, its glyph on the column of the posts' more.
 */
export function ConversationHead({
  back,
  faces: people,
  more,
  children,
}: {
  /** The back button (`BackButton`) to the list. */
  back: ReactNode;
  /** The people's avatars at a row's size. */
  faces: readonly ReactNode[];
  more?: ReactNode;
  /** The people's names, each in a <bdi> of its own, as the list shows them. */
  children: ReactNode;
}) {
  const s = head();
  const { direction } = useReader(undefined);
  return (
    <div className={s.root()}>
      {back}
      <div className={s.row()}>
        <ConversationFaces>{people}</ConversationFaces>
        <h2 className={s.title()} dir={direction}>
          {children}
        </h2>
        {more !== undefined && <span className={s.more()}>{more}</span>}
      </div>
    </div>
  );
}

const reply = tv({
  slots: {
    root: "grid grid-cols-[minmax(0,1fr)] gap-entry rounded-entry bg-surface-raised p-embed",
    to: "m-0 flex min-w-0 items-center gap-entry-caption text-caption text-muted-text",
    glyph: "inline-flex flex-none",
    people: "min-w-0 truncate",
    row: "flex items-center gap-entry-caption",
    tools: "-ms-icon-button-inline flex min-w-0 flex-1 items-center",
  },
});

/**
 * The reply in place at the end of a conversation, on the raised surface: to whom it goes, with the direct glyph (it
 * stays `direct`, to the conversation's people), the text (a `ComposerText`), and the row of its tools, its count and
 * the action that sends it; a form named for assistive technology, which nothing submits but its action.
 */
export function ConversationReply({
  label,
  to,
  visibility,
  tools,
  count,
  action,
  children,
}: {
  /** The form's name, for assistive technology ("Your reply to Mika and Ren"). */
  label: string;
  /** To whom it goes ("To Mika and Ren"). */
  to: string;
  /** What the glyph alone says for assistive technology (Direct). */
  visibility: string;
  tools?: ReactNode;
  count?: ReactNode;
  action: ReactNode;
  children: ReactNode;
}) {
  const s = reply();
  return (
    <form
      aria-label={label}
      className={s.root()}
      onSubmit={(e) => {
        e.preventDefault();
      }}
    >
      <p className={s.to()}>
        <span role="img" aria-label={visibility} className={s.glyph()}>
          <Icon name="direct" place="caption" />
        </span>
        <bdi className={s.people()}>{to}</bdi>
      </p>
      {children}
      <div className={s.row()}>
        <div className={s.tools()}>{tools}</div>
        {count}
        {action}
      </div>
    </form>
  );
}
