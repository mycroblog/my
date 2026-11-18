// SPDX-License-Identifier: AGPL-3.0-only

// Two-factor authentication, at /settings/security/two-factor, a screen under the settings' Security (FR-005): turning
// it on with an authenticator app, the recovery codes, turning it off. Back, in its head, goes to Security. Without a
// session it asks the reader to sign in.

import { loadSecuritySettings, SecurityTwoFactor, useSecurityTitle } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { SettingsUnder } from "../shell/settings";

export const Route = createFileRoute("/settings/security/two-factor")({
  loader: ({ context }) => screenLoad(context, loadSecuritySettings),
  component: TwoFactorScreen,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function TwoFactorScreen() {
  const { data } = Route.useRouteContext();
  return (
    <SettingsUnder
      entry="security"
      title={useSecurityTitle("twoFactor")}
      signedIn={data !== null}
      Screen={SecurityTwoFactor}
    />
  );
}
