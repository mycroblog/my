// SPDX-License-Identifier: AGPL-3.0-only

// A new rule of the link card crawler, at /admin/link-card-domains/new, under the entry Link card domains: its domain,
// its mode and a note, held while it has changes not saved. Without a session it asks the reader to sign in.

import { loadLinkCardDomains, loadUnderOperations } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { NewLinkCardRuleRoute } from "../shell/staff-operations";

export const Route = createFileRoute("/admin/link-card-domains/new")({
  loader: ({ context }) =>
    screenLoad(context, (data) => loadUnderOperations(data, "link-card-domains", loadLinkCardDomains)),
  component: Page,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function Page() {
  const { data } = Route.useRouteContext();
  return <NewLinkCardRuleRoute signedIn={data !== null} />;
}
