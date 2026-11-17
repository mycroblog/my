// SPDX-License-Identifier: AGPL-3.0-only

// The direct messages (FR-212, ADR-0154 §6): the reader's `direct` posts by conversation, each by its latest post,
// with the other accounts and whether it is unread; the head of a conversation; and a reply in place at its end, which
// keeps `direct` and its people. A conversation of the list is an entry of a feed, as a notice is (list.tsx), on the
// page's one surface: a line of the people's faces and names, the unread mark, the time and more at its end, over the
// latest post's first two lines at the full width (the pick conversation-list, line). The people's names lay out in
// the reader's direction, each name isolated, so that a list of names in two scripts keeps its order and its commas,
// and truncates at its end.

import type { ReactNode } from "react";
import { Stamp } from "./entry";
import { Faces } from "./faces";
import { Icon } from "./icons";
import { Box, Row, Stack } from "./layout";
import { Excerpt, UnreadMark } from "./list";
import { useReader } from "./locale";

/** The first three of a conversation's people as faces in a row (avatars at a row's size), each a little over the next. */
export function ConversationFaces({ children }: { children: readonly ReactNode[] }) {
  return <Faces limit={3}>{children}</Faces>;
}

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
  const read = unread === undefined;
  const { direction } = useReader(undefined);
  return (
    // Tight inside the conversation: its line and its excerpt a line's gap apart; the feed keeps the space between.
    <Stack shrink gap="entry-line">
      <Row shrink align="center" gap="entry-name">
        <ConversationFaces>{people}</ConversationFaces>
        {/* Unread, the names are strong and the excerpt in the text's color; read, the names regular and it muted. */}
        <Box as="span" id={namesId} dir={direction} grow lines={1} leading="entry" weight={read ? "normal" : "strong"}>
          {names}
        </Box>
        <Row
          as="span"
          rigid
          align="center"
          gap="entry-caption"
          onClick={(e) => {
            e.stopPropagation();
          }}
        >
          {unread !== undefined && <UnreadMark label={unread} />}
          <Stamp date={date} href={href} tall />
          {more !== undefined && (
            <Row as="span" inline rigid bleedBlock="icon-button-inline" bleedEnd="icon-button-inline">
              {more}
            </Row>
          )}
        </Row>
      </Row>
      <Excerpt lines={2} tone={read ? "muted" : "text"} body>
        {children}
      </Excerpt>
    </Stack>
  );
}

/** Who wrote a conversation's latest post, in its excerpt: strong, and isolated ("You", "みか"). */
export function ConversationAuthor({ children }: { children: ReactNode }) {
  return (
    <Box as="bdi" weight="strong">
      {children}
    </Box>
  );
}

/** A conversation's latest post's text, in its excerpt, in its own language and direction. */
export function ConversationText({ lang, children }: { lang?: string | undefined; children: ReactNode }) {
  return (
    <span lang={lang} dir="auto">
      {children}
    </span>
  );
}

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
  const { direction } = useReader(undefined);
  return (
    <Stack gap="screen-name-gap">
      {back}
      <Row align="center" gap="entry-name" shrink>
        <ConversationFaces>{people}</ConversationFaces>
        <Box as="h2" dir={direction} grow lines={1} text="title" weight="normal">
          {children}
        </Box>
        {more !== undefined && (
          <Row as="span" inline rigid bleedEnd="icon-button-inline">
            {more}
          </Row>
        )}
      </Row>
    </Stack>
  );
}

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
  return (
    <Stack
      as="form"
      gap="entry"
      surface="raised"
      radius="entry"
      pad="embed"
      aria-label={label}
      onSubmit={(e) => {
        e.preventDefault();
      }}
    >
      <Row as="p" shrink align="center" gap="entry-caption" text="caption" tone="muted">
        <Row as="span" inline rigid role="img" aria-label={visibility}>
          <Icon name="direct" place="caption" />
        </Row>
        <Box as="bdi" shrink lines={1}>
          {to}
        </Box>
      </Row>
      {children}
      <Row align="center" gap="entry-caption">
        <Row grow shrink align="center" bleedStart="icon-button-inline">
          {tools}
        </Row>
        {count}
        {action}
      </Row>
    </Stack>
  );
}
