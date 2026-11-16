// SPDX-License-Identifier: AGPL-3.0-only

// What waits for the reader's approval, at /notifications/waiting (ADR-0155 §10), which the row over the notifications
// and a request's notification beside the timeline open: the replies, boosts and quotes of the reader's posts to
// approve or decline, back to the notifications at its top. Without a session it asks the reader to sign in.

import { loadWaitingRequests, WaitingRequests } from "@my/app-features";
import { createFileRoute, useRouter } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/notifications_/waiting")({
  loader: ({ context }) => screenLoad(context, loadWaitingRequests),
  component: WaitingScreen,
  // A page of its own opens at its top, where the newest requests are.
  staticData: { scroll: "top" },
});

function WaitingScreen() {
  const { data } = Route.useRouteContext();
  const router = useRouter();
  const t = useStrings();
  return (
    <>
      <ScreenHeading>{t("app.notifications")}</ScreenHeading>
      {data === null ? (
        <SignedOut />
      ) : (
        <WaitingRequests
          onBack={() => {
            void router.navigate({ to: "/notifications" });
          }}
        />
      )}
    </>
  );
}
