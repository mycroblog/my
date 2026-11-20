// SPDX-License-Identifier: AGPL-3.0-only

// A webhook, at /admin/webhooks/{id}: where it stands with Send a test and Turn off or on, its form saved as a whole,
// its deliveries with Send again, a new secret, and Delete, under the entry Webhooks. Leaving it with changes that
// are not saved is held back until the reader says whether to. Without a session it asks the reader to sign in.

import { loadAdminWebhook } from "@my/app-data";
import { WebhookScreen } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { adminSubLoad, AdminSubPage } from "../shell/staff";

export const Route = createFileRoute("/admin/webhooks/$id")({
  loader: ({ context, params }) =>
    adminSubLoad(context, "manage_webhooks", (data) => loadAdminWebhook(data, params.id)),
  component: WebhookPage,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function WebhookPage() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  return (
    <AdminSubPage key={id} entry="webhooks" signedIn={data !== null} returnTo={`/admin/webhooks/${id}`}>
      {(sub) => <WebhookScreen id={id} {...sub} />}
    </AdminSubPage>
  );
}
