// SPDX-License-Identifier: AGPL-3.0-only

// The notifications, at /notifications, a place of the navigation: the reader's notifications, which the server's
// notices name by the instance's name. Without a session it asks the reader to sign in.

import { loadNotifications, Notifications } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/notifications")({
  loader: ({ context }) => (context.data === null ? undefined : loadNotifications(context.data)),
  component: NotificationsScreen,
  // A place of the navigation opens at its top, where the newest notifications are.
  staticData: { scroll: "top" },
});

function NotificationsScreen() {
  const { data, config } = Route.useRouteContext();
  const t = useStrings();
  return (
    <>
      <ScreenHeading>{t("app.notifications")}</ScreenHeading>
      {data === null ? <SignedOut /> : <Notifications instance={config.instanceName} />}
    </>
  );
}
