// SPDX-License-Identifier: AGPL-3.0-only

// The password, at /settings/security/password, a screen under the settings' Security (FR-002): changing it, setting
// one, removing it. Back, in its head, goes to Security. Without a session it asks the reader to sign in.

import { loadSecuritySettings, SecurityPassword, useSecurityTitle } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { SettingsUnder } from "../shell/settings";

export const Route = createFileRoute("/settings/security/password")({
  loader: ({ context }) => screenLoad(context, loadSecuritySettings),
  component: PasswordScreen,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function PasswordScreen() {
  const { data } = Route.useRouteContext();
  return (
    <SettingsUnder
      entry="security"
      title={useSecurityTitle("password")}
      signedIn={data !== null}
      Screen={SecurityPassword}
    />
  );
}
