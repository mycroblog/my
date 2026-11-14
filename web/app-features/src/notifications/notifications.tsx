// SPDX-License-Identifier: AGPL-3.0-only

// The reader's notifications (FR-205, ADR-0158 §7, the rough of #3047), newest first, one entry each: over them the
// tabs (all, or the mentions) with mark-all-read at the row's end, then, under all, the row of the requests the
// notification policy holds back. Each unread notification (newer than the read marker) carries its own mark, and is
// read once the reader clicks it or it goes out above the view (@my/app-data moves the marker). The affirmative answer
// of a notification that asks for one (follow back, accept, approve) is the primary button, its refusal a plain one.
// New notifications come in at the top as the stream receives them.

import { AccountLine, ListColumn, nameOf, Post, PostContent } from "@my/app-components";
import {
  notificationMarkerQuery,
  notificationRequestSummaryQuery,
  notificationsQuery,
  useAnswerRequest,
  useChangeRelationship,
  useInteractionRequest,
  useNotificationReads,
  useNotificationRequestSummary,
  useNotifications,
  useRelationship,
  type AppData,
  type InteractionRequest,
  type NotificationFilter,
  type NotificationItem,
} from "@my/app-data";
import {
  Avatar,
  Button,
  ButtonPair,
  CustomEmoji,
  Entry,
  Excerpt,
  IconButton,
  NoticeCaption,
  NoticeLine,
  OpenRow,
  ScreenStack,
  StateText,
  TabRow,
  useAction,
  useLocale,
} from "@my/design-system";
import { useId, useState, type ReactNode } from "react";
import { stringsIn } from "../locale";
import { messages } from "../messages.gen";
import { glyphOf, sentenceOf, shapeOf } from "./kinds";

type T = ReturnType<typeof stringsIn>["t"];

/** Where the row of the requests leads: their screen. */
const requestsPath = "/notifications/requests";

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
  ]).catch(() => undefined);
}

const known = (id: string) => id in (messages["en"] ?? {});

/** A press that waits for the server: busy until it answered, then as the cache says. */
function usePress(): [boolean, (run: () => Promise<void>) => void] {
  const [busy, setBusy] = useState(false);
  return [
    busy,
    (run) => {
      setBusy(true);
      run()
        .catch(() => undefined)
        .finally(() => {
          setBusy(false);
        });
    },
  ];
}

/** Follow back, or follow, the account: the primary button until the reader follows it or asked to. */
function FollowButton({ id, label, t }: { id: string; label: string; t: T }) {
  const relationship = useRelationship(id);
  const change = useChangeRelationship();
  const [busy, press] = usePress();
  if (relationship === undefined) return null;
  if (relationship.following || relationship.requested)
    return <StateText>{t(relationship.following ? "notifications.following" : "notifications.requested")}</StateText>;
  return (
    <Button
      size="timeline"
      kind="primary"
      busy={busy}
      onPress={() => {
        press(() => change(id, "follow"));
      }}
    >
      {label}
    </Button>
  );
}

/** The answers of a request: its refusal, then its confirmation; once answered, what the reader answered. */
function Answers({
  answer,
  open,
  t,
  accept,
  reject,
}: {
  answer: (accept: boolean) => Promise<void>;
  open: boolean;
  t: T;
  accept: string;
  reject: string;
}) {
  const [busy, press] = usePress();
  const [answered, setAnswered] = useState<boolean | undefined>(undefined);
  if (answered !== undefined)
    return <StateText>{t(answered ? "notifications.accepted" : "notifications.rejected")}</StateText>;
  if (!open) return null;
  const on = (yes: boolean) => () => {
    press(async () => {
      await answer(yes);
      setAnswered(yes);
    });
  };
  return (
    <>
      <Button size="timeline" isDisabled={busy} onPress={on(false)}>
        {reject}
      </Button>
      <Button size="timeline" kind="primary" busy={busy} onPress={on(true)}>
        {accept}
      </Button>
    </>
  );
}

function FollowRequest({ n, t }: { n: NotificationItem; t: T }) {
  const id = n.account?.id ?? "";
  const relationship = useRelationship(id);
  const answer = useAnswerRequest();
  if (n.account === undefined) return null;
  return (
    <AccountLine
      account={n.account}
      end={
        <Answers
          open={relationship?.requested_by ?? false}
          answer={(yes) => answer.follow(id, yes)}
          t={t}
          accept={t("notifications.accept")}
          reject={t("notifications.reject")}
        />
      }
    />
  );
}

