// SPDX-License-Identifier: AGPL-3.0-only

// An appeal of an action, at /moderation/appeals/{id} (ADR-0058 §2), under the queue of appeals, whose query its way
// back keeps: what the user wrote, the action, and the decision while it is pending. Without a session it asks the
// reader to sign in.

import { loadModeratedAppeal } from "@my/app-data";
import { loadUnderEntry } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { AppealRoute } from "../shell/staff-accounts";
import { StaffSignedOut } from "../shell/staff-under";

export const Route = createFileRoute("/moderation/appeals/$id")({
  loader: ({ context, params }) =>
    screenLoad(context, (data) => loadUnderEntry(data, "appeals", (d) => loadModeratedAppeal(d, params.id))),
  component: Page,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function Page() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  return data === null ? <StaffSignedOut returnTo={`/moderation/appeals/${id}`} /> : <AppealRoute id={id} />;
}
