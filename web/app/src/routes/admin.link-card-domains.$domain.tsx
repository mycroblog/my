// SPDX-License-Identifier: AGPL-3.0-only

// The rule of the link card crawler of a domain, at /admin/link-card-domains/{domain}, under the entry Link card
// domains: its mode and note as a form, and Remove last. Without a session it asks the reader to sign in.

import { loadLinkCardDomains, loadUnderOperations } from "@my/app-features";
import { createFileRoute } from "@tanstack/react-router";
import { screenLoad } from "../screen-load";
import { LinkCardRuleRoute } from "../shell/staff-operations";

export const Route = createFileRoute("/admin/link-card-domains/$domain")({
  loader: ({ context }) =>
    screenLoad(context, (data) => loadUnderOperations(data, "link-card-domains", loadLinkCardDomains)),
  component: Page,
  // A screen of its own opens at its top.
  staticData: { scroll: "top" },
});

function Page() {
  const { data } = Route.useRouteContext();
  const { domain } = Route.useParams();
  return <LinkCardRuleRoute domain={domain} signedIn={data !== null} />;
}
