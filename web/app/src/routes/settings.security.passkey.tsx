// SPDX-License-Identifier: AGPL-3.0-only

// Adding a passkey, at /settings/security/passkey, a screen under the settings' Security (FR-004): its name, then the
// browser's own dialog. Back, in its head, goes to Security, as the screen does once the passkey is added. Without a
// session it asks the reader to sign in.

import { loadSecuritySettings, SecurityPasskey, useSecurityTitle } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { SettingsUnder } from "../shell/settings";

export const Route = createFileRoute("/settings/security/passkey")({
  loader: ({ context }) => screenLoad(context, loadSecuritySettings),
  component: PasskeyScreen,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function PasskeyScreen() {
  const { data } = Route.useRouteContext();
  return (
    <SettingsUnder
      entry="security"
      title={useSecurityTitle("passkey")}
      signedIn={data !== null}
      Screen={SecurityPasskey}
    />
  );
}
