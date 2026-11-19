// SPDX-License-Identifier: AGPL-3.0-only

// A new rule, at /admin/rules/new: its text and explanation, a form saved as a whole under the entry Rules, which it
// goes back to once added. Leaving it with changes that are not saved is held back until the reader says whether to.
// Without a session it asks the reader to sign in.

import { loadRules, NewRuleScreen } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { adminSubLoad, AdminSubPage } from "../shell/staff";

export const Route = createFileRoute("/admin/rules/new")({
  loader: ({ context }) => adminSubLoad(context, "manage_rules", loadRules),
  component: NewRulePage,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function NewRulePage() {
  const { data } = Route.useRouteContext();
  return (
    <AdminSubPage entry="rules" signedIn={data !== null} returnTo="/admin/rules/new">
      {(sub) => <NewRuleScreen {...sub} />}
    </AdminSubPage>
  );
}
