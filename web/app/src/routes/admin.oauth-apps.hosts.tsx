// SPDX-License-Identifier: AGPL-3.0-only

// The denied hosts of app metadata documents, at /admin/oauth-apps/hosts, under the entry OAuth apps: a host to deny,
// and each denial with Lift, a page at a time, the page kept in the address. Without a session it asks the reader to
// sign in.

import { loadOAuthHosts, OAuthHostsScreen, type AdminSubProps } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { adminSubLoad, AdminSubPage, searchOfQuery } from "../shell/staff";
import { useStaffAddress } from "../shell/staff-under";

export const Route = createFileRoute("/admin/oauth-apps/hosts")({
  loaderDeps: ({ search }) => ({ search: search as Record<string, unknown> }),
  loader: ({ context, location }) =>
    adminSubLoad(context, "manage_oauth_apps", (data) => loadOAuthHosts(data, searchOfQuery(location.searchStr))),
  component: OAuthHostsPage,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function OAuthHostsPage() {
  const { data } = Route.useRouteContext();
  return (
    <AdminSubPage entry="oauth-apps" signedIn={data !== null} returnTo="/admin/oauth-apps/hosts">
      {(sub) => <Hosts {...sub} />}
    </AdminSubPage>
  );
}

/** The screen, given its address. */
function Hosts(sub: AdminSubProps) {
  const { search, onSearch } = useStaffAddress();
  return <OAuthHostsScreen {...sub} search={search} onSearch={onSearch} />;
}
