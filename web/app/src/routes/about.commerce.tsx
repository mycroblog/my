// SPDX-License-Identifier: AGPL-3.0-only

// The commerce disclosure of the server's supporter plans (ADR-0072 §13), at /about/commerce, for everyone, signed in
// or not, where the about page, the supporters' settings and `Instance.commerce_disclosure_url` lead.

import { createFileRoute } from "@tanstack/react-router";
import { DocumentPage } from "../about";

export const Route = createFileRoute("/about/commerce")({
  component: () => <DocumentPage kind="commerce_disclosure" />,
  // A page of its own opens at its top.
  staticData: { scroll: "top" },
});
