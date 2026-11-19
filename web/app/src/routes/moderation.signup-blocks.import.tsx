// SPDX-License-Identifier: AGPL-3.0-only

// The form that adds sign-up blocks from a list, at /moderation/signup-blocks/import (ADR-0058 §2), which then says
// what it added and each line it skipped. Without a session it asks the reader to sign in.

import { createFileRoute } from "@tanstack/react-router";
import { SignupBlockFormRoute } from "../shell/staff-accounts";
import { StaffSignedOut } from "../shell/staff-under";

export const Route = createFileRoute("/moderation/signup-blocks/import")({
  component: Page,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function Page() {
  const { data } = Route.useRouteContext();
  return data === null ? (
    <StaffSignedOut returnTo="/moderation/signup-blocks/import" />
  ) : (
    <SignupBlockFormRoute form="import" />
  );
}
