// SPDX-License-Identifier: AGPL-3.0-only

// A new cost item, at /admin/finance/costs/new, a screen under Finance (ADR-0067): recorded, the reader goes on to its
// screen. Without a session it asks the reader to sign in.

import { CostItemCreateScreen, loadCostItemsScreen, loadStaffEntry } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { StaffUnderPage } from "../shell/staff-under";

export const Route = createFileRoute("/admin/finance/costs/new")({
  // The newest item, whose currency a new one starts with.
  loader: ({ context }) =>
    screenLoad(context, (data) => loadStaffEntry(data, "admin", "finance", {}, (d) => loadCostItemsScreen(d, {}))),
  component: NewCost,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function NewCost() {
  const { data } = Route.useRouteContext();
  return (
    <StaffUnderPage
      area="admin"
      entry="finance"
      signedIn={data !== null}
      screen={(address) => <CostItemCreateScreen {...address} />}
    />
  );
}
