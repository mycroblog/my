// SPDX-License-Identifier: AGPL-3.0-only

// The form of a new sign-up block, at /moderation/signup-blocks/new (ADR-0058 §2), which opens the block once added.
// Without a session it asks the reader to sign in.

import { createFileRoute } from "@tanstack/react-router";
import { SignupBlockFormRoute } from "../shell/staff-accounts";
import { StaffSignedOut } from "../shell/staff-under";

export const Route = createFileRoute("/moderation/signup-blocks/new")({
  component: Page,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function Page() {
  const { data } = Route.useRouteContext();
  return data === null ? (
    <StaffSignedOut returnTo="/moderation/signup-blocks/new" />
  ) : (
    <SignupBlockFormRoute form="new" />
  );
}
