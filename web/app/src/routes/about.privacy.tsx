// SPDX-License-Identifier: AGPL-3.0-only

// The server's privacy policy, at /about/privacy, for everyone, signed in or not, where the about page, the footer and
// `Instance.privacy_policy_url` lead.

import { createFileRoute } from "@tanstack/react-router";
import { DocumentPage } from "../about";

export const Route = createFileRoute("/about/privacy")({
  component: () => <DocumentPage kind="privacy_policy" />,
  // A page of its own opens at its top.
  staticData: { scroll: "top" },
});
