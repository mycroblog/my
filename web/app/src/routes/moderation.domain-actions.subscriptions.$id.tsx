// SPDX-License-Identifier: AGPL-3.0-only

// A subscription to a domain block list, at /moderation/domain-actions/subscriptions/{id} (ADR-0159 §6): what it is,
// its drafts, its fetches, its settings, and Stop subscribing. Without a session it asks the reader to sign in.

import { loadDomainListSubscription } from "@my/app-data";
import { loadStaffEntry } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { DomainListRoute } from "../shell/staff-domains";
import { StaffSignedOut } from "../shell/staff-under";

export const Route = createFileRoute("/moderation/domain-actions/subscriptions/$id")({
  loader: ({ context, params }) =>
    screenLoad(context, (data) =>
      loadStaffEntry(data, "moderation", "domain-actions", {}, (d) => loadDomainListSubscription(d, params.id)),
    ),
  component: Page,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function Page() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  return data === null ? (
    <StaffSignedOut returnTo={`/moderation/domain-actions/subscriptions/${id}`} />
  ) : (
    <DomainListRoute id={id} />
  );
}
