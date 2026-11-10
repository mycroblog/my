// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { Link } from "@my/ui";
import { useId } from "react";
import { z } from "zod";
import {
  NotificationRequest,
  notificationRequestNotificationsQueryOptions,
  notificationRequestQueryOptions,
  NotificationsSkeleton,
} from "../features/notifications";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { ApiError } from "../lib/errors";
import { useT } from "../lib/i18n";
import { PageHeading } from "./-layout/PageHeading";
import { RouteError } from "./-layout/RouteStates";

const paramsSchema = z.object({ accountId: z.string().min(1) });

/**
 * The notifications held in the request of one source account, and the answers to it (ADR-0158 §7). The `_`s keep the
 * page out of the notifications page and the list of requests.
 */
export const Route = createFileRoute("/notifications_/requests_/$accountId")({
  staticData: { reloadSafe: true, title: "client:notification_requests.request.title", back: true },
  params: { parse: (params) => paramsSchema.parse(params) },
  beforeLoad: requireSignIn,
  // The request opens the page, or its 404 is the page's error; a failure of its notifications is the feed's own,
  // shown with what to do (useNotificationRequest), so it lets the page open.
  loader: ({ context: { queryClient, api }, params: { accountId } }) =>
    Promise.all([
      queryClient.query({ ...notificationRequestQueryOptions(api, accountId), staleTime: Infinity }),
      queryClient
        .infiniteQuery({ ...notificationRequestNotificationsQueryOptions(api, accountId), staleTime: Infinity })
        .catch(() => undefined),
    ]),
  component: NotificationRequestPage,
  pendingComponent: NotificationRequestPending,
  errorComponent: NotificationRequestError,
});

function NotificationRequestPage() {
  const t = useT();
  const headingId = useId();
  const { accountId } = Route.useParams();
  return (
    <>
      <PageHeading id={headingId}>{t("client:notification_requests.request.title")}</PageHeading>
      {/* Another account is another request: nothing of the previous one carries over (web-client/README.md R10). */}
      <NotificationRequest key={accountId} accountId={accountId} headingId={headingId} />
    </>
  );
}

function NotificationRequestPending() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:notification_requests.request.title")}</PageHeading>
      <NotificationsSkeleton />
    </>
  );
}

/** An answered request, one of an account the user blocks, and none at all are the same 404 (api/openapi.yaml). */
function NotificationRequestError(props: ErrorComponentProps) {
  const t = useT();
  if (!(props.error instanceof ApiError && props.error.status === 404)) return <RouteError {...props} />;
  return (
    <>
      <PageHeading>{t("client:notification_requests.not_found.title")}</PageHeading>
      <p>{t("client:notification_requests.not_found.body")}</p>
      <p>
        <Link variant="standalone" href="/notifications/requests">
          {t("client:notification_requests.not_found.back")}
        </Link>
      </p>
    </>
  );
}
