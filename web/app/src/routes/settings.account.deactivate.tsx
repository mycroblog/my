// SPDX-License-Identifier: AGPL-3.0-only

// Deactivate account, at /settings/account/deactivate, the screen under the settings' entry Account and data
// (ADR-0110 §5) where the reader deactivates their account (FR-008): it says what that does and until when it can be
// undone, and, once done, the app leaves as after signing out, since the server ended the session. Back goes to the
// entry. Without a session it asks the reader to sign in.

import { AccountDeactivateScreen, loadAccountDeactivateSettings, useAccountDeactivateTitle } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { SettingsUnder } from "../shell/settings";

export const Route = createFileRoute("/settings/account/deactivate")({
  loader: ({ context }) => screenLoad(context, loadAccountDeactivateSettings),
  component: AccountDeactivateRoute,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function AccountDeactivateRoute() {
  const { data } = Route.useRouteContext();
  return (
    <SettingsUnder
      entry="account"
      title={useAccountDeactivateTitle()}
      signedIn={data !== null}
      Screen={AccountDeactivateScreen}
    />
  );
}
