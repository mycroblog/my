// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { NotificationRequests, notificationRequestsQueryOptions } from "../features/notifications";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { PageHeading } from "./-layout/PageHeading";

/**
 * The notifications that the user's notification policy held, one request per source account (ADR-0158 §7). The `_`
 * keeps the page out of the notifications page, which has no outlet.
 */
export const Route = createFileRoute("/notifications_/requests")({
  staticData: { reloadSafe: true, title: "client:notification_requests.title", back: true },
  beforeLoad: requireSignIn,
  loader: ({ context: { queryClient, api } }) =>
    queryClient.infiniteQuery({ ...notificationRequestsQueryOptions(api), staleTime: Infinity }),
  component: NotificationRequestsPage,
});

function NotificationRequestsPage() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:notification_requests.title")}</PageHeading>
      <NotificationRequests />
    </>
  );
}
