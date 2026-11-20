// SPDX-License-Identifier: AGPL-3.0-only

// A domain action, at /moderation/domain-actions/{id} (ADR-0058 §1): all it holds, and Lift while it is in effect.
// Without a session it asks the reader to sign in.

import { loadDomainAction } from "@my/app-data";
import { loadStaffEntry } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { DomainActionRoute } from "../shell/staff-domains";
import { StaffSignedOut } from "../shell/staff-under";

export const Route = createFileRoute("/moderation/domain-actions/$id")({
  loader: ({ context, params }) =>
    screenLoad(context, (data) =>
      loadStaffEntry(data, "moderation", "domain-actions", {}, (d) => loadDomainAction(d, params.id)),
    ),
  component: Page,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function Page() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  return data === null ? (
    <StaffSignedOut returnTo={`/moderation/domain-actions/${id}`} />
  ) : (
    <DomainActionRoute id={id} />
  );
}
