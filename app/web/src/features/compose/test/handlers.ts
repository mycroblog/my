// SPDX-License-Identifier: AGPL-3.0-only
// The default MSW handlers of the compose feature (web-client/README.md §7, web-client/12-compose.md §12): the preferences, posting,
// appealing a refused post, the media uploads, the drafts saved on the server (none, until a test saves one), and the searches of the completion, which find nothing unless a test offers results with
// `server.use(accountSearchHandler(...))` or `server.use(tagSearchHandler(...))`.
import { http, HttpResponse } from "msw";
import {
  draftFixture,
  mediaFixture,
  preferencesFixture,
  type Schemas,
  viewerStatusFixture,
} from "../../../test/fixtures";
import { apiBase, authorized, unauthenticated, withViewer } from "../../../test/msw/http";

/** The ID the default POST /media handler gives an upload. */
export const uploadedMediaId = "0192f0c4-7a8e-7000-8000-0000000000b1";

/** GET /accounts/search answering `accounts` whatever the query; `onQuery` sees each query. */
export function accountSearchHandler(accounts: Schemas["Account"][], onQuery?: (q: string) => void) {
  return http.get(`${apiBase}/accounts/search`, ({ request }) =>
    withViewer(request, () => {
      onQuery?.(new URL(request.url).searchParams.get("q") ?? "");
      return HttpResponse.json({ items: accounts });
    }),
  );
}

/** GET /tags/search answering `tags` whatever the query. */
export function tagSearchHandler(tags: Schemas["TagSuggestion"][]) {
  return http.get(`${apiBase}/tags/search`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: tags })),
  );
}

/** The ID the default POST /drafts handler gives a new draft. */
export const savedDraftId = "0192f0c4-7a8e-7000-8000-0000000004a1";

/** A draft's answer with its ETag, as every draft operation but the list sends it. */
export function draftResponse(draft: Schemas["Draft"], etag: string, status = 200) {
  return HttpResponse.json(draft, { status, headers: { ETag: etag } });
}

export const handlers = [
  // operationId listDrafts: no draft is saved.
  http.get(`${apiBase}/drafts`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: [] } satisfies Schemas["DraftPage"])),
  ),
  // operationId createDraft.
  http.post(`${apiBase}/drafts`, async ({ request }) => {
    if (!authorized(request)) return unauthenticated(request);
    const body = (await request.json()) as Schemas["DraftRequest"];
    return draftResponse(draftFixture(savedDraftId, body), '"v1"', 201);
  }),
  // operationId getDraft: any draft has some text.
  http.get(`${apiBase}/drafts/:id`, ({ request, params }) =>
    withViewer(request, () => draftResponse(draftFixture(String(params["id"]), { text: "A saved draft" }), '"v1"')),
  ),
  // operationId updateDraft.
  http.put(`${apiBase}/drafts/:id`, async ({ request, params }) => {
    if (!authorized(request)) return unauthenticated(request);
    const body = (await request.json()) as Schemas["DraftRequest"];
    return draftResponse(draftFixture(String(params["id"]), body), '"v2"');
  }),
  // operationId deleteDraft.
  http.delete(`${apiBase}/drafts/:id`, ({ request }) =>
    withViewer(request, () => new HttpResponse(null, { status: 204 })),
  ),
  // operationId scheduleDraft.
  http.put(`${apiBase}/drafts/:id/schedule`, async ({ request, params }) => {
    if (!authorized(request)) return unauthenticated(request);
    const { publish_at: publishAt } = (await request.json()) as Schemas["DraftScheduleRequest"];
    return draftResponse(
      draftFixture(String(params["id"]), { text: "A saved draft" }, { scheduled_at: publishAt }),
      '"v1"',
    );
  }),
  // operationId unscheduleDraft.
  http.delete(`${apiBase}/drafts/:id/schedule`, ({ request, params }) =>
    withViewer(request, () => draftResponse(draftFixture(String(params["id"]), { text: "A saved draft" }), '"v1"')),
  ),
  // operationIds searchAccounts and searchTags.
  accountSearchHandler([]),
  tagSearchHandler([]),
  http.get(`${apiBase}/preferences`, ({ request }) => withViewer(request, () => HttpResponse.json(preferencesFixture))),
  // Posts the status as the viewer, as the server would render it.
  http.post(`${apiBase}/statuses`, async ({ request }) => {
    if (!authorized(request)) return unauthenticated(request);
    const body = (await request.json()) as Schemas["CreateStatusRequest"];
    return HttpResponse.json(
      viewerStatusFixture({
        content: `<p>${body.text}</p>`,
        spoiler_text: body.spoiler_text ?? "",
        sensitive: body.sensitive ?? false,
        visibility: body.visibility,
        language: body.language ?? preferencesFixture.default_post_language,
        ...(body.in_reply_to_id === undefined ? {} : { in_reply_to_id: body.in_reply_to_id }),
        ...(body.quoted_status_id === undefined ? {} : { quote: { state: "pending" } }),
        interaction_policy: { ...preferencesFixture.default_interaction_policy, ...body.interaction_policy },
        media_attachments: (body.media ?? []).map((m) => mediaFixture(m.id, "ready", m.description)),
      }),
    );
  }),
  // operationId appealHookDecision: the appeal of a refused post awaits a moderator.
  http.post(`${apiBase}/hook_decisions/:id/appeal`, ({ request, params }) =>
    withViewer(request, () =>
      HttpResponse.json({
        id: "0192f0c4-7a8e-7000-8000-0000000003b1",
        hook_decision_id: String(params["id"]),
        state: "pending",
        submitted_at: "2026-11-06T00:10:00Z",
      } satisfies Schemas["OwnHookAppeal"]),
    ),
  ),
  // operationId getOwnHookAppeal: the appeal of a refused post still awaits a moderator.
  http.get(`${apiBase}/hook_decisions/:id/appeal`, ({ request, params }) =>
    withViewer(request, () =>
      HttpResponse.json({
        id: "0192f0c4-7a8e-7000-8000-0000000003b1",
        hook_decision_id: String(params["id"]),
        state: "pending",
        submitted_at: "2026-11-06T00:10:00Z",
      } satisfies Schemas["OwnHookAppeal"]),
    ),
  ),
  // Accepts the upload; it is processed by the time the client asks again.
  http.post(`${apiBase}/media`, () => HttpResponse.json(mediaFixture(uploadedMediaId, "processing"), { status: 202 })),
  http.get(`${apiBase}/media/:id`, ({ params }) => HttpResponse.json(mediaFixture(String(params["id"])))),
  http.patch(`${apiBase}/media/:id`, async ({ params, request }) =>
    HttpResponse.json(
      mediaFixture(String(params["id"]), "ready", ((await request.json()) as Schemas["MediaUpdate"]).description),
    ),
  ),
];
