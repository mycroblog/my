// SPDX-License-Identifier: AGPL-3.0-only

// The requests for custom emoji, at /admin/custom-emojis/requests, the second tab of the entry Custom emoji: the
// pending ones by default, each opened over the list to approve or reject it, the state shown and the page kept in the
// address. Without a session it asks the reader to sign in.

import { AdminEmojiRequestsScreen, loadEmojiRequestsScreen, type AdminSubProps } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { adminSubLoad, AdminSubPage, searchOfQuery } from "../shell/staff";
import { useStaffAddress } from "../shell/staff-under";

export const Route = createFileRoute("/admin/custom-emojis/requests")({
  loaderDeps: ({ search }) => ({ search: search as Record<string, unknown> }),
  loader: ({ context, location }) =>
    adminSubLoad(context, "manage_custom_emojis", (data) =>
      loadEmojiRequestsScreen(data, searchOfQuery(location.searchStr)),
    ),
  component: RequestsPage,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function RequestsPage() {
  const { data } = Route.useRouteContext();
  return (
    <AdminSubPage entry="custom-emojis" signedIn={data !== null} returnTo="/admin/custom-emojis/requests">
      {(sub) => <Requests {...sub} />}
    </AdminSubPage>
  );
}

/** The screen, given its address. */
function Requests(sub: AdminSubProps) {
  const { search, onSearch } = useStaffAddress();
  return <AdminEmojiRequestsScreen {...sub} search={search} onSearch={onSearch} />;
}
