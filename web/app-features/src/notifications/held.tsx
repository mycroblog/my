// SPDX-License-Identifier: AGPL-3.0-only

// The notifications the reader's notification policy holds back (ADR-0158 §7), on screens of their own under
// Notifications, which the row over the notifications opens. The first lists the accounts that sent them, the newest
// first, each with how many it holds, its name leading to them, and Dismiss beside Accept; the second, an account's
// held-back notifications, the account and the same answer over them. The first leads to the notification settings,
// where the policy that holds them back is chosen. Accepting moves the notifications into the
// list and lets the account's later ones through; dismissing drops them. An answer shows in place of the buttons and
// takes their focus, and the row over the notifications counts again at once.

import {
  AccountLine,
  AccountName,
  handleOf,
  heldOfPath,
  ListColumn,
  nameOf,
  notificationSettingsPath,
} from "@my/app-components";
import {
  notificationRequestQuery,
  notificationRequestsQuery,
  requestNotificationsQuery,
  useAnswerRequest,
  useNotificationRequest,
  useNotificationRequests,
  useRequestAnswer,
  useRequestNotifications,
  type AppData,
  type NotificationRequest,
  type RequestAnswer,
} from "@my/app-data";
import {
  AccountRow,
  Avatar,
  BackButton,
  Button,
  FeedNotice,
  Fold,
  FormText,
  ScreenHead,
  ScreenStack,
  ScreenTitle,
  StateText,
  TextButton,
  TextLink,
  useLocale,
} from "@my/design-system";
import { useEffect, useRef } from "react";
import { stringsIn } from "../locale";
import { NotificationRow } from "./notifications";

type T = ReturnType<typeof stringsIn>["t"];

/** What the screen of the requests waits for: their first page, from the cache unless a write marked it stale. */
export async function loadHeld(data: AppData): Promise<void> {
  await data.queryClient
    .infiniteQuery({ ...notificationRequestsQuery(data), staleTime: Infinity })
    .catch(() => undefined);
}

/** What the screen of an account's held-back notifications waits for: its request and its first page. */
export async function loadHeldOf(data: AppData, id: string): Promise<void> {
  await Promise.all([
    data.queryClient.query({ ...notificationRequestQuery(data, id), staleTime: Infinity }),
    data.queryClient.infiniteQuery({ ...requestNotificationsQuery(data, id), staleTime: Infinity }),
  ]).catch(() => undefined);
}

/**
 * Dismiss beside Accept for the request of `id`; once answered, what the reader answered, which takes the focus of the
 * button pressed (gone with it). A press while the server answers does nothing.
 */
function Answers({ id, t }: { id: string; t: T }) {
  const answered = useRequestAnswer(id);
  const answer = useAnswerRequest();
  const sending = useRef(false);
  const pressed = useRef(false);
  const result = useRef<HTMLSpanElement>(null);
  useEffect(() => {
    if (answered !== undefined && pressed.current) {
      pressed.current = false;
      result.current?.focus();
    }
  }, [answered]);
  if (answered !== undefined)
    return (
      <StateText ref={result}>
        {t(answered === "accepted" ? "notifications.held_accepted" : "notifications.held_dismissed")}
      </StateText>
    );
  const on = (what: RequestAnswer) => () => {
    if (sending.current) return;
    sending.current = true;
    pressed.current = true;
    answer(id, what)
      .catch(() => {
        pressed.current = false;
      })
      .finally(() => {
        sending.current = false;
      });
  };
  return (
    <>
      <Button size="timeline" onPress={on("dismissed")}>
        {t("notifications.held_dismiss")}
      </Button>
      <Button size="timeline" kind="primary" onPress={on("accepted")}>
        {t("notifications.held_accept")}
      </Button>
    </>
  );
}

/** An account's request: its name leading to its held-back notifications, how many, and the answer at the end. */
function Request({ request, t }: { request: NotificationRequest; t: T }) {
  const { account } = request;
  return (
    <AccountRow
      avatar={<Avatar name={nameOf(account)} src={account.avatar?.small.url} size="post" decorative />}
      name={<AccountName account={account} />}
      handle={handleOf(account)}
      href={heldOfPath(request.id)}
      note={t("notifications.held_count", { count: request.notifications_count })}
      end={<Answers id={request.id} t={t} />}
    />
  );
}

