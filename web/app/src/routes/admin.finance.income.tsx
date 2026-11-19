// SPDX-License-Identifier: AGPL-3.0-only

// The income, at /admin/finance/income, a screen under Finance (ADR-0067): its totals per period, its net by source,
// and the money movements, of the kind the address's query names. Without a session it asks the reader to sign in.

import { IncomeAdminScreen } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { StaffUnderPage } from "../shell/staff-under";

export const Route = createFileRoute("/admin/finance/income")({
  component: Income,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function Income() {
  const { data } = Route.useRouteContext();
  return (
    <StaffUnderPage
      area="admin"
      entry="finance"
      signedIn={data !== null}
      screen={(address) => <IncomeAdminScreen {...address} />}
    />
  );
}
