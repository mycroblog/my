// SPDX-License-Identifier: AGPL-3.0-only

// A custom emoji, at /admin/custom-emojis/{id}, under the entry Custom emoji: disabling or enabling it, what it is, a
// local one's image, its place in the picker, a form saved as a whole, and Delete; a remote one's copy to this server.
// Leaving it with changes that are not saved is held back until the reader says whether to. Without a session it asks
// the reader to sign in.

import { loadAdminEmoji } from "@my/app-data";
import { EmojiScreen } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { adminSubLoad, AdminSubPage } from "../shell/staff";

export const Route = createFileRoute("/admin/custom-emojis/$id")({
  loader: ({ context, params }) =>
    adminSubLoad(context, "manage_custom_emojis", (data) => loadAdminEmoji(data, params.id)),
  component: EmojiPage,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function EmojiPage() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  return (
    <AdminSubPage key={id} entry="custom-emojis" signedIn={data !== null} returnTo={`/admin/custom-emojis/${id}`}>
      {(sub) => <EmojiScreen id={id} {...sub} />}
    </AdminSubPage>
  );
}
