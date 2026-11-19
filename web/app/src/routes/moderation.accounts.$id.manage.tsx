// SPDX-License-Identifier: AGPL-3.0-only

// An account's Management page, at /moderation/accounts/{id}/manage (ADR-0058 §2): its email and IP addresses on
// demand, its email delivery, its sign-in recovery, its roles, or a remote account's posts fetched again. Without a session it asks
// the reader to sign in.

import { loadAccountRoles, loadModeratedAccount } from "@my/app-data";
import { loadUnderEntry } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { AccountRoute } from "../shell/staff-accounts";
import { StaffSignedOut } from "../shell/staff-under";

export const Route = createFileRoute("/moderation/accounts/$id/manage")({
  loader: ({ context, params }) =>
    screenLoad(context, (data) =>
      loadUnderEntry(data, "accounts", async (d) => {
        await Promise.all([loadModeratedAccount(d, params.id), loadAccountRoles(d, params.id)]);
      }),
    ),
  component: Page,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function Page() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  return data === null ? (
    <StaffSignedOut returnTo={`/moderation/accounts/${id}/manage`} />
  ) : (
    <AccountRoute id={id} page="manage" />
  );
}
