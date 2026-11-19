// SPDX-License-Identifier: AGPL-3.0-only

// An action on an account, at /moderation/accounts/{id}/actions/{actionId} (ADR-0058 §2): all it holds, and Lift while
// it is in effect. Without a session it asks the reader to sign in.

import { loadAccountAction, loadModeratedAccount } from "@my/app-data";
import { loadUnderEntry } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { AccountActionRoute } from "../shell/staff-accounts";
import { StaffSignedOut } from "../shell/staff-under";

export const Route = createFileRoute("/moderation/accounts/$id/actions/$actionId")({
  loader: ({ context, params }) =>
    screenLoad(context, (data) =>
      loadUnderEntry(data, "accounts", (d) =>
        Promise.all([loadAccountAction(d, params.actionId), loadModeratedAccount(d, params.id)]).then(() => undefined),
      ),
    ),
  component: Page,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function Page() {
  const { data } = Route.useRouteContext();
  const { id, actionId } = Route.useParams();
  return data === null ? (
    <StaffSignedOut returnTo={`/moderation/accounts/${id}/actions/${actionId}`} />
  ) : (
    <AccountActionRoute account={id} id={actionId} />
  );
}
