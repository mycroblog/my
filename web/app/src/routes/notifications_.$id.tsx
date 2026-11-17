// SPDX-License-Identifier: AGPL-3.0-only

// One notification, at /notifications/{id}, where a push and an email about it lead (the server names no screen of the
// client): it reads the notification and goes on to what it tells of (notificationPath), in place of this address in
// the history, and counts it as read. One that leads nowhere else opens the notifications; one the server no longer has
// (dismissed, or never the reader's) opens them with a line that says so. While it is read, and when that failed (with
// Try again), a line says so. Without a session it asks the reader to sign in, and the sign-in comes back here.

import { notificationPath } from "@my/app-components";
import { notificationQuery, useNotification, useNotificationReads } from "@my/app-data";
import { FeedNotice, ScreenStack, TextButton } from "@my/design-system";
import { createFileRoute, useRouter } from "@tanstack/react-router";
import { useEffect, useRef } from "react";
import { useStrings } from "../locale";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/notifications_/$id")({
  loader: ({ context, params }) =>
    screenLoad(context, async (data) => {
      await data.queryClient
        .query({ ...notificationQuery(data, params.id), staleTime: Infinity })
        .catch(() => undefined);
    }),
  component: NotificationScreen,
  staticData: { scroll: "top" },
});

/** Goes on to where the notification of `id` leads, once read; says that it is read, or that it failed. */
function Open({ id }: { id: string }) {
  const t = useStrings();
  const router = useRouter();
  const { notification, status, refetch } = useNotification(id);
  const reads = useNotificationReads();
  // Once: it goes on as soon as the notification is read, never again for a render after.
  const went = useRef(false);
  useEffect(() => {
    if (went.current) return;
    if (notification !== undefined) {
      went.current = true;
      reads.read(notification.id);
      void router.navigate({ href: notificationPath(notification), replace: true });
    } else if (status === "gone") {
      went.current = true;
      void router.navigate({ to: "/notifications", search: { gone: true }, replace: true });
    }
  }, [notification, status, router, reads]);
  return (
    <ScreenStack busy={status === "pending"}>
      {status === "error" ? (
        <FeedNotice start alert action={<TextButton onPress={refetch}>{t("app.notification_retry")}</TextButton>}>
          {t("app.notification_failed")}
        </FeedNotice>
      ) : (
        <FeedNotice start>{t("app.notification_opening")}</FeedNotice>
      )}
    </ScreenStack>
  );
}

function NotificationScreen() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  const t = useStrings();
  return (
    <>
      <ScreenHeading>{t("app.notifications")}</ScreenHeading>
      {data === null ? <SignedOut returnTo={`/notifications/${encodeURIComponent(id)}`} /> : <Open key={id} id={id} />}
    </>
  );
}
