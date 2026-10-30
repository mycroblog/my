// SPDX-License-Identifier: AGPL-3.0-only
// One notification as an article of the notifications feed (web-client/README.md Y6): a sentence that says what
// happened ("Alice followed you") with the actor's name as a link to their profile, when, relative to now
// as in the timelines (G8), and for the types about a post, a quote of the post that links to its page.
// A follow offers to follow back. PostCard lays it out, so the article is named by the sentence and the
// time and described by the post. The follows that a suspension by the moderators ended say which server or
// account was suspended and how many followers and follows the user lost.
//
// The notifications from the instance lead to where the user follows them up, as links in the row's actions:
// a moderation notice (and the decision on an appeal of it) says what the action does, why, and until when,
// and leads to the account's status; an appeal to the moderators leads to the appeal; a
// resolved report names the reported account; a moderator's new report or registration leads to the report or
// the registrations; a custom emoji request leads the administrators to the requests and its decision the
// requester to their custom emoji, with the reason of a rejection; a supporter notification names the plan and
// says when the grace period ends or why the subscription ended, and leads to the supporter page except in the
// Tauri app, which shows no way to pay (ADR-0072 §14); and lost follows lead to the user's lists of followers and
// follows. An announcement of the administrators has the announcement's mark in place of an avatar, and its title as
// the sentence, a link to it on the list page, which marks it read.
//
// A group of the favourites or the boosts of one post is one row: "Alice and 2 others favourited your
// post", with the avatars of the others in a row above the sentence, so the number of people is in the
// article's name.
//
// A row has one tab stop per place it leads to: the actor's name (the avatar is the same link for the
// pointer, out of the tab order), and the quote of the post or the follow back button. The avatars of the
// others are links to their profiles for the pointer only; the post's page lists who favourited and
// boosted it.
import {
  Avatar,
  EmojiText,
  Hint,
  Button,
  Link,
  NotificationKindMark,
  NotificationLinks,
  NotificationOthers,
  NotificationSentence,
  PostCard,
  PostCardAvatarLink,
  PostCardNameLink,
  PostExcerpt,
  PostExcerptText,
  QuoteBlock,
  ReplyIcon,
  Text,
} from "@my/ui";
import { useIntl } from "react-intl";
import { useT } from "../../../lib/i18n";
import { useReply } from "../../compose";
import { isolate, RelationshipActions, useRelationship } from "../../profile";
import type { NotificationAccount } from "../api/notifications";
import type { NotificationGroup } from "../hooks/groupNotifications";
import {
  type ModerationNoticeView,
  type MoveOutcomeKey,
  type NotificationActor,
  type NotificationView,
  type SupporterNoticeView,
  useNotificationView,
} from "../hooks/useNotificationView";

/** The most avatars of the others that a group shows; the sentence counts them all. */
const maxOtherAvatars = 6;

export type NotificationItemProps = {
  /** One notification, or the favourites or the boosts of one post. */
  group: NotificationGroup;
  /** Whether it was unread when the page opened. */
  isUnread: boolean;
};

