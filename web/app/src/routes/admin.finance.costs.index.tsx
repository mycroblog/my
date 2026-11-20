// SPDX-License-Identifier: AGPL-3.0-only

// The cost items, at /admin/finance/costs, a screen under Finance (ADR-0067): every item that is not deleted, newest
// first, a page at a time, and the way to record one. Without a session it asks the reader to sign in.

import { CostItemsScreen, loadCostItemsScreen, loadStaffEntry } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { searchOfQuery } from "../shell/staff";
import { StaffUnderPage } from "../shell/staff-under";

export const Route = createFileRoute("/admin/finance/costs/")({
  loaderDeps: ({ search }) => ({ search: search as Record<string, unknown> }),
  loader: ({ context, location }) =>
    screenLoad(context, (data) =>
      loadStaffEntry(data, "admin", "finance", {}, (d) => loadCostItemsScreen(d, searchOfQuery(location.searchStr))),
    ),
  component: Costs,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function Costs() {
  const { data } = Route.useRouteContext();
  return (
    <StaffUnderPage
      area="admin"
      entry="finance"
      signedIn={data !== null}
      screen={(address) => <CostItemsScreen {...address} />}
    />
  );
}
