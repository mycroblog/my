// SPDX-License-Identifier: AGPL-3.0-only

// An app the account authorized, at /settings/apps/authorized/{id}, a screen under the settings' Apps (FR-003,
// ADR-0090 §5): where it comes from, what it was granted, when it was authorized and last used, and revoking it. Back,
// in its head, goes to Apps. Without a session it asks the reader to sign in.

import {
  AuthorizedAppScreen,
  loadAppsSettings,
  useAuthorizedAppTitle,
  type SettingsSessionProps,
  type SettingsSubProps,
} from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { SettingsUnder } from "../shell/settings";

export const Route = createFileRoute("/settings/apps/authorized/$id")({
  loader: ({ context }) => screenLoad(context, loadAppsSettings),
  component: AuthorizedScreen,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

/** The screen of the app the address names. */
function Screen(props: SettingsSubProps & SettingsSessionProps) {
  const { id } = Route.useParams();
  return <AuthorizedAppScreen key={id} id={id} {...props} />;
}

function AuthorizedScreen() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  return <SettingsUnder entry="apps" title={useAuthorizedAppTitle(id)} signedIn={data !== null} Screen={Screen} />;
}
