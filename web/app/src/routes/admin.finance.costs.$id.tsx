// SPDX-License-Identifier: AGPL-3.0-only

// A cost item, at /admin/finance/costs/{id}, a screen under Finance (ADR-0067): its facts, its prices each from its
// date, what names it, its end, and deleting it. Its title is the item's name. Without a session it asks the reader to
// sign in.

import { CostItemScreen, loadCostItemScreen, useCostItemTitle, loadStaffEntry } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { StaffUnderPage } from "../shell/staff-under";

export const Route = createFileRoute("/admin/finance/costs/$id")({
  loader: ({ context, params }) =>
    screenLoad(context, (data) =>
      loadStaffEntry(data, "admin", "finance", {}, (d) => loadCostItemScreen(d, params.id)),
    ),
  component: Cost,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

/** The screen of the thing the address names, under its name, for a signed-in reader. */
function Titled({ id }: { id: string }) {
  return (
    <StaffUnderPage
      area="admin"
      entry="finance"
      signedIn
      title={useCostItemTitle(id)}
      screen={(address) => <CostItemScreen id={id} {...address} />}
    />
  );
}

function Cost() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  if (data !== null) return <Titled key={id} id={id} />;
  return <StaffUnderPage area="admin" entry="finance" signedIn={false} screen={() => null} />;
}
