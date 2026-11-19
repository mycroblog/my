// SPDX-License-Identifier: AGPL-3.0-only

// A relay, at /admin/relays/{id}: where its membership stands and what can be done now, under the entry Relays.
// Without a session it asks the reader to sign in.

import { loadRelays, RelayScreen, useRelayTitle } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { adminSubLoad, AdminSubHeading, AdminSubPage } from "../shell/staff";

export const Route = createFileRoute("/admin/relays/$id")({
  loader: ({ context }) => adminSubLoad(context, "manage_settings", loadRelays),
  component: RelayPage,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function RelayPage() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  return (
    <AdminSubPage
      key={id}
      entry="relays"
      signedIn={data !== null}
      returnTo={`/admin/relays/${id}`}
      heading={<Heading />}
    >
      {(sub) => <RelayScreen id={id} {...sub} />}
    </AdminSubPage>
  );
}

/** The heading, which names what the screen shows once it is read. */
function Heading() {
  const { id } = Route.useParams();
  return <AdminSubHeading entry="relays" title={useRelayTitle(id)} />;
}
