// SPDX-License-Identifier: AGPL-3.0-only

// A subscription to a supporter plan, at /admin/supporters/subscriptions/{id}, a screen under Supporters (ADR-0072):
// its facts, and ending it at once. Its query is the list's the reader came from, which its way back keeps. Without a
// session it asks the reader to sign in.

import { loadSubscription, SubscriptionScreen, useSubscriptionTitle, loadStaffEntry } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { StaffUnderPage } from "../shell/staff-under";

export const Route = createFileRoute("/admin/supporters/subscriptions/$id")({
  loader: ({ context, params }) =>
    screenLoad(context, (data) =>
      loadStaffEntry(data, "admin", "supporters", {}, (d) => loadSubscription(d, params.id)),
    ),
  component: Subscription,
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
      title={useSubscriptionTitle(id)}
      screen={(address) => <SubscriptionScreen id={id} {...address} />}
    />
  );
}

function Subscription() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  if (data !== null) return <Titled key={id} id={id} />;
  return <StaffUnderPage area="admin" entry="supporters" signedIn={false} screen={() => null} />;
}
