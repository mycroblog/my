// SPDX-License-Identifier: AGPL-3.0-only

// The announcement of the closure, at /admin/closure/announcement (ADR-0088): announcing it, or changing the dates of
// the one in effect, a form saved as a whole under the entry Server closure; back goes to the entry. Leaving it with
// changes that are not saved is held back until the reader says whether to. Without a session it asks the reader to
// sign in.

import { ClosureAnnouncementScreen, loadClosure, useClosureAnnouncementTitle } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { adminSubLoad, AdminSubHeading, AdminSubPage } from "../shell/staff";

export const Route = createFileRoute("/admin/closure/announcement")({
  loader: ({ context }) => adminSubLoad(context, "manage_service_closure", loadClosure),
  component: ClosureAnnouncementPage,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function ClosureAnnouncementPage() {
  const { data } = Route.useRouteContext();
  return (
    <AdminSubPage entry="closure" signedIn={data !== null} returnTo="/admin/closure/announcement" heading={<Heading />}>
      {(sub) => <ClosureAnnouncementScreen {...sub} />}
    </AdminSubPage>
  );
}

/** The heading, which names what the screen does once the closure is read. */
function Heading() {
  return <AdminSubHeading entry="closure" title={useClosureAnnouncementTitle()} />;
}
