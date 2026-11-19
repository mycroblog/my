// SPDX-License-Identifier: AGPL-3.0-only

// The form of a new subscription to a domain block list, at /moderation/domain-actions/subscriptions/new (ADR-0159 §6),
// which opens the subscription once made. Without a session it asks the reader to sign in.

import { createFileRoute } from "@tanstack/react-router";
import { NewDomainListRoute } from "../shell/staff-domains";
import { StaffSignedOut } from "../shell/staff-under";

export const Route = createFileRoute("/moderation/domain-actions/subscriptions/new")({
  component: Page,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function Page() {
  const { data } = Route.useRouteContext();
  return data === null ? (
    <StaffSignedOut returnTo={"/moderation/domain-actions/subscriptions/new"} />
  ) : (
    <NewDomainListRoute />
  );
}
