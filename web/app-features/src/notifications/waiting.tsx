// SPDX-License-Identifier: AGPL-3.0-only

// What waits for the reader's approval (ADR-0155 §10): the replies, boosts and quotes of the reader's posts from
// people outside who may do them at once. Over the notifications, a row counts them and opens their screen; on it,
// under its title with a back control to the notifications, each says who asks for what, with what is approved (the
// reply, the quoting post, or for a boost the reader's post), and Decline beside Approve, which once answered say what
// the reader answered. A request's own notification is a plain line of the feed, answered here alone.

import { excerptHtml, ListColumn, nameOf, PostContent, waitingPath } from "@my/app-components";
import {
  interactionRequestsQuery,
  useAnswerInteraction,
  useWaitingCount,
  useWaitingRequests,
  waitingRequestsQuery,
  type AppData,
  type InteractionRequest,
} from "@my/app-data";
import {
  Avatar,
  BackButton,
  Button,
  ButtonPair,
  Entry,
  Excerpt,
  NoticeLine,
  OpenRow,
  ScreenStack,
  ScreenTitle,
  StateText,
  useLocale,
} from "@my/design-system";
import { useEffect, useId, useRef } from "react";
import { stringsIn } from "../locale";
import { glyphOf } from "./kinds";

type T = ReturnType<typeof stringsIn>["t"];

/** The notification of each kind of request, whose sentence and glyph a request shares. */
const noticeOf = { reply: "reply_request", announce: "boost_request", quote: "quote_request" } as const;

/**
 * What the screen's route waits for: the first page of the requests, from the cache unless a write marked it stale. A
 * failure is left to the screen.
 */
export async function loadWaitingRequests(data: AppData): Promise<void> {
  await data.queryClient.infiniteQuery({ ...waitingRequestsQuery(data), staleTime: Infinity }).catch(() => undefined);
}

/** What the notifications' loader also waits for: the count of what waits, for the row over them. */
export async function loadWaitingCount(data: AppData): Promise<void> {
  await data.queryClient.query({ ...interactionRequestsQuery(data), staleTime: Infinity }).catch(() => undefined);
}

/** The row over the notifications that counts what waits for the reader's approval and opens its screen; none at 0. */
export function WaitingRow({ t }: { t: T }) {
  const count = useWaitingCount();
  if (count === undefined || count === 0) return null;
  return <OpenRow href={waitingPath} icon="time" title={t("notifications.waiting_count", { count })} />;
}

/**
 * Decline beside Approve; once answered, what the reader answered, which takes the focus of the button pressed (gone
 * with it), so that a screen reader reads the answer. A press while the server answers does nothing. No button shows
 * itself busy: React Aria announces a pending button by its id, which the answer takes away, and the announcement
 * would be left naming nothing.
 */
function Answers({ r, t }: { r: InteractionRequest; t: T }) {
  const answer = useAnswerInteraction();
  const sending = useRef(false);
  const pressed = useRef(false);
  const result = useRef<HTMLSpanElement>(null);
  const answered = r.state !== "pending";
  useEffect(() => {
    if (answered && pressed.current) {
      pressed.current = false;
      result.current?.focus();
    }
  }, [answered]);
  if (answered)
    return (
      <StateText ref={result}>
        {t(r.state === "accepted" ? "notifications.approved" : "notifications.declined")}
      </StateText>
    );
  const on = (accept: boolean) => () => {
    if (sending.current) return;
    sending.current = true;
    pressed.current = true;
    answer(r.id, accept)
      .catch(() => {
        pressed.current = false;
      })
      .finally(() => {
        sending.current = false;
      });
  };
  return (
    <>
      <Button size="timeline" onPress={on(false)}>
        {t("notifications.decline")}
      </Button>
      <Button size="timeline" kind="primary" onPress={on(true)}>
        {t("notifications.approve")}
      </Button>
    </>
  );
}

/** A request: who asks for what, what is approved, and its answers. */
function Request({ r, t }: { r: InteractionRequest; t: T }) {
  const id = useId();
  const notice = noticeOf[r.kind];
  // What is approved: the reply or the quoting post, or for a boost (and a reply the server has not) the reader's post.
  const shown = r.interacting_status ?? r.status;
  return (
    <Entry labelledBy={id}>
      <NoticeLine
        icon={glyphOf(notice)}
        avatar={<Avatar name={nameOf(r.account)} src={r.account.avatar?.small.url} size="inline" decorative />}
        date={r.created_at}
        id={id}
      >
        {t(`notifications.kind.${notice}`, { name: nameOf(r.account) })}
      </NoticeLine>
      <Excerpt>
        <PostContent html={excerptHtml(shown)} emojis={shown.emojis} lang={undefined} />
      </Excerpt>
      <ButtonPair>
        <Answers r={r} t={t} />
      </ButtonPair>
    </Entry>
  );
}

/** The screen of what waits for the reader's approval, newest first, back to the notifications at its top. */
export function WaitingRequests({ onBack }: { onBack: () => void }) {
  const { t } = stringsIn(useLocale().locale);
  const { items, ...list } = useWaitingRequests();
  const waiting = items.filter((r) => r.state === "pending").length;
  return (
    <ScreenStack>
      <ScreenTitle
        glyph="time"
        count={list.status === "success" && waiting > 0 ? String(waiting) : undefined}
        back={<BackButton onPress={onBack}>{t("notifications.label")}</BackButton>}
      >
        {t("notifications.waiting")}
      </ScreenTitle>
      <ListColumn
        {...list}
        label={t("notifications.waiting")}
        items={items}
        keyOf={(r) => r.id}
        words={{
          loading: t("notifications.waiting_loading"),
          loadingMore: t("notifications.waiting_loading_more"),
          failed: t("notifications.waiting_failed"),
          failedMore: t("notifications.waiting_failed_more"),
          empty: t("notifications.waiting_empty"),
        }}
      >
        {(r) => <Request r={r} t={t} />}
      </ListColumn>
    </ScreenStack>
  );
}
