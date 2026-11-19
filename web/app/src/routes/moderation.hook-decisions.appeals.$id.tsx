// SPDX-License-Identifier: AGPL-3.0-only

// An appeal of a refused post, at /moderation/hook-decisions/appeals/{id} (ADR-0058 §2, ADR-0147): the post its author
// tried to send, their comment, and the decision while it is pending. Without a session it asks the reader to sign in.

import { loadModeratedHookAppeal } from "@my/app-data";
import { loadUnderEntry } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { HookAppealRoute } from "../shell/staff-accounts";
import { StaffSignedOut } from "../shell/staff-under";

export const Route = createFileRoute("/moderation/hook-decisions/appeals/$id")({
  loader: ({ context, params }) =>
    screenLoad(context, (data) => loadUnderEntry(data, "hook-decisions", (d) => loadModeratedHookAppeal(d, params.id))),
  component: Page,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function Page() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  return data === null ? (
    <StaffSignedOut returnTo={`/moderation/hook-decisions/appeals/${id}`} />
  ) : (
    <HookAppealRoute id={id} />
  );
}
