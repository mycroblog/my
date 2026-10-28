// SPDX-License-Identifier: AGPL-3.0-only
// The default MSW handlers of the onboarding feature (web-client/README.md §7): the accounts suggested to follow, none
// unless a test offers some with `server.use(suggestionsHandler(...))`.
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../../test/fixtures";
import { apiBase, withViewer } from "../../../test/msw/http";

/** GET /accounts/suggestions answering `suggestions`. */
export function suggestionsHandler(suggestions: Schemas["AccountSuggestion"][]) {
  return http.get(`${apiBase}/accounts/suggestions`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(suggestions)),
  );
}

export const handlers = [
  // operationId listAccountSuggestions.
  suggestionsHandler([]),
];
