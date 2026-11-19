// SPDX-License-Identifier: AGPL-3.0-only

// A new announcement, at /admin/announcements/new: its title, its text and when it shows, kept as a draft or
// published, under the entry Announcements. Leaving it with changes that are not saved is held back until the reader
// says whether to. Without a session it asks the reader to sign in.

import { NewAnnouncementScreen } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { adminSubLoad, AdminSubPage } from "../shell/staff-server";

export const Route = createFileRoute("/admin/announcements/new")({
  loader: ({ context }) => adminSubLoad(context, "manage_announcements", () => Promise.resolve()),
  component: NewAnnouncementPage,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function NewAnnouncementPage() {
  const { data } = Route.useRouteContext();
  return (
    <AdminSubPage entry="announcements" signedIn={data !== null} returnTo="/admin/announcements/new">
      {(sub) => <NewAnnouncementScreen {...sub} />}
    </AdminSubPage>
  );
}
