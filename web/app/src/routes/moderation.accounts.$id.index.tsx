// SPDX-License-Identifier: AGPL-3.0-only

// An account's Review page, at /moderation/accounts/{id} (ADR-0058 §2): its facts, a new action, its newest actions and
// reports. Its query names the report the reader came from (`?report=`). Without a session it asks the reader to sign in.

import { loadModeratedAccount } from "@my/app-data";
import { loadUnderEntry } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { AccountRoute } from "../shell/staff-accounts";
import { StaffSignedOut } from "../shell/staff-under";

export const Route = createFileRoute("/moderation/accounts/$id/")({
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
    <StaffSignedOut returnTo={`/moderation/accounts/${id}`} />
  ) : (
    <AccountRoute id={id} page="review" />
  );
}
