// SPDX-License-Identifier: AGPL-3.0-only

// The subscriptions to domain block lists, at /moderation/domain-actions/subscriptions (ADR-0159 §6), a page at a time.
// Without a session it asks the reader to sign in.

import { loadDomainListsScreen, loadStaffEntry } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { searchOfQuery } from "../shell/staff";
import { DomainListsRoute } from "../shell/staff-domains";
import { StaffSignedOut } from "../shell/staff-under";

export const Route = createFileRoute("/moderation/domain-actions/subscriptions/")({
  loaderDeps: ({ search }) => ({ search: search as Record<string, unknown> }),
  loader: ({ context, location }) =>
    screenLoad(context, (data) =>
      loadStaffEntry(data, "moderation", "domain-actions", {}, (d) =>
        loadDomainListsScreen(d, searchOfQuery(location.searchStr)),
      ),
    ),
  component: Page,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function Page() {
  const { data } = Route.useRouteContext();
  return data === null ? (
    <StaffSignedOut returnTo={"/moderation/domain-actions/subscriptions"} />
  ) : (
    <DomainListsRoute />
  );
}
