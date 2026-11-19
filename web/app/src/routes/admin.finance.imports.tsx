// SPDX-License-Identifier: AGPL-3.0-only

// The billing imports, at /admin/finance/imports, a screen under Finance (ADR-0068): each source's state, its billing
// months, and importing now. Without a session it asks the reader to sign in.

import { BillingImportsScreen, loadBillingImportsScreen } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { StaffUnderPage } from "../shell/staff-under";

export const Route = createFileRoute("/admin/finance/imports")({
  loader: ({ context }) => screenLoad(context, loadBillingImportsScreen),
  component: Imports,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function Imports() {
  const { data } = Route.useRouteContext();
  return (
    <StaffUnderPage
      area="admin"
      entry="finance"
      signedIn={data !== null}
      screen={(address) => <BillingImportsScreen {...address} />}
    />
  );
}
