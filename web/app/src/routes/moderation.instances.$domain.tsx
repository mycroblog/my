// SPDX-License-Identifier: AGPL-3.0-only

// A remote server, at /moderation/instances/{domain} (ADR-0058 §1): whether deliveries go to it, its availability and
// the deliveries to it. Without a session it asks the reader to sign in.

import { loadRemoteServer } from "@my/app-data";
import { loadStaffEntry } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { RemoteServerRoute } from "../shell/staff-domains";
import { StaffSignedOut } from "../shell/staff-under";

export const Route = createFileRoute("/moderation/instances/$domain")({
  loader: ({ context, params }) =>
    screenLoad(context, (data) =>
      loadStaffEntry(data, "moderation", "instances", {}, (d) => loadRemoteServer(d, params.domain)),
    ),
  component: Page,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function Page() {
  const { data } = Route.useRouteContext();
  const { domain } = Route.useParams();
  return data === null ? (
    <StaffSignedOut returnTo={`/moderation/instances/${domain}`} />
  ) : (
    <RemoteServerRoute domain={domain} />
  );
}
