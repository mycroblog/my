// SPDX-License-Identifier: AGPL-3.0-only

// A supporter plan, at /admin/supporters/plans/{id}, a screen under Supporters (ADR-0072): its facts, the change of
// it and of its role, and retiring it. Its title is the plan's name. Without a session it asks the reader to sign in.

import { loadPlans, PlanScreen, usePlanTitle } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { StaffUnderPage } from "../shell/staff-under";

export const Route = createFileRoute("/admin/supporters/plans/$id")({
  loader: ({ context }) => screenLoad(context, loadPlans),
  component: Plan,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

/** The screen of the thing the address names, under its name, for a signed-in reader. */
function Titled({ id }: { id: string }) {
  return (
    <StaffUnderPage
      area="admin"
      entry="supporters"
      signedIn
      title={usePlanTitle(id)}
      screen={(address) => <PlanScreen id={id} {...address} />}
    />
  );
}

function Plan() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  if (data !== null) return <Titled key={id} id={id} />;
  return <StaffUnderPage area="admin" entry="supporters" signedIn={false} screen={() => null} />;
}
