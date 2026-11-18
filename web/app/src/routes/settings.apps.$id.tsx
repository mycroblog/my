// SPDX-License-Identifier: AGPL-3.0-only

// An app the reader registered, at /settings/apps/{id}, a screen under the settings' Apps (ADR-0090 §1): its client
// ID, a new secret, the way to a token of it, its settings and deleting it. Back, in its head, goes to Apps. Without a
// session it asks the reader to sign in. The screen that issues its token (settings.apps.$id_.token.tsx) is a route
// of its own.

import { loadOwnApp, OwnApp, useOwnAppTitle, type SettingsSubProps } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { SettingsUnder } from "../shell/settings";

export const Route = createFileRoute("/settings/apps/$id")({
  loader: ({ context, params }) => screenLoad(context, (data) => loadOwnApp(data, params.id)),
  component: AppScreen,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

/** The screen of the app the address names. */
function Screen(props: SettingsSubProps) {
  const { id } = Route.useParams();
  return <OwnApp key={id} id={id} {...props} />;
}

function AppScreen() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  return <SettingsUnder entry="apps" title={useOwnAppTitle(id)} signedIn={data !== null} Screen={Screen} />;
}