function InteractionAnswers({ n, t }: { n: NotificationItem; t: T }) {
  const request = useInteractionRequest(n);
  const answer = useAnswerRequest();
  // The request answered here leaves the server's list of those that wait: the answer stays shown.
  const [open, setOpen] = useState<InteractionRequest | undefined>(undefined);
  if (request !== undefined && request.id !== open?.id) setOpen(request);
  if (open === undefined) return null;
  return (
    <ButtonPair>
      <Answers
        open
        answer={(yes) => answer.interaction(open.id, yes)}
        t={t}
        accept={t("notifications.approve")}
        reject={t("notifications.decline")}
      />
    </ButtonPair>
  );
}

/** What the reader may do with a notification of its kind, under its line. */
function Act({ n, t }: { n: NotificationItem; t: T }): ReactNode {
  switch (n.type) {
    case "follow":
      return (
        n.account && (
          <AccountLine
            account={n.account}
            end={<FollowButton id={n.account.id} label={t("notifications.follow_back")} t={t} />}
          />
        )
      );
    case "follow_request":
      return <FollowRequest n={n} t={t} />;
    case "account_moved": {
      const target = n.account_move?.target;
      return (
        target && (
          <AccountLine account={target} end={<FollowButton id={target.id} label={t("notifications.follow")} t={t} />} />
        )
      );
    }
    case "reply_request":
    case "boost_request":
    case "quote_request":
      return <InteractionAnswers n={n} t={t} />;
    default:
      return null;
  }
}

/** A reaction's emoji, in place of the glyph of its kind. */
function emojiOf(n: NotificationItem): ReactNode {
  const r = n.reaction;
  if (r === undefined) return undefined;
  return r.emoji === undefined ? (
    r.name
  ) : (
    <CustomEmoji shortcode={r.emoji.shortcode} url={r.emoji.url} staticUrl={r.emoji.static_url} />
  );
}

function Line({ n, unread, t, instance }: { n: NotificationItem; unread: string | undefined; t: T; instance: string }) {
  const id = useId();
  const avatar = n.account && (
    <Avatar name={nameOf(n.account)} src={n.account.avatar?.small.url} size="inline" decorative />
  );
  return (
    <Entry labelledBy={id}>
      <NoticeLine icon={glyphOf(n.type)} emoji={emojiOf(n)} avatar={avatar} date={n.created_at} id={id} unread={unread}>
        {sentenceOf(n, t, instance, known)}
      </NoticeLine>
      {n.status !== undefined && (
        <Excerpt>
          <PostContent html={n.status.content} emojis={n.status.emojis} lang={undefined} />
        </Excerpt>
      )}
      <Act n={n} t={t} />
    </Entry>
  );
}

function Row({ n, unread, t, instance }: { n: NotificationItem; unread: boolean; t: T; instance: string }) {
  const mark = unread ? t("notifications.unread") : undefined;
  if (shapeOf(n) === "post" && n.status !== undefined)
    return (
      <Post
        status={n.status}
        context={
          <NoticeCaption icon={glyphOf(n.type)} unread={mark}>
            {sentenceOf(n, t, instance, known)}
          </NoticeCaption>
        }
      />
    );
  return <Line n={n} unread={mark} t={t} instance={instance} />;
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
      href={requestsPath}
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
      onEntryClick={reads.read}
    >
      {(n) => <Row n={n} unread={reads.unread(n.id)} t={t} instance={instance} />}
    </ListColumn>
  );
}

/** The reader's notifications; `instance` is the server's name, which the notices of the server say. */
export function Notifications({ instance }: { instance: string }) {
  const { t } = stringsIn(useLocale().locale);
  const [filter, setFilter] = useState<NotificationFilter>("all");
  const reads = useNotificationReads();
  return (
    <ScreenStack>
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
          {filter === "all" && <Requests t={t} />}
          <List filter={filter} t={t} instance={instance} />
        </ScreenStack>
      </TabRow>
    </ScreenStack>
  );
}
