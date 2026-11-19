// SPDX-License-Identifier: AGPL-3.0-only

// The screen of an entry of the server's administration, at /admin/{entry} (ADR-0058 §2): the entries are the
// features' list (@my/app-features, staff/entries.tsx), so a new one needs no route. Its query is the screen's, which
// its loader reads with. An address that names no screen open to the reader says so. Without a session it asks the
// reader to sign in.

import { loadStaffEntry } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { searchOfQuery, StaffEntryPage } from "../shell/staff";

export const Route = createFileRoute("/admin/$entry")({
  loaderDeps: ({ search }) => ({ search: search as Record<string, unknown> }),
  loader: ({ context, params, location }) =>
    screenLoad(context, (data) => loadStaffEntry(data, "admin", params.entry, searchOfQuery(location.searchStr))),
  component: AdminEntry,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function AdminEntry() {
  const { data } = Route.useRouteContext();
  const { entry } = Route.useParams();
  return <StaffEntryPage area="admin" entry={entry} signedIn={data !== null} />;
}
