// SPDX-License-Identifier: AGPL-3.0-only

// The check before closing the server, and the closure, at /admin/closure/close (ADR-0088), under the entry Server
// closure; back goes to the entry. Without a session it asks the reader to sign in.

import { ClosureCloseScreen, loadClosureClose, useClosureCloseTitle } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { adminSubLoad, AdminSubHeading, AdminSubPage } from "../shell/staff";

export const Route = createFileRoute("/admin/closure/close")({
  loader: ({ context }) => adminSubLoad(context, "manage_service_closure", loadClosureClose),
  component: ClosureClosePage,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function ClosureClosePage() {
  const { data } = Route.useRouteContext();
  return (
    <AdminSubPage entry="closure" signedIn={data !== null} returnTo="/admin/closure/close" heading={<Heading />}>
      {(sub) => <ClosureCloseScreen {...sub} />}
    </AdminSubPage>
  );
}

function Heading() {
  return <AdminSubHeading entry="closure" title={useClosureCloseTitle()} />;
}
