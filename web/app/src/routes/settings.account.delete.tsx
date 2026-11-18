// SPDX-License-Identifier: AGPL-3.0-only

// Delete account, at /settings/account/delete, the screen under the settings' entry Account and data (ADR-0110 §5)
// where the reader deletes their account at once (FR-008): it says what that does, and, once done, the app leaves as
// after signing out, since the server ended the session. Back goes to the entry. Without a session it asks the
// reader to sign in.

import { AccountDeleteScreen, loadAccountDeleteSettings, useAccountDeleteTitle } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { SettingsUnder } from "../shell/settings";

export const Route = createFileRoute("/settings/account/delete")({
  loader: ({ context }) => screenLoad(context, loadAccountDeleteSettings),
  component: AccountDeleteRoute,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function AccountDeleteRoute() {
  const { data } = Route.useRouteContext();
  return (
    <SettingsUnder
      entry="account"
      title={useAccountDeleteTitle()}
      signedIn={data !== null}
      Screen={AccountDeleteScreen}
    />
  );
}
