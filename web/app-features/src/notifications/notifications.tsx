// SPDX-License-Identifier: AGPL-3.0-only

// The reader's notifications (FR-205, ADR-0158 §7, the rough of #3047), newest first, one entry each: over them the
// tabs (all, or the mentions) with mark-all-read at the row's end, then, under all, the row of the requests the
// notification policy holds back. Each unread notification (newer than the read marker) carries its own mark, and is
// read once the reader acts on it or it goes out above the view (@my/app-data moves the marker): a press on its words,
// on a control or a link of it, or on an item of a menu of it; opening a menu is not yet an action. Over them all, a row
// counts the follow requests and opens their screen (follow-requests.tsx), and one under it what waits for the reader's
// approval (waiting.tsx), where those are answered: their notifications here are plain lines. A notification that
// asks for an answer of its own (follow back) has it as the primary button; that of a scheduled post that could not be
// published opens its draft in the composer.
// New notifications come in at the top as the stream receives them. The actions of a post it shows are answered as on
// every screen (post-actions.tsx).

import {
  AccountLine,
  excerptHtml,
  FilterSentence,
  heldPath,
  ListColumn,
  nameOf,
  notificationPath,
  PostContent,
} from "@my/app-components";
import {
  notificationMarkerQuery,
  notificationSettingsQuery,
  pushSubscriptionQuery,
  notificationRequestSummaryQuery,
  notificationsQuery,
  refusalDetail,
  useDismissNotification,
  useNotificationReads,
  useNotificationRequestSummary,
  useNotifications,
  type AppData,
  type NotificationFilter,
  type NotificationItem,
} from "@my/app-data";
import {
  Avatar,
  CustomEmoji,
  Entry,
  Excerpt,
  FeedNotice,
  HeardActions,
  IconButton,
  NoticeCaption,
  NoticeLine,
  OpenRow,
  ScreenStack,
  TabRow,
  TextButton,
  useAction,
  useLocale,
} from "@my/design-system";
import { useId, useRef, useState, type ReactNode } from "react";
import { FollowOf } from "../account-follow";
import { stringsIn } from "../locale";
import { useCompose, usePostActions, usePostNotice, usePostOwn, usePostPicker, usePostVoting } from "../post-actions";
import { TranslatablePost as Post } from "../post-translation";
import { messages } from "../messages.gen";
import { glyphOf, sentenceOf, shapeOf } from "./kinds";
import { FollowRequestsRow, loadFollowRequestCount } from "./follow-requests";
import { PushPrompt } from "./push-prompt";
import { loadWaitingCount, WaitingRow } from "./waiting";

type T = ReturnType<typeof stringsIn>["t"];

/**
 * What the route's loader waits for: the first page of all the notifications, the read marker and the count of the
 * requests, from the cache unless a write marked them stale. A failure is left to the screen.
 */
export async function loadNotifications(data: AppData): Promise<void> {
  const { queryClient } = data;
  await Promise.all([
    queryClient.infiniteQuery({ ...notificationsQuery(data, "all"), staleTime: Infinity }),
    queryClient.query({ ...notificationMarkerQuery(data), staleTime: Infinity }),
    queryClient.query({ ...notificationRequestSummaryQuery(data), staleTime: Infinity }),
    loadWaitingCount(data),
    loadFollowRequestCount(data),
  ]).catch(() => undefined);
  // What the prompt to turn Web Push on reads, so that it shows with the screen rather than after it; a failure leaves
  // it out.
  await Promise.all([
    queryClient.query({ ...pushSubscriptionQuery(data), staleTime: Infinity }),
    queryClient.query({ ...notificationSettingsQuery(data), staleTime: Infinity }),
  ]).catch(() => undefined);
}

const known = (id: string) => id in (messages["en"] ?? {});

/** What the reader may do with a notification of its kind, under its line (`FollowOf`: account-follow.tsx). */
function Act({ n, t }: { n: NotificationItem; t: T }): ReactNode {
  switch (n.type) {
    case "follow":
      // Follow back, from the relationship, as every row of an account follows.
      return n.account && <AccountLine account={n.account} end={<FollowOf account={n.account} />} />;
    case "account_moved": {
      const target = n.account_move?.target;
      return target && <AccountLine account={target} end={<FollowOf account={target} />} />;
    }
    case "scheduled_status_failed":
      return <OpenDraft n={n} t={t} />;
    default:
      return null;
  }
}

