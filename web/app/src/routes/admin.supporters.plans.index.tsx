// SPDX-License-Identifier: AGPL-3.0-only

// The supporter plans, at /admin/supporters/plans, a screen under Supporters (ADR-0072): every plan, the retired ones
// too, and the way to create one. Without a session it asks the reader to sign in.

import { loadPlans, PlanListScreen, loadStaffEntry } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { StaffUnderPage } from "../shell/staff-under";

export const Route = createFileRoute("/admin/supporters/plans/")({
  loader: ({ context }) => screenLoad(context, (data) => loadStaffEntry(data, "admin", "supporters", {}, loadPlans)),
  component: Plans,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function Plans() {
  const { data } = Route.useRouteContext();
  return (
    <StaffUnderPage
      area="admin"
      entry="supporters"
      signedIn={data !== null}
      screen={(address) => <PlanListScreen {...address} />}
    />
  );
}
