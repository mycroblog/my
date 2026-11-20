// SPDX-License-Identifier: AGPL-3.0-only

// What the server states to search engines and AI crawlers, and what that means, at /about/crawler (FR-725, ADR-0057
// §5), for everyone, signed in or not, where the about page leads.

import { createFileRoute } from "@tanstack/react-router";
import { CrawlerPage } from "../about";

export const Route = createFileRoute("/about/crawler")({
  component: CrawlerPage,
  // A page of its own opens at its top.
  staticData: { scroll: "top" },
});