/** The draft of a scheduled post that could not be published, opened in the composer to fix and send again. */
function OpenDraft({ n, t }: { n: NotificationItem; t: T }) {
  const compose = useCompose();
  const id = n.scheduled_status_failure?.draft_id;
  if (compose === undefined || id === undefined) return null;
  return (
    <TextButton
      onPress={() => {
        compose({ kind: "draft", id });
      }}
    >
      {t("notifications.open_draft")}
    </TextButton>
  );
}

/**
 * A reaction's emoji, in place of the glyph of its kind: a custom emoji's image, or a Unicode emoji. A custom emoji
 * whose image the server cannot show (it has a shortcode and no image) keeps the kind's glyph: its name is no emoji.
 */
function emojiOf(n: NotificationItem): ReactNode {
  const r = n.reaction;
  if (r === undefined) return undefined;
  if (r.emoji !== undefined)
    return <CustomEmoji shortcode={r.emoji.shortcode} url={r.emoji.url} staticUrl={r.emoji.static_url} />;
  return r.shortcode === undefined ? r.name : undefined;
}

/**
 * Dismisses the notification by its button at the line's end: it leaves the list at once; a failure puts it back and says so, with Try again. A second
 * press while the first is in flight does nothing.
 */
function Dismiss({ n, t }: { n: NotificationItem; t: T }) {
  const dismiss = useDismissNotification();
  const failed = usePostNotice();
  const flying = useRef(false);
  const run = () => {
    if (flying.current) return;
    flying.current = true;
    dismiss(n.id)
      .catch((error: unknown) => {
        // The server's own reason where it gives one; never a bare HTTP title.
        failed(undefined, refusalDetail(error) ?? t("notifications.dismiss_failed"), run);
      })
      .finally(() => {
        flying.current = false;
      });
  };
  return <IconButton icon="close" label={t("notifications.dismiss")} onPress={run} />;
}

/** Where a notification's line leads, when it leads elsewhere than the notifications themselves. */
function targetOf(n: NotificationItem): string | undefined {
  const path = notificationPath(n);
  return path === "/notifications" ? undefined : path;
}

function Line({
  n,
  unread,
  t,
  instance,
  held,
}: {
  n: NotificationItem;
  unread: string | undefined;
  t: T;
  instance: string;
  held: boolean;
}) {
  const id = useId();
  const avatar = n.account && (
    <Avatar name={nameOf(n.account)} src={n.account.avatar?.small.url} size="inline" decorative />
  );
  return (
    <Entry labelledBy={id}>
      <NoticeLine
        icon={glyphOf(n.type)}
        emoji={emojiOf(n)}
        avatar={avatar}
        date={n.created_at}
        id={id}
        unread={unread}
        href={targetOf(n)}
        action={held ? undefined : <Dismiss n={n} t={t} />}
        excerpt={
          n.status !== undefined && (
            <Excerpt>
              {/* A post that the reader's keyword filters matched shows none of its words here, only the filters'
                  names (ADR-0157 §7, as a push about it does); the line leads to the post, which shows them. */}
              {n.status.filtered.length > 0 ? (
                <FilterSentence filtered={n.status.filtered} />
              ) : (
                <PostContent html={excerptHtml(n.status)} emojis={n.status.emojis} lang={undefined} />
              )}
            </Excerpt>
          )
        }
      >
        {sentenceOf(n, t, instance, known)}
      </NoticeLine>
      <Act n={n} t={t} />
    </Entry>
  );
}

/**
 * A notification of the list: the post it is about whole, under the sentence of its kind, for a mention, a quote or a
 * new post; else the line of its kind. `held` draws one the notification policy holds back, which is answered with its
 * account's request, never dismissed alone.
 */
