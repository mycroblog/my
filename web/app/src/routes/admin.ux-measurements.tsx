// SPDX-License-Identifier: AGPL-3.0-only

// The UX measurements, at /admin/ux-measurements: the entry's screen, over the window its query names, given a route
// of its own (over /admin/$entry) so that leaving its form with changes not saved is held back until the reader says
// whether to, as on every other form of the administration. Without a session it asks the reader to sign in.

import { loadUnderOperations, loadUXScreen } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { UXMeasurementsRoute } from "../shell/staff-operations";
import { searchOfQuery } from "../shell/staff";

export const Route = createFileRoute("/admin/ux-measurements")({
  loaderDeps: ({ search }) => ({ search: search as Record<string, unknown> }),
  loader: ({ context, location }) =>
    screenLoad(context, (data) =>
      loadUnderOperations(data, "ux-measurements", (d) => loadUXScreen(d, searchOfQuery(location.searchStr))),
    ),
  component: Page,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function Page() {
  const { data } = Route.useRouteContext();
  return <UXMeasurementsRoute signedIn={data !== null} />;
}