export function NotificationItem({ group, isUnread }: NotificationItemProps) {
  const t = useT();
  const intl = useIntl();
  const view = useNotificationView(group);
  const reply = useReply();
  const { actor } = view;
  // A row without an actor names the account it is about, without its avatar.
  const named = actor ?? view.subject;
  const replyTo = view.replyTo;
  return (
    <PostCard
      // The kind's mark on the avatar's corner, or alone in its place for a row without an actor (P3).
      avatar={
        <NotificationKindMark kind={view.kind}>
          {actor !== null && (
            <PostCardAvatarLink href={actor.href}>
              <Avatar sources={actor.avatar} name={actor.name} seed={actor.id} decorative />
            </PostCardAvatarLink>
          )}
        </NotificationKindMark>
      }
      author={
        <>
          {view.others.length > 0 && <OtherAvatars others={view.others.slice(0, maxOtherAvatars)} />}
          <NotificationSentence isUnread={isUnread} unreadLabel={t("client:notifications.unread")}>
            {/* The actor's name is the user's text, isolated from the sentence around it (Y10). */}
            {intl.formatMessage(
              { id: view.sentence },
              {
                name:
                  named === null ? null : (
                    <PostCardNameLink key="name" href={named.href}>
                      <EmojiText text={named.name} emojis={named.emojis} />
                    </PostCardNameLink>
                  ),
                count: view.others.length,
                // The server's name is no running text, isolated from the sentence around it (Y10).
                domain:
                  view.severance === null || view.severance.domain === null ? null : (
                    <bdi key="domain">{view.severance.domain}</bdi>
                  ),
                // The title is the administrators' plain text in any language, isolated by the link (Y10).
                title:
                  view.announcement === null ? null : (
                    <PostCardNameLink key="title" href={view.announcement.href}>
                      {view.announcement.title}
                    </PostCardNameLink>
                  ),
                followers: view.severance?.followers ?? 0,
                following: view.severance?.following ?? 0,
                // The emoji is the reacting user's choice, isolated from the sentence around it (Y10).
                // A requested emoji's shortcode is no running text either.
                emoji:
                  view.reaction !== null ? (
                    <bdi key="emoji">{view.reaction}</bdi>
                  ) : view.emojiRequest === null ? null : (
                    <bdi key="emoji" dir="ltr">
                      {view.emojiRequest.emoji}
                    </bdi>
                  ),
              },
            )}
          </NotificationSentence>
        </>
      }
      time={
        <time dateTime={view.time.dateTime} title={view.time.absolute}>
          {view.time.relative}
        </time>
      }
      // A notification is not a post; the post it is about is quoted in the content.
      visibility={null}
      content={
        view.post !== null ? (
          <PostQuote post={view.post} />
        ) : view.notice !== null ? (
          <ModerationNotice notice={view.notice} />
        ) : view.emojiRequest !== null && view.emojiRequest.reason !== null ? (
          <RejectionReason reason={view.emojiRequest.reason} />
        ) : view.supporter !== null ? (
          <SupporterNotice supporter={view.supporter} />
        ) : view.move !== null && view.move.outcome !== null ? (
          <MoveOutcome move={view.move} outcome={view.move.outcome} />
        ) : null
      }
      {...(view.followBack !== null && { actions: <FollowBack account={view.followBack} /> })}
      {...(replyTo !== null &&
        reply !== null && {
          // A mention most often wants an answer: the reply starts here, without opening the post first.
          actions: (
            <Button
              size="sm"
              variant="subtle"
              onPress={() => {
                reply(replyTo);
              }}
            >
              <ReplyIcon />
              {t("client:notifications.mention.reply")}
            </Button>
          ),
        })}
      {...(view.move !== null && {
        actions: (
          <NotificationLinks>
            <Link href={view.move.href} variant="standalone">
              {t("client:notifications.account_moved.target", { target: isolate(view.move.handle) })}
            </Link>
          </NotificationLinks>
        ),
      })}
      {...(view.links.length > 0 && {
        actions: (
          <NotificationLinks>
            {view.links.map((link) => (
              <Link key={link.href} href={link.href} variant="standalone">
                {t(link.label)}
              </Link>
            ))}
          </NotificationLinks>
        ),
      })}
    />
  );
}

/**
 * The avatars of the other people of a group, as links to their profiles for the pointer: out of the tab
 * order and hidden from screen readers, which hear their number in the sentence.
 */
function OtherAvatars({ others }: { others: readonly NotificationActor[] }) {
  return (
    <NotificationOthers>
      {others.map((other) => (
        <PostCardAvatarLink key={other.href} href={other.href}>
          <Avatar sources={other.avatar} name={other.name} seed={other.id} size="xs" decorative />
        </PostCardAvatarLink>
      ))}
    </NotificationOthers>
  );
}

/**
 * The post a notification is about, as a quote that links to its page in the app, with its thread and the
 * actions on it (FR-113). The server embeds it in the notification, and leaves it out when the user may not
 * see it. A post behind a content warning shows the warning only.
 */
function PostQuote({ post }: { post: NonNullable<NotificationView["post"]> }) {
  const t = useT();
  const intl = useIntl();
  const label =
    post.contentWarning !== null ? (
      // The warning is the author's text in the post's language, isolated from the UI's sentence (Y10).
      intl.formatMessage(
        { id: "client:notifications.post.content_warning" },
        {
          summary: (
            <bdi key="summary" lang={post.lang}>
              {post.contentWarning}
            </bdi>
          ),
        },
      )
    ) : post.text === "" ? (
      // A post of pictures only has no text to quote.
      t("client:notifications.post.open")
    ) : (
      <PostExcerptText lang={post.lang}>{post.text}</PostExcerptText>
    );
  return <PostExcerpt href={post.href}>{label}</PostExcerpt>;
}