export function NotificationRow({
  n,
  unread,
  t,
  instance,
  held = false,
}: {
  n: NotificationItem;
  unread: boolean;
  t: T;
  instance: string;
  held?: boolean;
}) {
  const mark = unread ? t("notifications.unread") : undefined;
  const onAction = usePostActions();
  const votingOf = usePostVoting();
  const ownOf = usePostOwn();
  const picker = usePostPicker();
  if (shapeOf(n) === "post" && n.status !== undefined)
    return (
      <Post
        status={n.status}
        context={
          <NoticeCaption icon={glyphOf(n.type)} unread={mark} action={held ? undefined : <Dismiss n={n} t={t} />}>
            {sentenceOf(n, t, instance, known)}
          </NoticeCaption>
        }
        onAction={onAction}
        voting={votingOf(n.status)}
        own={ownOf(n.status)}
        picker={picker}
      />
    );
  return <Line n={n} unread={mark} t={t} instance={instance} held={held} />;
}

function ReadAll({ t, onPress }: { t: T; onPress: () => void }) {
  const keys = useAction("notifications.read_all", onPress, { scope: "screen", label: t("notifications.read_all") });
  return <IconButton icon="read" label={t("notifications.read_all")} onPress={onPress} {...keys} />;
}

function Requests({ t }: { t: T }) {
  const summary = useNotificationRequestSummary();
  if (summary === undefined || summary.accounts_count === 0) return null;
  return (
    <OpenRow
      href={heldPath}
      icon="followers"
      title={t("notifications.requests", { people: summary.accounts_count, count: summary.notifications_count })}
      hint={t("notifications.requests_hint")}
    />
  );
}

function List({ filter, t, instance }: { filter: NotificationFilter; t: T; instance: string }) {
  const { items, ...list } = useNotifications(filter);
  const reads = useNotificationReads();
  return (
    <ListColumn
      {...list}
      label={t("notifications.label")}
      items={items}
      keyOf={(n) => n.id}
      words={{
        loading: t("notifications.loading"),
        loadingMore: t("notifications.loading_more"),
        failed: t("notifications.failed"),
        failedMore: t("notifications.failed_more"),
        empty: t("notifications.empty"),
      }}
      onPassed={reads.read}
    >
      {(n) => {
        // The one place where a notification is read by what the reader does on it, whatever the row holds, now or
        // later: a press on it, the one that opens a menu or a picker included, or on an item of a menu of it, and
        // a shortcut's key that acts on it (reply, boost, favourite, more). Either shows that the reader saw it.
        const read = () => {
          reads.read(n.id);
        };
        return (
          <HeardActions onAct={read}>
            <NotificationRow n={n} unread={reads.unread(n.id)} t={t} instance={instance} />
          </HeardActions>
        );
      }}
    </ListColumn>
  );
}

/**
 * The reader's notifications; `instance` is the server's name, which the notices of the server say. `gone` says over
 * them that a notification a link led to is no longer there (dismissed, or never the reader's).
 */
export function Notifications({
  instance,
  gone = false,
  defaultFilter = "all",
}: {
  instance: string;
  gone?: boolean;
  /** The tab shown at first, which the reader changes; all of them by default. */
  defaultFilter?: NotificationFilter | undefined;
}) {
  const { t } = stringsIn(useLocale().locale);
  const [filter, setFilter] = useState<NotificationFilter>(defaultFilter);
  const reads = useNotificationReads();
  return (
    <ScreenStack>
      {gone && <FeedNotice start>{t("notifications.gone")}</FeedNotice>}
      <PushPrompt t={t} />
      <TabRow
        label={t("notifications.filters")}
        tabs={[
          { id: "all", name: t("notifications.all") },
          { id: "mentions", name: t("notifications.mentions") },
        ]}
        current={filter}
        onChange={setFilter}
        end={
          <ReadAll
            t={t}
            onPress={() => {
              void reads.readAll().catch(() => undefined);
            }}
          />
        }
      >
        <ScreenStack>
          <FollowRequestsRow t={t} />
          <WaitingRow t={t} />
          {filter === "all" && <Requests t={t} />}
          <List filter={filter} t={t} instance={instance} />
        </ScreenStack>
      </TabRow>
    </ScreenStack>
  );
}
