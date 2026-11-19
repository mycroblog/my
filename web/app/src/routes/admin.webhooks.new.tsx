// SPDX-License-Identifier: AGPL-3.0-only

// A new webhook, at /admin/webhooks/new: its address, what it is for and its events, under the entry Webhooks; once
// set up, its secret, shown once. Leaving it with changes that are not saved is held back until the reader says
// whether to. Without a session it asks the reader to sign in.

import { NewWebhookScreen } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { adminSubLoad, AdminSubPage } from "../shell/staff-server";

export const Route = createFileRoute("/admin/webhooks/new")({
  loader: ({ context }) => adminSubLoad(context, "manage_webhooks", () => Promise.resolve()),
  component: NewWebhookPage,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function NewWebhookPage() {
  const { data } = Route.useRouteContext();
  return (
    <AdminSubPage entry="webhooks" signedIn={data !== null} returnTo="/admin/webhooks/new">
      {(sub) => <NewWebhookScreen {...sub} />}
    </AdminSubPage>
  );
}
