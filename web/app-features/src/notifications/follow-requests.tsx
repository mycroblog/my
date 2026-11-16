// SPDX-License-Identifier: AGPL-3.0-only

// The follow requests to the reader (FR-302), on a screen of their own apart from what waits for their approval
// (waiting.tsx): over the notifications, a row counts them, above the row of what waits, and opens the screen; on it,
// under its title with a back control to the notifications, each account that asks, with Decline beside Approve, which
// once answered say what the reader answered, in place. A request's own notification is a plain line of the feed,
// answered here alone.

import { AccountLine } from "@my/app-components";
import {
  followRequestCountQuery,
  followRequestsQuery,
  useAnswerFollowRequest,
  useFollowRequestCount,
  useFollowRequests,
  useStillAsking,
  useRelationship,
  type Account,
  type AppData,
} from "@my/app-data";
import {
  BackButton,
  Button,
  FeedNotice,
  Fold,
  OpenRow,
  ScreenStack,
  ScreenTitle,
  StateText,
  TextButton,
  useLocale,
} from "@my/design-system";
import { useEffect, useRef } from "react";
import { stringsIn } from "../locale";

type T = ReturnType<typeof stringsIn>["t"];

/** Where the row of the follow requests leads: their screen. */
export const followRequestsPath = "/notifications/follow_requests";

/**
 * What the screen's route waits for: the first page of the requests, from the cache unless a write marked it stale. A
 * failure is left to the screen.
 */
export async function loadFollowRequests(data: AppData): Promise<void> {
  await data.queryClient.infiniteQuery({ ...followRequestsQuery(data), staleTime: Infinity }).catch(() => undefined);
}

/** What the notifications' loader also waits for: the count of the follow requests, for the row over them. */
export async function loadFollowRequestCount(data: AppData): Promise<void> {
  await data.queryClient.query({ ...followRequestCountQuery(data), staleTime: Infinity }).catch(() => undefined);
}

/** The row over the notifications that counts the follow requests and opens their screen; none at 0. */
export function FollowRequestsRow({ t }: { t: T }) {
  const count = useFollowRequestCount();
  if (count === undefined || count === 0) return null;
  return (
    <OpenRow href={followRequestsPath} icon="follow" title={t("notifications.follow_requests_count", { count })} />
  );
}

/**
 * Decline beside Approve; once answered (the account no longer asks), what the reader answered, which takes the focus
 * of the button pressed (gone with it), so that a screen reader reads the answer. A press while the server answers
 * does nothing.
 */
function Answers({ account, t }: { account: Account; t: T }) {
  const relationship = useRelationship(account.id);
  const answer = useAnswerFollowRequest();
  const sending = useRef(false);
  const pressed = useRef(false);
  const result = useRef<HTMLSpanElement>(null);
  const answered = relationship !== undefined && !relationship.requested_by;
  useEffect(() => {
    if (answered && pressed.current) {
      pressed.current = false;
      result.current?.focus();
    }
  }, [answered]);
  if (relationship === undefined) return null;
  if (answered)
    return (
      <StateText ref={result}>
        {t(relationship.followed_by ? "notifications.approved" : "notifications.declined")}
      </StateText>
    );
  const on = (accept: boolean) => () => {
    if (sending.current) return;
    sending.current = true;
    pressed.current = true;
    answer(account.id, accept)
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

/**
 * The screen of the follow requests, newest first, back to the notifications at its top: a row per account that asks,
 * then a line that they load, that they failed (with Try again), that there are none, or a fold that shows more.
 */
export function FollowRequests({ onBack }: { onBack: () => void }) {
  const { t } = stringsIn(useLocale().locale);
  const requests = useFollowRequests();
  const { items, status } = requests;
  // The title counts those that still wait: one answered here leaves the count, though its row stays.
  const asking = useStillAsking(items);
  const retry = (onPress: () => void) => <TextButton onPress={onPress}>{t("notifications.retry")}</TextButton>;
  const failedMore = status === "success" && requests.error !== null && !requests.isFetchingNextPage;
  const end =
    status === "pending" || requests.isFetchingNextPage ? (
      <FeedNotice start>{t("notifications.follow_requests_loading")}</FeedNotice>
    ) : status === "error" ? (
      <FeedNotice start alert action={retry(requests.refetch)}>
        {t("notifications.follow_requests_failed")}
      </FeedNotice>
    ) : failedMore && requests.hasNextPage ? (
      <FeedNotice start alert action={retry(requests.fetchNextPage)}>
        {t("notifications.follow_requests_failed")}
      </FeedNotice>
    ) : items.length === 0 ? (
      <FeedNotice start>{t("notifications.follow_requests_empty")}</FeedNotice>
    ) : requests.hasNextPage ? (
      <Fold>
        <TextButton onPress={requests.fetchNextPage}>{t("profile.follows_more")}</TextButton>
      </Fold>
    ) : undefined;
  return (
    <ScreenStack busy={status === "pending"}>
      <ScreenTitle
        glyph="follow"
        count={status === "success" && asking > 0 ? String(asking) : undefined}
        back={<BackButton onPress={onBack}>{t("notifications.label")}</BackButton>}
      >
        {t("notifications.follow_requests")}
      </ScreenTitle>
      {items.map((a) => (
        <AccountLine key={a.id} account={a} end={<Answers account={a} t={t} />} />
      ))}
      {end}
    </ScreenStack>
  );
}
