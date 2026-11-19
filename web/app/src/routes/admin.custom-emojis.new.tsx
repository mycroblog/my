// SPDX-License-Identifier: AGPL-3.0-only

// A new local custom emoji, at /admin/custom-emojis/new: its shortcode, its image and its place in the picker, under
// the entry Custom emoji. Leaving it with changes that are not saved is held back until the reader says whether to.
// Without a session it asks the reader to sign in.

import { NewEmojiScreen } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { adminSubLoad, AdminSubPage } from "../shell/staff-server";

export const Route = createFileRoute("/admin/custom-emojis/new")({
  loader: ({ context }) => adminSubLoad(context, "manage_custom_emojis", () => Promise.resolve()),
  component: NewEmojiPage,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function NewEmojiPage() {
  const { data } = Route.useRouteContext();
  return (
    <AdminSubPage entry="custom-emojis" signedIn={data !== null} returnTo="/admin/custom-emojis/new">
      {(sub) => <NewEmojiScreen {...sub} />}
    </AdminSubPage>
  );
}