/**
 * The screen of the requests, back to the notifications at its top: a row per account, then a line that they load,
 * that they failed (with Try again), that there are none, or a fold that shows more.
 */
export function Held({ onBack }: { onBack: () => void }) {
  const { t } = stringsIn(useLocale().locale);
  const requests = useNotificationRequests();
  const { items, status } = requests;
  const retry = (onPress: () => void) => <TextButton onPress={onPress}>{t("notifications.retry")}</TextButton>;
  // The way to where the reader chooses what is held back: the policy, in the notification settings.
  const settings = <TextLink href={notificationSettingsPath}>{t("notifications.held_settings")}</TextLink>;
  const failedMore = status === "success" && requests.error !== null && !requests.isFetchingNextPage;
  const end =
    status === "pending" || requests.isFetchingNextPage ? (
      <FeedNotice start>{t("notifications.held_loading")}</FeedNotice>
    ) : status === "error" ? (
      <FeedNotice start alert action={retry(requests.refetch)}>
        {t("notifications.held_failed")}
      </FeedNotice>
    ) : failedMore && requests.hasNextPage ? (
      <FeedNotice start alert action={retry(requests.fetchNextPage)}>
        {t("notifications.held_failed")}
      </FeedNotice>
    ) : items.length === 0 ? (
      <FeedNotice start action={settings}>
        {t("notifications.held_empty")}
      </FeedNotice>
    ) : requests.hasNextPage ? (
      <Fold>
        <TextButton onPress={requests.fetchNextPage}>{t("profile.follows_more")}</TextButton>
      </Fold>
    ) : undefined;
  return (
    <ScreenStack busy={status === "pending"}>
      <ScreenTitle glyph="followers" back={<BackButton onPress={onBack}>{t("notifications.label")}</BackButton>}>
        {t("notifications.held")}
      </ScreenTitle>
      {items.length > 0 && (
        <FormText muted>
          {t("notifications.held_about")} {settings}
        </FormText>
      )}
      {items.map((r) => (
        <Request key={r.id} request={r} t={t} />
      ))}
      {end}
    </ScreenStack>
  );
}

/**
 * The screen of the held-back notifications of the account of `id`: under the way back to the requests, the account's
 * row with the answer as its head, then its notifications as the list draws them.
 */
export function HeldOf({ id, instance, onBack }: { id: string; instance: string; onBack: () => void }) {
  const { t } = stringsIn(useLocale().locale);
  const { request, status, refetch } = useNotificationRequest(id);
  const list = useRequestNotifications(id);
  const answered = useRequestAnswer(id);
  const back = <BackButton onPress={onBack}>{t("notifications.held")}</BackButton>;
  // The server lists no request of the account (answered elsewhere): it says so. A read that failed otherwise says so,
  // with Try again.
  if (status === "gone" && answered === undefined)
    return (
      <ScreenStack>
        <ScreenTitle back={back}>{t("notifications.held")}</ScreenTitle>
        <FeedNotice start>{t("notifications.held_gone")}</FeedNotice>
      </ScreenStack>
    );
  if (status === "error" && answered === undefined)
    return (
      <ScreenStack>
        <ScreenTitle back={back}>{t("notifications.held")}</ScreenTitle>
        <FeedNotice start alert action={<TextButton onPress={refetch}>{t("notifications.retry")}</TextButton>}>
          {t("notifications.held_failed")}
        </FeedNotice>
      </ScreenStack>
    );
  return (
    <ScreenStack busy={request === undefined}>
      {/* The account's row is the screen's head, under the way back. */}
      <ScreenHead back={back}>
        {request !== undefined && <AccountLine account={request.account} end={<Answers id={id} t={t} />} heading />}
      </ScreenHead>
      <ListColumn
        {...list}
        label={t("notifications.held")}
        keyOf={(n) => n.id}
        words={{
          loading: t("notifications.held_loading"),
          loadingMore: t("notifications.loading_more"),
          failed: t("notifications.held_failed"),
          failedMore: t("notifications.failed_more"),
          empty: t("notifications.held_none"),
        }}
      >
        {(n) => <NotificationRow n={n} unread={false} t={t} instance={instance} held />}
      </ListColumn>
    </ScreenStack>
  );
}
