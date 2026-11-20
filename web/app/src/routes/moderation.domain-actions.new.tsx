// SPDX-License-Identifier: AGPL-3.0-only

// The form of a new domain action, at /moderation/domain-actions/new (ADR-0058 §1), on the domain its query names,
// which opens the action once taken. Without a session it asks the reader to sign in.

import { createFileRoute } from "@tanstack/react-router";
import { NewDomainActionRoute } from "../shell/staff-domains";
import { StaffSignedOut } from "../shell/staff-under";

export const Route = createFileRoute("/moderation/domain-actions/new")({
  component: Page,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function Page() {
  const { data } = Route.useRouteContext();
  return data === null ? <StaffSignedOut returnTo={"/moderation/domain-actions/new"} /> : <NewDomainActionRoute />;
}
