// SPDX-License-Identifier: AGPL-3.0-only

// Issuing a token of an app the reader registered, at /settings/apps/{id}/token, a screen under that app's (ADR-0090
// §2, §17): the scopes, and once the token is issued the access token and the refresh token, shown once. Back, in its
// head, goes to the app's screen. Without a session it asks the reader to sign in.

import { appsPaths, AppToken, loadOwnApp, useAppsTitle, type SettingsSubProps } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { SettingsUnder } from "../shell/settings";

export const Route = createFileRoute("/settings/apps/$id_/token")({
  loader: ({ context, params }) => screenLoad(context, (data) => loadOwnApp(data, params.id)),
  component: TokenScreen,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

/** The screen for the app the address names. */
function Screen(props: SettingsSubProps) {
  const { id } = Route.useParams();
  return <AppToken key={id} id={id} {...props} />;
}

function TokenScreen() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  return (
    <SettingsUnder
      entry="apps"
      under={appsPaths.app(id)}
      title={useAppsTitle("token")}
      signedIn={data !== null}
      Screen={Screen}
    />
  );
}
