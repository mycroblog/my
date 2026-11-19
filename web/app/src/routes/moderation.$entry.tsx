// SPDX-License-Identifier: AGPL-3.0-only

// The screen of an entry of the moderators' screens, at /moderation/{entry} (ADR-0058 §2): the entries are the
// features' list (@my/app-features, staff/entries.tsx), so a new one needs no route. Its query (a queue's filters and
// page) is the screen's, which its loader reads with. An address that names no screen open to the reader says so.
// Without a session it asks the reader to sign in. A screen under an entry (a report) is a route of its own.

import { loadStaffEntry } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { searchOfQuery, StaffEntryPage } from "../shell/staff";

export const Route = createFileRoute("/moderation/$entry")({
  loaderDeps: ({ search }) => ({ search: search as Record<string, unknown> }),
  loader: ({ context, params, location }) =>
    screenLoad(context, (data) => loadStaffEntry(data, "moderation", params.entry, searchOfQuery(location.searchStr))),
  component: ModerationEntry,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function ModerationEntry() {
  const { data } = Route.useRouteContext();
  const { entry } = Route.useParams();
  return <StaffEntryPage area="moderation" entry={entry} signedIn={data !== null} />;
}
