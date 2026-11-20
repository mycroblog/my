// SPDX-License-Identifier: AGPL-3.0-only

// The jobs of a state and a type, at /admin/jobs/list, under the entry Jobs, a page at a time; the address's query
// keeps the filters and the page. Without a session it asks the reader to sign in.

import { loadJobList, loadUnderOperations } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { JobListRoute } from "../shell/staff-operations";
import { searchOfQuery } from "../shell/staff";

export const Route = createFileRoute("/admin/jobs/list")({
  loaderDeps: ({ search }) => ({ search: search as Record<string, unknown> }),
  loader: ({ context, location }) =>
    screenLoad(context, (data) =>
      loadUnderOperations(data, "jobs", (d) => loadJobList(d, searchOfQuery(location.searchStr))),
    ),
  component: Page,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function Page() {
  const { data } = Route.useRouteContext();
  return <JobListRoute signedIn={data !== null} />;
}
