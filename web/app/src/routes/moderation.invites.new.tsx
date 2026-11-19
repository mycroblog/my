// SPDX-License-Identifier: AGPL-3.0-only

// The form of a new invite of the staff, at /moderation/invites/new (ADR-0058 §2), which then shows its link and its
// code. Without a session it asks the reader to sign in.

import { createFileRoute } from "@tanstack/react-router";
import { NewInviteRoute } from "../shell/staff-accounts";
import { StaffSignedOut } from "../shell/staff-under";

export const Route = createFileRoute("/moderation/invites/new")({
  component: Page,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function Page() {
  const { data } = Route.useRouteContext();
  return data === null ? <StaffSignedOut returnTo="/moderation/invites/new" /> : <NewInviteRoute />;
}
