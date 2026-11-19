// SPDX-License-Identifier: AGPL-3.0-only

// A new supporter plan, at /admin/supporters/plans/new, a screen under Supporters (ADR-0072): its form, on a role of
// its own; once created, the plan's screen. Without a session it asks the reader to sign in.

import { loadPlans, PlanCreateScreen } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { StaffUnderPage } from "../shell/staff-under";

export const Route = createFileRoute("/admin/supporters/plans/new")({
  loader: ({ context }) => screenLoad(context, loadPlans),
  component: NewPlan,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function NewPlan() {
  const { data } = Route.useRouteContext();
  return (
    <StaffUnderPage
      area="admin"
      entry="supporters"
      signedIn={data !== null}
      screen={(address) => <PlanCreateScreen {...address} />}
    />
  );
}
