// SPDX-License-Identifier: AGPL-3.0-only

// A rule, at /admin/rules/{id}: its text, explanation and place, a form saved as a whole, and Retire, under the entry
// Rules; a retired rule's facts. Leaving it with changes that are not saved is held back until the reader says
// whether to. Without a session it asks the reader to sign in.

import { loadRules, RuleScreen } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { adminSubLoad, AdminSubPage } from "../shell/staff";

export const Route = createFileRoute("/admin/rules/$id")({
  loader: ({ context }) => adminSubLoad(context, "manage_rules", loadRules),
  component: RulePage,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function RulePage() {
  const { data } = Route.useRouteContext();
  const { id } = Route.useParams();
  return (
    <AdminSubPage key={id} entry="rules" signedIn={data !== null} returnTo={`/admin/rules/${id}`}>
      {(sub) => <RuleScreen id={id} {...sub} />}
    </AdminSubPage>
  );
}
