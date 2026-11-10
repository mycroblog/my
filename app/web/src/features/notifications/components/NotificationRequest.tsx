// SPDX-License-Identifier: AGPL-3.0-only
// The page of one notification request (web-client/screens/notifications-request.md, ADR-0158 §7): who sent the held
// notifications and how many, the answers with what they do, and the notifications as the notifications page shows
// them (a feed, Y6), without the marks of unread ones, since held notifications pass no read marker.
import { AccountItem, Button, Cluster, EmptyState, Feed, Hint, InlineError, Stack } from "@my/ui";
import { handleOf } from "../../../lib/handle";
import { imageSources } from "../../../lib/images";
import { useT } from "../../../lib/i18n";
import { profilePath } from "../../../lib/paths";
import { type HeldNotifications, useNotificationRequest } from "../hooks/useNotificationRequest";
import { requestName } from "../hooks/useNotificationRequests";
import { NotificationItem } from "./NotificationItem";
import { NotificationsError } from "./NotificationsError";
import { NotificationsSkeleton } from "./NotificationsSkeleton";

export type NotificationRequestProps = {
  /** The ID of the request's source account, from the URL. */
  accountId: string;
  /** The ID of the page heading, which names the feed. */
  headingId: string;
};

export function NotificationRequest({ accountId, headingId }: NotificationRequestProps) {
  const t = useT();
  const view = useNotificationRequest(accountId);
  const { request, notifications } = view;
  const { account } = request;
  const name = requestName(request);
  return (
    <Stack gap="6">
      <Stack gap="3">
        <AccountItem
          name={name}
          handle={handleOf(account.acct)}
          avatar={imageSources(account.avatar)}
          avatarSeed={account.id}
          href={profilePath(account.acct)}
          details={<span>{t("client:notification_requests.count", { count: request.notifications_count })}</span>}
        />
        <Hint size="inherit">{t("client:notification_requests.answers.description", { name })}</Hint>
        <Cluster gap="2">
          <Button
            variant="primary"
            isPending={view.pending === "accept"}
            isDisabled={view.pending === "dismiss"}
            onPress={() => {
              view.answer("accept");
            }}
          >
            {t("client:notification_requests.accept")}
          </Button>
          <Button
            isPending={view.pending === "dismiss"}
            isDisabled={view.pending === "accept"}
            onPress={() => {
              view.answer("dismiss");
            }}
          >
            {t("client:notification_requests.dismiss")}
          </Button>
        </Cluster>
        {view.error !== undefined && <InlineError alert>{view.error}</InlineError>}
      </Stack>
      <HeldNotificationList notifications={notifications} headingId={headingId} />
    </Stack>
  );
}

function HeldNotificationList({ notifications, headingId }: { notifications: HeldNotifications; headingId: string }) {
  const t = useT();
  switch (notifications.status) {
    case "loading":
      return <NotificationsSkeleton />;
    case "error":
      return (
        <NotificationsError
          headingLevel={2}
          message={notifications.message}
          retry={notifications.retry}
          isRetrying={notifications.isRetrying}
        />
      );
    case "loaded":
      // The server counts what it holds; the notifications about deleted posts are left out of the list.
      if (notifications.items.length === 0)
        return <EmptyState heading={t("client:notification_requests.notifications.empty")} />;
      return (
        <Feed
          aria-labelledby={headingId}
          items={notifications.items}
          getKey={(group) => group.key}
          renderItem={(group) => <NotificationItem group={group} isUnread={false} />}
          isLoading={notifications.isLoadingMore}
          hasMore={notifications.hasMore}
          onLoadMore={notifications.loadMore}
          loadMoreLabel={t("client:notifications.load_more")}
          loadingLabel={t("client:notifications.loading")}
          estimateSize={120}
        />
      );
  }
}