/**
 * What a moderation notice tells of the action on the user's account: what it does and why, the moderators'
 * explanation (their own text, in its own direction; Y10), and when it ends by itself.
 */
function ModerationNotice({ notice }: { notice: ModerationNoticeView }) {
  const t = useT();
  const intl = useIntl();
  return (
    <QuoteBlock>
      <Text as="p" weight="semibold">
        {t("client:notifications.moderation.action", { kind: notice.kind, category: notice.category })}
      </Text>
      {notice.reason !== null && (
        <Text as="p" dir="auto" whitespace="preLine" wrap="anywhere" align="matchParent">
          {notice.reason}
        </Text>
      )}
      {notice.decision !== null && (
        <Text as="p" weight="semibold">
          {t(notice.decision.sentence)}
        </Text>
      )}
      {notice.decision !== null && notice.decision.reason !== null && (
        <Text as="p" dir="auto" whitespace="preLine" wrap="anywhere" align="matchParent">
          {notice.decision.reason}
        </Text>
      )}
      {notice.until !== null && (
        <Hint as="p">
          {intl.formatMessage(
            { id: "client:notifications.moderation.until" },
            {
              date: (
                <time key="until" dateTime={notice.until.dateTime}>
                  {notice.until.text}
                </time>
              ),
            },
          )}
        </Hint>
      )}
    </QuoteBlock>
  );
}

/** The administrator's reason of a rejected custom emoji request: their own text, in its own direction (Y10). */
function RejectionReason({ reason }: { reason: string }) {
  const t = useT();
  return (
    <QuoteBlock>
      <Hint as="p">{t("client:notifications.custom_emoji_request.reason")}</Hint>
      <Text as="p" dir="auto" whitespace="preLine" wrap="anywhere">
        {reason}
      </Text>
    </QuoteBlock>
  );
}

/**
 * What a supporter notification tells of the user's subscription: the plan's name (the administrators' text, in its
 * own direction and of no known language; Y10), and when the grace period ends, why it ended, or what a retirement
 * does. It names no way to pay, which the Tauri app does not offer (ADR-0072 §14).
 */
function SupporterNotice({ supporter }: { supporter: SupporterNoticeView }) {
  const t = useT();
  const intl = useIntl();
  const { detail } = supporter;
  return (
    <QuoteBlock>
      <Text as="p" dir="auto" lang="" weight="semibold" wrap="anywhere">
        {supporter.plan}
      </Text>
      {detail?.kind === "grace" && (
        <Hint as="p">
          {intl.formatMessage(
            { id: "client:notifications.supporter.grace" },
            {
              date: (
                <time key="grace" dateTime={detail.date.dateTime}>
                  {detail.date.text}
                </time>
              ),
            },
          )}
        </Hint>
      )}
      {detail?.kind === "ended" && (
        <Hint as="p">{t("client:notifications.supporter.ended", { reason: detail.reason })}</Hint>
      )}
      {detail?.kind === "retired" && <Hint as="p">{t("client:notifications.supporter.retired")}</Hint>}
    </QuoteBlock>
  );
}

/**
 * What became of the user's follow when the account moved: they follow the new account instead, or why they do not
 * (the new account's handle is isolated from the sentence around it; Y10).
 */
function MoveOutcome({ move, outcome }: { move: NonNullable<NotificationView["move"]>; outcome: MoveOutcomeKey }) {
  const t = useT();
  return (
    <QuoteBlock>
      <Text as="p">{t(outcome, { target: isolate(move.handle) })}</Text>
    </QuoteBlock>
  );
}

/** Follow back from a follow notification: the profile's follow button, without its menu. */
function FollowBack({ account }: { account: NotificationAccount }) {
  const relationship = useRelationship(account);
  return (
    <RelationshipActions
      account={account}
      name={account.display_name || account.username}
      state={relationship}
      showMenu={false}
    />
  );
}
