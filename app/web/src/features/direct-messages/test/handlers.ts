// SPDX-License-Identifier: AGPL-3.0-only
// The default MSW handlers of the direct messages (web-client/README.md §7): Alice's one conversation with Bob, read.
import { http, HttpResponse } from "msw";
import { otherAccountFixture, type Schemas, statusFixture } from "../../../test/fixtures";
import { apiBase, problem, withViewer } from "../../../test/msw/http";

/** Bob's direct post to Alice with `content`, ID `id`, written at `createdAt`. */
export function directStatusFixture(
  id: string,
  content: string,
  createdAt = "2026-10-16T09:00:00Z",
): Schemas["Status"] {
  return {
    ...statusFixture(id),
    visibility: "direct",
    content: `<p><a href="https://my.example/@alice">@alice</a> ${content}</p>`,
    created_at: createdAt,
  };
}

/** A conversation with Bob whose newest post is `lastStatus`, with `overrides`. */
export function conversationFixture(
  id: string,
  lastStatus: Schemas["Status"],
  overrides: Partial<Schemas["Conversation"]> = {},
): Schemas["Conversation"] {
  return { id, accounts: [otherAccountFixture], last_status: lastStatus, unread: false, ...overrides };
}

/** Alice's conversations for GET /conversations (operationId listConversations), newest first. */
export const conversationsFixture: Schemas["Conversation"][] = [
  conversationFixture(
    "0192f0c4-7a8e-7000-8000-00000000d001",
    directStatusFixture("0192f0c4-7a8e-7000-8000-0000000000e1", "see you at the bakery"),
  ),
];

/** The conversation `id` of the fixture, or a 404. */
function conversationResponse(id: unknown) {
  const conversation = conversationsFixture.find((item) => item.id === id);
  return conversation === undefined
    ? problem(404, "/api/v1/problems/not-found", "Not Found")
    : HttpResponse.json({ ...conversation, unread: false });
}

export const handlers = [
  http.get(`${apiBase}/conversations`, ({ request }) =>
    withViewer(request, () => {
      const body: Schemas["ConversationPage"] = { items: conversationsFixture };
      return HttpResponse.json(body);
    }),
  ),
  http.get(`${apiBase}/conversations/:id`, ({ request, params }) =>
    withViewer(request, () => conversationResponse(params["id"])),
  ),
  http.post(`${apiBase}/conversations/:id/read`, ({ request, params }) =>
    withViewer(request, () => conversationResponse(params["id"])),
  ),
  http.post(`${apiBase}/conversations/:id/hide`, ({ request }) =>
    withViewer(request, () => new HttpResponse(null, { status: 204 })),
  ),
];
