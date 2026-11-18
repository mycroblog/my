// SPDX-License-Identifier: AGPL-3.0-only

// Export, at /settings/account/export, the screen under the settings' entry Account and data (ADR-0110 §5) where the
// reader requests and downloads the archive of their account and downloads their lists as CSV files; the closure's
// banner and the closure's emails lead here. Back goes to the entry. Without a session it asks the reader to sign in.

import { AccountExportScreen, loadAccountExport, useAccountExportTitle } from "@my/app-features";
import { createFileRoute, useRouter } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { ScreenHeading } from "../shell/heading";
import { SignedOut } from "../shell/signed-out";

export const Route = createFileRoute("/settings/account/export")({
  loader: ({ context }) => screenLoad(context, loadAccountExport),
  component: AccountExportRoute,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function AccountExportRoute() {
  const { data } = Route.useRouteContext();
  const router = useRouter();
  const title = useAccountExportTitle();
  return (
    <>
      <ScreenHeading>{title}</ScreenHeading>
      {data === null ? (
        <SignedOut />
      ) : (
        <AccountExportScreen
          onBack={() => {
            void router.navigate({ to: "/settings/$entry", params: { entry: "account" } });
          }}
        />
      )}
    </>
  );
}
