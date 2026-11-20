// SPDX-License-Identifier: AGPL-3.0-only

// The allowlist of federation, at /moderation/domain-actions/allowed (ADR-0159 §6). Without a session it asks the
// reader to sign in.

import { loadDomainAllows } from "@my/app-data";
import { loadStaffEntry } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { DomainAllowsRoute } from "../shell/staff-domains";
import { StaffSignedOut } from "../shell/staff-under";

export const Route = createFileRoute("/moderation/domain-actions/allowed")({
  loader: ({ context }) =>
    screenLoad(context, (data) => loadStaffEntry(data, "moderation", "domain-actions", {}, (d) => loadDomainAllows(d))),
  component: Page,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function Page() {
  const { data } = Route.useRouteContext();
  return data === null ? <StaffSignedOut returnTo={"/moderation/domain-actions/allowed"} /> : <DomainAllowsRoute />;
}
