// SPDX-License-Identifier: AGPL-3.0-only
// The row of the notification requests at the top of the notifications (web-client/screens/notifications.md §3,
// ADR-0158 §7): how many people's notifications the notification policy held, as a link to the requests, and how many
// notifications wait there. Nothing while none waits.
import { Callout, Hint, Link } from "@my/ui";
import { useT } from "../../../lib/i18n";
import { notificationRequestsPath } from "../api/requests";
import { useNotificationRequestSummary } from "../hooks/useNotificationRequestSummary";

export function NotificationRequestsLink() {
  const t = useT();
  const summary = useNotificationRequestSummary();
  if (summary === null) return null;
  return (
    <Callout>
      <Link variant="standalone" href={notificationRequestsPath}>
        {t("client:notification_requests.link", { count: summary.accounts_count })}
      </Link>
      <Hint size="inherit">
        {t("client:notification_requests.link.description", { count: summary.notifications_count })}
      </Hint>
    </Callout>
  );
}
