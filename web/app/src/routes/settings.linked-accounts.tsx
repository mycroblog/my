// SPDX-License-Identifier: AGPL-3.0-only

// /settings/linked-accounts, where the server sends the browser back from a login provider when the app named no
// other address (ADR-0106 §3.5–§3.7): the linked accounts are a section of the settings' Security, so this address
// leads there, with the fragment that says what happened at the provider. The app itself names Security, or the
// screen that asked, as the address to come back to.

import { securityPath } from "@my/app-features";
import { createFileRoute, redirect } from "@tanstack/react-router";

export const Route = createFileRoute("/settings/linked-accounts")({
  beforeLoad: ({ location }) => {
    // eslint-disable-next-line typescript/only-throw-error -- a redirect is thrown, as the router asks
    throw redirect({ href: `${securityPath}${location.hash === "" ? "" : `#${location.hash}`}`, replace: true });
  },
  // Never shown: the address is left before it loads.
  staticData: { scroll: "top" },
});
