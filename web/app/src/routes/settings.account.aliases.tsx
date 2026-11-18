// SPDX-License-Identifier: AGPL-3.0-only

// Aliases, at /settings/account/aliases, the screen under the settings' entry Account and data (ADR-0110 §5) where
// the reader adds and removes the accounts this one is also known as, which lets each move its followers here. Back
// goes to the entry. Without a session it asks the reader to sign in.

import { AccountAliasesScreen, loadAccountAliases, useAccountAliasesTitle } from "@my/app-features";
import { createFileRoute, useRouter } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/settings/account/aliases")({
  loader: ({ context }) => screenLoad(context, loadAccountAliases),
  component: AccountAliasesRoute,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function AccountAliasesRoute() {
  const { data } = Route.useRouteContext();
  const router = useRouter();
  const title = useAccountAliasesTitle();
  return (
    <>
      <ScreenHeading>{title}</ScreenHeading>
      {data === null ? (
        <SignedOut />
      ) : (
        <AccountAliasesScreen
          onBack={() => {
            void router.navigate({ to: "/settings/$entry", params: { entry: "account" } });
          }}
        />
      )}
    </>
  );
}
