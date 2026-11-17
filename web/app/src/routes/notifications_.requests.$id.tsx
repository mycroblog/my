// SPDX-License-Identifier: AGPL-3.0-only

// The held-back notifications of one account, at /notifications/requests/{id} (ADR-0158 §7), which its row on the
// screen of the held-back notifications opens: the account, to accept or dismiss, over its notifications, back to the
// held-back notifications. Without a session it asks the reader to sign in.

import { HeldOf, loadHeldOf } from "@my/app-features";
import { createFileRoute, useRouter } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/notifications_/requests/$id")({
  loader: ({ context, params }) => screenLoad(context, (data) => loadHeldOf(data, params.id)),
  component: HeldOfScreen,
  staticData: { scroll: "top" },
});

function HeldOfScreen() {
  const { data, config } = Route.useRouteContext();
  const { id } = Route.useParams();
  const router = useRouter();
  const t = useStrings();
  return (
    <>
      <ScreenHeading>{t("app.notifications")}</ScreenHeading>
      {data === null ? (
        <SignedOut />
      ) : (
        <HeldOf
          key={id}
          id={id}
          instance={config.instanceName}
          onBack={() => {
            void router.navigate({ to: "/notifications/requests" });
          }}
        />
      )}
    </>
  );
}
