// SPDX-License-Identifier: AGPL-3.0-only
// The default MSW handlers of the keyword filters feature (web-client/README.md §7): the user has no filters, and the
// server answers a filter it is sent as it would store it.
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../../test/fixtures";
import { apiBase, problem, withViewer } from "../../../test/msw/http";

/** A filter as the server answers it: `request` with an ID, its creation, and whether its end has passed. */
export function keywordFilterFixture(
  id: string,
  request: Schemas["KeywordFilterRequest"],
  now = "2026-11-01T09:00:00Z",
): Schemas["KeywordFilter"] {
  return {
    id,
    title: request.title.trim(),
    action: request.action,
    contexts: request.contexts,
    keywords: request.keywords.map((keyword) => ({ keyword: keyword.keyword.trim(), whole_word: keyword.whole_word })),
    ...(request.expires_at === undefined ? {} : { expires_at: request.expires_at }),
    expired: request.expires_at !== undefined && Date.parse(request.expires_at) <= Date.parse(now),
    created_at: now,
  };
}

export const handlers = [
  // operationIds listKeywordFilters, createKeywordFilter, getKeywordFilter, updateKeywordFilter, deleteKeywordFilter:
  // no filters by default; a filter sent is answered as stored.
  http.get(`${apiBase}/filters`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: [] } satisfies Schemas["KeywordFilterCollection"])),
  ),
  http.post(`${apiBase}/filters`, async ({ request }) => {
    const body = (await request.clone().json()) as Schemas["KeywordFilterRequest"];
    return withViewer(request, () => HttpResponse.json(keywordFilterFixture("filter-1", body), { status: 201 }));
  }),
  http.get(`${apiBase}/filters/:id`, ({ request }) =>
    withViewer(request, () => problem(404, "about:blank", "Not Found")),
  ),
  http.patch(`${apiBase}/filters/:id`, async ({ request, params }) => {
    const body = (await request.clone().json()) as Schemas["KeywordFilterRequest"];
    return withViewer(request, () => HttpResponse.json(keywordFilterFixture(String(params["id"]), body)));
  }),
  http.delete(`${apiBase}/filters/:id`, ({ request }) =>
    withViewer(request, () => new HttpResponse(null, { status: 204 })),
  ),
];
