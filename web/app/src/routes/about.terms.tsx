// SPDX-License-Identifier: AGPL-3.0-only

// The server's terms of service, at /about/terms, for everyone, signed in or not, where the about page, the footer and
// `Instance.terms_url` lead.

import { createFileRoute } from "@tanstack/react-router";
import { DocumentPage } from "../about";

export const Route = createFileRoute("/about/terms")({
  component: () => <DocumentPage kind="terms" />,
  // A page of its own opens at its top.
  staticData: { scroll: "top" },
});
