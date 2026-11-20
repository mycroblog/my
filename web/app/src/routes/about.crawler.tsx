// SPDX-License-Identifier: AGPL-3.0-only

// The page for the operators of the websites that posts link to, on the crawler that fetches them for their cards, at
// /about/crawler (ADR-0028), for everyone, signed in or not, where the about page leads.

import { createFileRoute } from "@tanstack/react-router";
import { CrawlerPage } from "../about";

export const Route = createFileRoute("/about/crawler")({
  component: CrawlerPage,
  // A page of its own opens at its top.
  staticData: { scroll: "top" },
});
