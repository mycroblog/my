// SPDX-License-Identifier: AGPL-3.0-only

// Account status, at /settings/account/status, the screen under the settings' entry Account and data (ADR-0110 §5)
// that the entry's status leads to: the moderators' actions in effect on the reader's account and the past ones, each
// the door to its own screen; back goes to the entry. Without a session it asks the reader to sign in.

import { AccountStatusScreen, loadAccountStatus, useAccountStatusTitle } from "@my/app-features";
import { createFileRoute, useRouter } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/settings/account/status/")({
  loader: ({ context }) => screenLoad(context, loadAccountStatus),
  component: AccountStatusRoute,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function AccountStatusRoute() {
  const { data } = Route.useRouteContext();
  const router = useRouter();
  const title = useAccountStatusTitle();
  return (
    <>
      <ScreenHeading>{title}</ScreenHeading>
      {data === null ? (
        <SignedOut />
      ) : (
        <AccountStatusScreen
          onBack={() => {
            void router.navigate({ to: "/settings/$entry", params: { entry: "account" } });
          }}
        />
      )}
    </>
  );
}
