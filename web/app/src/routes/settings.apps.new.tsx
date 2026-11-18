// SPDX-License-Identifier: AGPL-3.0-only

// Registering an app, at /settings/apps/new, a screen under the settings' Apps (ADR-0090 §1): the form, and once the
// app is registered its client ID and its secret, shown once. Back, in its head, goes to Apps. Without a session it
// asks the reader to sign in.

import { AppRegister, useAppsTitle } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { SettingsUnder } from "../shell/settings";

export const Route = createFileRoute("/settings/apps/new")({
  component: RegisterScreen,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function RegisterScreen() {
  const { data } = Route.useRouteContext();
  return <SettingsUnder entry="apps" title={useAppsTitle("register")} signedIn={data !== null} Screen={AppRegister} />;
}
