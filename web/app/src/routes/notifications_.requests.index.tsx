// SPDX-License-Identifier: AGPL-3.0-only

// The notifications the reader's notification policy holds back, at /notifications/requests (ADR-0158 §7), which the
// row over the notifications opens: the accounts that sent them, to accept or dismiss, back to the notifications at its
// top. Without a session it asks the reader to sign in.

import { Held, loadHeld } from "@my/app-features";
import { createFileRoute, useRouter } from "@tanstack/react-router";
import { useStrings } from "../locale";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/notifications_/requests/")({
  loader: ({ context }) => screenLoad(context, loadHeld),
  component: HeldScreen,
  // A page of its own opens at its top, where the newest are.
  staticData: { scroll: "top" },
});

function HeldScreen() {
  const { data } = Route.useRouteContext();
  const router = useRouter();
  const t = useStrings();
  return (
    <>
      <ScreenHeading>{t("app.notifications")}</ScreenHeading>
      {data === null ? (
        <SignedOut />
      ) : (
        <Held
          onBack={() => {
            void router.navigate({ to: "/notifications" });
          }}
        />
      )}
    </>
  );
}
