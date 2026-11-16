// SPDX-License-Identifier: AGPL-3.0-only

// The follow requests to the reader, at /notifications/follow_requests (FR-302), which the row over the notifications
// and a request's notification beside the timeline open: the accounts that ask to follow the reader, to approve or
// decline, back to the notifications at its top. Without a session it asks the reader to sign in.

import { FollowRequests, loadFollowRequests } from "@my/app-features";
import { createFileRoute, useRouter } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/notifications_/follow_requests")({
  loader: ({ context }) => screenLoad(context, loadFollowRequests),
  component: FollowRequestsScreen,
  // A page of its own opens at its top, where the newest requests are.
  staticData: { scroll: "top" },
});

function FollowRequestsScreen() {
  const { data } = Route.useRouteContext();
  const router = useRouter();
  const t = useStrings();
  return (
    <>
      <ScreenHeading>{t("app.notifications")}</ScreenHeading>
      {data === null ? (
        <SignedOut />
      ) : (
        <FollowRequests
          onBack={() => {
            void router.navigate({ to: "/notifications" });
          }}
        />
      )}
    </>
  );
}
