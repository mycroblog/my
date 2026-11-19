// SPDX-License-Identifier: AGPL-3.0-only

// The requests for custom emoji, at /admin/custom-emojis/requests, the second tab of the entry Custom emoji: the
// pending ones by default, each opened over the list to approve or reject it. Without a session it asks the reader
// to sign in.

import { loadAdminEmojiRequests } from "@my/app-data";
import { AdminEmojiRequestsScreen } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { adminSubLoad, AdminSubPage } from "../shell/staff";

export const Route = createFileRoute("/admin/custom-emojis/requests")({
  loader: ({ context }) =>
    adminSubLoad(context, "manage_custom_emojis", (data) => loadAdminEmojiRequests(data, "pending")),
  component: RequestsPage,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function RequestsPage() {
  const { data } = Route.useRouteContext();
  return (
    <AdminSubPage entry="custom-emojis" signedIn={data !== null} returnTo="/admin/custom-emojis/requests">
      {(sub) => <AdminEmojiRequestsScreen {...sub} />}
    </AdminSubPage>
  );
}
