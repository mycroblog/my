// SPDX-License-Identifier: AGPL-3.0-only
// One conversation as an article of the direct messages feed (web-client/README.md Y6; screens/direct-messages.md):
// the people in it, whose names link to their profiles, when its newest post was written, relative to now as in the
// timelines (G8), and an excerpt of that post, which links to the thread. PostCard lays it out, so the article is
// named by the people and the time, starting with "Unread" while a post of another account is unread, and described
// by the excerpt. The first person's avatar is in the avatar's place, and the others' in a row above the names, as a
// notification of a group shows them.
//
// A row has one tab stop per place it leads to: each name (the avatars are the same links for the pointer, out of the
// tab order) and the excerpt, which opens the thread and marks the conversation read.
import {
  Avatar,
  EmojiText,
  NotificationOthers,
  NotificationSentence,
  PostCard,
  PostCardAvatarLink,
  PostCardNameLink,
  PostExcerpt,
  PostExcerptText,
} from "@my/ui";
import { useIntl } from "react-intl";
import { useT } from "../../../lib/i18n";
import type { Conversation } from "../api/conversations";
import { type ConversationView, type ParticipantView, useConversationView } from "../hooks/useConversationView";

export type ConversationItemProps = {
  conversation: Conversation;
  /** The user opens the thread from the excerpt. */
  onOpen: () => void;
};

export function ConversationItem({ conversation, onOpen }: ConversationItemProps) {
  const t = useT();
  const view = useConversationView(conversation);
  const [first, ...others] = view.participants;
  return (
    <PostCard
      avatar={
        first === undefined ? undefined : (
          <PostCardAvatarLink href={first.href}>
            <Avatar sources={first.avatar} name={first.name} seed={first.id} decorative />
          </PostCardAvatarLink>
        )
      }
      author={
        <>
          {others.length > 0 && (
            <NotificationOthers>
              {others.map((other) => (
                <PostCardAvatarLink key={other.id} href={other.href}>
                  <Avatar sources={other.avatar} name={other.name} seed={other.id} size="xs" decorative />
                </PostCardAvatarLink>
              ))}
            </NotificationOthers>
          )}
          <NotificationSentence isUnread={view.unread} unreadLabel={t("client:direct_messages.unread")}>
            <Participants view={view} />
          </NotificationSentence>
        </>
      }
      time={
        <time dateTime={view.time.dateTime} title={view.time.absolute}>
          {view.time.relative}
        </time>
      }
      // Every post of the list is a direct message; the page says so, and the row is a conversation, not a post.
      visibility={null}
      content={
        <PostExcerpt href={view.href} onPress={onOpen}>
          <Excerpt view={view} />
        </PostExcerpt>
      }
    />
  );
}

/** A participant's name as a link to their profile; the name is the user's text, isolated from the sentence (Y10). */
function NameLink({ participant }: { participant: ParticipantView }) {
  return (
    <PostCardNameLink href={participant.href}>
      <EmojiText text={participant.name} emojis={participant.emojis} />
    </PostCardNameLink>
  );
}

/** The people of the conversation: up to three names in a list of the locale, then how many more. */
function Participants({ view }: { view: ConversationView }) {
  const t = useT();
  const intl = useIntl();
  if (view.named.length === 0) return t("client:direct_messages.participants.self");
  const names = intl.formatList(
    view.named.map((participant) => <NameLink key={participant.id} participant={participant} />),
    { type: view.more > 0 ? "unit" : "conjunction" },
  );
  if (view.more === 0) return names;
  return intl.formatMessage({ id: "client:direct_messages.participants.more" }, { names, count: view.more });
}

/** The excerpt of the newest post: its text, its content warning, or what to do for a post of pictures only. */
function Excerpt({ view }: { view: ConversationView }) {
  const t = useT();
  const intl = useIntl();
  const { excerpt } = view;
  switch (excerpt.kind) {
    case "text":
      return <PostExcerptText lang={excerpt.lang}>{excerpt.text}</PostExcerptText>;
    case "warning":
      // The warning is the author's text in the post's language, isolated from the UI's sentence (Y10).
      return intl.formatMessage(
        { id: "client:direct_messages.content_warning" },
        {
          summary: (
            <bdi key="summary" {...(excerpt.lang !== undefined && { lang: excerpt.lang })}>
              {excerpt.summary}
            </bdi>
          ),
        },
      );
    case "media":
      return t("client:direct_messages.open");
  }
}
