// SPDX-License-Identifier: AGPL-3.0-only

// An announcement, at /admin/announcements/{id}: where it stands, its title, text and window as a form saved as a
// whole, Publish for a draft, what readers see, and Withdraw, under the entry Announcements; a withdrawn one's facts.
// Leaving it with changes that are not saved is held back until the reader says whether to. Without a session it asks
// the reader to sign in.

import { loadAdminAnnouncement } from "@my/app-data";
import { AnnouncementScreen } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { adminSubLoad, AdminSubPage } from "../shell/staff";

export const Route = createFileRoute("/admin/announcements/$id")({
  loader: ({ context, params }) =>
    adminSubLoad(context, "manage_announcements", (data) => loadAdminAnnouncement(data, params.id)),
  component: AnnouncementPage,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function AnnouncementPage() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  return (
    <AdminSubPage key={id} entry="announcements" signedIn={data !== null} returnTo={`/admin/announcements/${id}`}>
      {(sub) => <AnnouncementScreen id={id} {...sub} />}
    </AdminSubPage>
  );
}
