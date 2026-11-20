// SPDX-License-Identifier: AGPL-3.0-only

// A job, at /admin/jobs/{id}, under the entry Jobs: its facts, its last error, its payload and its attempts, and Run
// again for a failed one. Without a session it asks the reader to sign in.

import { loadJob, loadUnderOperations } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { JobRoute } from "../shell/staff-operations";

export const Route = createFileRoute("/admin/jobs/$id")({
  loader: ({ context, params }) =>
    screenLoad(context, (data) => loadUnderOperations(data, "jobs", (d) => loadJob(d, params.id))),
  component: Page,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function Page() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  return <JobRoute id={id} signedIn={data !== null} />;
}
