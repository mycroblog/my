// SPDX-License-Identifier: AGPL-3.0-only

// Mastodon's domain block CSV, at /moderation/domain-actions/import (ADR-0058 §1): its export, and its import after a
// preview. Without a session it asks the reader to sign in.

import { createFileRoute } from "@tanstack/react-router";
import { DomainImportRoute } from "../shell/staff-domains";
import { StaffSignedOut } from "../shell/staff-under";

export const Route = createFileRoute("/moderation/domain-actions/import")({
  component: Page,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function Page() {
  const { data } = Route.useRouteContext();
  return data === null ? <StaffSignedOut returnTo={"/moderation/domain-actions/import"} /> : <DomainImportRoute />;
}
