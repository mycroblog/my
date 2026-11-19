// SPDX-License-Identifier: AGPL-3.0-only

// A new relay, at /admin/relays/new: its URL, kind and whether it receives, under the entry Relays; once joined, the
// relay's page. Leaving it with changes that are not saved is held back until the reader says whether to. Without a
// session it asks the reader to sign in.

import { loadRelays, NewRelayScreen } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { adminSubLoad, AdminSubPage } from "../shell/staff";

export const Route = createFileRoute("/admin/relays/new")({
  loader: ({ context }) => adminSubLoad(context, "manage_settings", loadRelays),
  component: NewRelayPage,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function NewRelayPage() {
  const { data } = Route.useRouteContext();
  return (
    <AdminSubPage entry="relays" signedIn={data !== null} returnTo="/admin/relays/new">
      {(sub) => <NewRelayScreen {...sub} />}
    </AdminSubPage>
  );
}
