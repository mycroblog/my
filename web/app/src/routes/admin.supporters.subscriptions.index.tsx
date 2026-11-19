// SPDX-License-Identifier: AGPL-3.0-only

// The subscriptions to the supporter plans, at /admin/supporters/subscriptions, a screen under Supporters (ADR-0072):
// filtered by plan and state, which the address's query keeps. Without a session it asks the reader to sign in.

import { loadSubscriptionList, SubscriptionListScreen } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { searchOfQuery } from "../shell/staff";
import { StaffUnderPage } from "../shell/staff-under";

export const Route = createFileRoute("/admin/supporters/subscriptions/")({
  loaderDeps: ({ search }) => ({ search: search as Record<string, unknown> }),
  loader: ({ context, location }) =>
    screenLoad(context, (data) => loadSubscriptionList(data, searchOfQuery(location.searchStr))),
  component: Subscriptions,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function Subscriptions() {
  const { data } = Route.useRouteContext();
  return (
    <StaffUnderPage
      area="admin"
      entry="supporters"
      signedIn={data !== null}
      screen={(address) => <SubscriptionListScreen {...address} />}
    />
  );
}
