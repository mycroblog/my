// SPDX-License-Identifier: AGPL-3.0-only

// Import, at /settings/account/import, the screen under the settings' entry Account and data (ADR-0110 §5) where the
// reader imports a CSV file, after a preview of what it would do, and follows the imports the server carries out.
// Back goes to the entry. Without a session it asks the reader to sign in.

import { AccountImportScreen, loadAccountImport, useAccountImportTitle } from "@my/app-features";
import { createFileRoute, useRouter } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/settings/account/import")({
  loader: ({ context }) => screenLoad(context, loadAccountImport),
  component: AccountImportRoute,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function AccountImportRoute() {
  const { data } = Route.useRouteContext();
  const router = useRouter();
  const title = useAccountImportTitle();
  return (
    <>
      <ScreenHeading>{title}</ScreenHeading>
      {data === null ? (
        <SignedOut />
      ) : (
        <AccountImportScreen
          onBack={() => {
            void router.navigate({ to: "/settings/$entry", params: { entry: "account" } });
          }}
        />
      )}
    </>
  );
}
