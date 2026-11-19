// SPDX-License-Identifier: AGPL-3.0-only

// A decision of the automated reviews, at /moderation/hook-decisions/{id} (ADR-0058 §2, ADR-0147): all it holds, and
// Cancel where it can be cancelled. Without a session it asks the reader to sign in.

import { loadHookDecision } from "@my/app-data";
import { loadUnderEntry } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { HookDecisionRoute } from "../shell/staff-accounts";
import { StaffSignedOut } from "../shell/staff-under";

export const Route = createFileRoute("/moderation/hook-decisions/$id")({
  loader: ({ context, params }) =>
    screenLoad(context, (data) => loadUnderEntry(data, "hook-decisions", (d) => loadHookDecision(d, params.id))),
  component: Page,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function Page() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  return data === null ? (
    <StaffSignedOut returnTo={`/moderation/hook-decisions/${id}`} />
  ) : (
    <HookDecisionRoute id={id} />
  );
}
