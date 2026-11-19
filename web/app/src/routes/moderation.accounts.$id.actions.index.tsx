// SPDX-License-Identifier: AGPL-3.0-only

// An account's Actions page, at /moderation/accounts/{id}/actions (ADR-0058 §2): every action on it, a page at a time,
// or those in effect. Without a session it asks the reader to sign in.

import { loadModeratedAccount } from "@my/app-data";
import { loadUnderEntry } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { AccountRoute } from "../shell/staff-accounts";
import { StaffSignedOut } from "../shell/staff-under";

export const Route = createFileRoute("/moderation/accounts/$id/actions/")({
  loader: ({ context, params }) =>
    screenLoad(context, (data) => loadUnderEntry(data, "accounts", (d) => loadModeratedAccount(d, params.id))),
  component: Page,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function Page() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  return data === null ? (
    <StaffSignedOut returnTo={`/moderation/accounts/${id}/actions`} />
  ) : (
    <AccountRoute id={id} page="actions" />
  );
}
