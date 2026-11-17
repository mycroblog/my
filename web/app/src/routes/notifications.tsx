// SPDX-License-Identifier: AGPL-3.0-only

// The notifications, at /notifications, a place of the navigation: the reader's notifications, which the server's
// notices name by the instance's name. `?gone` says over them that the notification a link led to is no longer there
// (notifications_.$id.tsx), and `?tab=mentions` opens them on the Mentions tab. Without a session it asks the reader
// to sign in.

import { loadNotifications, Notifications } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/notifications")({
  validateSearch: (search: Record<string, unknown>): { gone?: boolean; tab?: "mentions" } => ({
    ...(search["gone"] === true || search["gone"] === "true" ? { gone: true } : {}),
    ...(search["tab"] === "mentions" ? { tab: "mentions" } : {}),
  }),
  loader: ({ context }) => screenLoad(context, loadNotifications),
  component: NotificationsScreen,
  // A place of the navigation opens at its top, where the newest notifications are.
  staticData: { scroll: "top" },
});

function NotificationsScreen() {
  const { data, config } = Route.useRouteContext();
  const { gone, tab } = Route.useSearch();
  const t = useStrings();
  return (
    <>
      <ScreenHeading>{t("app.notifications")}</ScreenHeading>
      {data === null ? (
        <SignedOut />
      ) : (
        <Notifications instance={config.instanceName} gone={gone === true} defaultFilter={tab} />
      )}
    </>
  );
}
