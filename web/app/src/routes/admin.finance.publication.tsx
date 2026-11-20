// SPDX-License-Identifier: AGPL-3.0-only

// What the server publishes of its finance, at /admin/finance/publication, a screen under Finance (ADR-0067): the
// settings of what is published, or what is published now for a reader who may not change them. Without a session it
// asks the reader to sign in.

import { FinancePublicationScreen, loadFinancePublication, loadStaffEntry } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { StaffUnderPage } from "../shell/staff-under";

export const Route = createFileRoute("/admin/finance/publication")({
  loader: ({ context }) =>
    screenLoad(context, (data) => loadStaffEntry(data, "admin", "finance", {}, loadFinancePublication)),
  component: Publication,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function Publication() {
  const { data } = Route.useRouteContext();
  return (
    <StaffUnderPage
      area="admin"
      entry="finance"
      signedIn={data !== null}
      screen={(address) => <FinancePublicationScreen {...address} />}
    />
  );
}
