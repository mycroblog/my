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

/** The screen under the app's name, for a signed-in reader, whose authorized apps alone are read. */
function Titled() {
  const { id } = Route.useParams();
  return <SettingsUnder entry="apps" title={useAuthorizedAppTitle(id)} signedIn Screen={Screen} />;
}

function AuthorizedScreen() {
  const { data } = Route.useRouteContext();
  // Without a session nothing of the reader's is read: the title is the settings' own.
  return data === null ? <SettingsUnder entry="apps" title="" signedIn={false} Screen={Screen} /> : <Titled />;
}
