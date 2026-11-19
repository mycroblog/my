// SPDX-License-Identifier: AGPL-3.0-only

// The denied hosts of app metadata documents, at /admin/oauth-apps/hosts, under the entry OAuth apps: a host to deny,
// and each denial with Lift. Without a session it asks the reader to sign in.

import { loadOAuthHosts, OAuthHostsScreen } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { adminSubLoad, AdminSubPage } from "../shell/staff";

export const Route = createFileRoute("/admin/oauth-apps/hosts")({
  loader: ({ context }) => adminSubLoad(context, "manage_oauth_apps", loadOAuthHosts),
  component: OAuthHostsPage,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function OAuthHostsPage() {
  const { data } = Route.useRouteContext();
  return (
    <AdminSubPage entry="oauth-apps" signedIn={data !== null} returnTo="/admin/oauth-apps/hosts">
      {(sub) => <OAuthHostsScreen {...sub} />}
    </AdminSubPage>
  );
}
