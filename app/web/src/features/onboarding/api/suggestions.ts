// SPDX-License-Identifier: AGPL-3.0-only
// The queries of the onboarding feature (web-client/README.md S1): the accounts suggested to follow (FR-308).
import type { components } from "@mycroblog/mykit";
import type { Api } from "../../../lib/api/client";

export type AccountSuggestion = components["schemas"]["AccountSuggestion"];

/** How many suggestions the home timeline shows (web-client/screens/home-first-run.md). */
const suggestionsShown = 6;

/** The accounts suggested to the viewer (GET /accounts/suggestions), featured ones first. */
export function suggestionsQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/accounts/suggestions", { params: { query: { limit: suggestionsShown } } });
}
