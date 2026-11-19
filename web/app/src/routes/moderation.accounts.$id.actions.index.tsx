// SPDX-License-Identifier: AGPL-3.0-only

// An account's Actions page, at /moderation/accounts/{id}/actions (ADR-0058 §2): every action on it, a page at a time,
// or those in effect. Without a session it asks the reader to sign in.

import { loadAccountActions, loadModeratedAccount } from "@my/app-data";
import { loadUnderEntry } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { searchOfQuery } from "../shell/staff";
import { AccountRoute } from "../shell/staff-accounts";
import { StaffSignedOut } from "../shell/staff-under";

export const Route = createFileRoute("/moderation/accounts/$id/actions/")({
  // The page of actions its query asks for (in effect alone, the page after a cursor), which the screen shows.
  loaderDeps: ({ search }) => ({ search: search as Record<string, unknown> }),
  loader: ({ context, params, location }) => {
    const search = searchOfQuery(location.searchStr);
    const cursor = search["cursor"] === "" ? undefined : search["cursor"];
    return screenLoad(context, (data) =>
      loadUnderEntry(data, "accounts", async (d) => {
        await Promise.all([
          loadModeratedAccount(d, params.id),
          loadAccountActions(d, params.id, search["active"] === "true", cursor),
        ]);
      }),
    );
  },
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
