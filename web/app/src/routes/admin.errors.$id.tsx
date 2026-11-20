// SPDX-License-Identifier: AGPL-3.0-only

// An error group, at /admin/errors/{id}, under the entry Errors, whose query (the list's filters and page) its way back
// keeps: resolving, muting and unmuting it, its facts, its latest occurrence and its history. Without a session it asks
// the reader to sign in.

import { loadError, loadUnderOperations } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { ErrorGroupRoute } from "../shell/staff-operations";

export const Route = createFileRoute("/admin/errors/$id")({
  loader: ({ context, params }) =>
    screenLoad(context, (data) => loadUnderOperations(data, "errors", (d) => loadError(d, params.id))),
  component: Page,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function Page() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  return <ErrorGroupRoute id={id} signedIn={data !== null} />;
}
