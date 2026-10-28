// SPDX-License-Identifier: AGPL-3.0-only
// The default MSW handlers of the status feature (web-client/README.md §7, web-client/13-status-detail-and-actions.md §13): any post (GET /statuses/{id}), as
// Bob's post `statusFixture`; the thread around it (GET /statuses/{id}/context): no ancestors and no
// replies; its source (GET /statuses/{id}/source): the plain text of that post; its history
// (GET /statuses/{id}/history): the one revision of a post never edited; and the actions on it (boost,
// favourite, bookmark, pin and their removal; edit; delete; the emoji reaction and its removal). An action answers
// the post with the viewer's relation set as asked (a reaction: the post with that one reaction, the viewer's); an
// edit answers it with the new text as its content and `edited_at` set. The accounts that reacted to a post
// (GET /statuses/{id}/reactions) are Bob with 👍, in one page. A vote in the poll of a post
// (POST /statuses/{id}/poll/votes) answers the post with `pollFixture` and the vote counted in it. A change of who
// may quote a post (PUT /statuses/{id}/quote_policy) answers the post with that policy; the posts that quote a post
// (GET /statuses/{id}/quotes) are none; and the revocation of an approval (POST /statuses/{id}/quotes/{quote_id}/revoke)
// answers 204.
import { http, HttpResponse } from "msw";
import { otherAccountFixture, type Schemas, statusFixture } from "../../../test/fixtures";
import { apiBase, withViewer } from "../../../test/msw/http";

/** The viewer's relations to a post, as the Status fields that the actions set. */
type RelationFlag = "boosted" | "favourited" | "bookmarked";

const relation = (path: string, flag: RelationFlag) =>
  [true, false].map((on) =>
    (on ? http.post : http.delete)(`${apiBase}/statuses/:id/${path}`, ({ params, request }) =>
      withViewer(request, () => HttpResponse.json({ ...statusFixture(String(params["id"])), [flag]: on })),
    ),
  );

const context: Schemas["StatusContext"] = { ancestors: [], descendants: [] };

/** The one revision of a post never edited. */
function historyOf(status: Schemas["Status"]): Schemas["StatusHistory"] {
  const { content, spoiler_text, sensitive, created_at, media_attachments, emojis } = status;
  return { items: [{ content, spoiler_text, sensitive, created_at, media_attachments, emojis }] };
}

/** A single-choice poll of three options that ends a day after the tests' now, before the viewer voted. */
export const pollFixture: Schemas["Poll"] = {
  id: "0192f0c4-7a8e-7000-8000-0000000000e1",
  expires_at: "2026-10-17T12:00:00Z",
  expired: false,
  multiple: false,
  votes_count: 4,
  voters_count: 4,
  options: [
    { title: "Red", votes_count: 1 },
    { title: "Blue", votes_count: 3 },
    { title: "Green", votes_count: 0 },
  ],
  voted: false,
  own_votes: [],
};

/** `poll` after the viewer voted for the options at `choices`. */
export function votedPoll(poll: Schemas["Poll"], choices: readonly number[]): Schemas["Poll"] {
  return {
    ...poll,
    votes_count: poll.votes_count + choices.length,
    ...(poll.voters_count !== undefined && { voters_count: poll.voters_count + 1 }),
    options: poll.options.map((option, index) =>
      choices.includes(index) ? { ...option, votes_count: (option.votes_count ?? 0) + 1 } : option,
    ),
    voted: true,
    own_votes: [...choices],
  };
}

export const handlers = [
  http.get(`${apiBase}/statuses/:id`, ({ params }) => HttpResponse.json(statusFixture(String(params["id"])))),
  http.get(`${apiBase}/statuses/:id/context`, () => HttpResponse.json(context)),
  http.get(`${apiBase}/statuses/:id/history`, ({ params }) =>
    HttpResponse.json(historyOf(statusFixture(String(params["id"])))),
  ),
  http.get(`${apiBase}/statuses/:id/source`, ({ params, request }) =>
    withViewer(request, () =>
      HttpResponse.json({
        id: String(params["id"]),
        text: "@alice hello",
        spoiler_text: "",
        language: "en",
      } satisfies Schemas["StatusSource"]),
    ),
  ),
  http.put(`${apiBase}/statuses/:id`, async ({ params, request }) => {
    const body = (await request.clone().json()) as Schemas["EditStatusRequest"];
    return withViewer(request, () =>
      HttpResponse.json({
        ...statusFixture(String(params["id"])),
        content: `<p>${body.text}</p>`,
        spoiler_text: body.spoiler_text ?? "",
        edited_at: "2026-10-16T12:00:00Z",
      }),
    );
  }),
  ...relation("boost", "boosted"),
  ...relation("favourite", "favourited"),
  ...relation("bookmark", "bookmarked"),
  ...[true, false].map((on) =>
    (on ? http.post : http.delete)(`${apiBase}/statuses/:id/pin`, ({ params, request }) =>
      withViewer(request, () => HttpResponse.json({ ...statusFixture(String(params["id"])), pinned: on })),
    ),
  ),
  http.delete(`${apiBase}/statuses/:id`, ({ request }) =>
    withViewer(request, () => new HttpResponse(null, { status: 204 })),
  ),
  http.put(`${apiBase}/statuses/:id/reaction`, async ({ params, request }) => {
    const body = (await request.clone().json()) as Schemas["ReactToStatusRequest"];
    return withViewer(request, () =>
      HttpResponse.json({
        ...statusFixture(String(params["id"])),
        reactions: [{ name: body.emoji, count: 1, me: true }],
      } satisfies Schemas["Status"]),
    );
  }),
  http.delete(`${apiBase}/statuses/:id/reaction`, ({ params, request }) =>
    withViewer(request, () => HttpResponse.json(statusFixture(String(params["id"])))),
  ),
  http.post(`${apiBase}/statuses/:id/poll/votes`, async ({ params, request }) => {
    const body = (await request.clone().json()) as Schemas["VoteInPollRequest"];
    return withViewer(request, () =>
      HttpResponse.json({
        ...statusFixture(String(params["id"])),
        poll: votedPoll(pollFixture, body.choices),
      } satisfies Schemas["Status"]),
    );
  }),
  http.get(`${apiBase}/statuses/:id/reactions`, () =>
    HttpResponse.json({
      items: [{ account: otherAccountFixture, emoji: { name: "👍" }, created_at: "2026-10-16T10:00:00Z" }],
    } satisfies Schemas["StatusReactionPage"]),
  ),
  http.put(`${apiBase}/statuses/:id/quote_policy`, async ({ params, request }) => {
    const body = (await request.json()) as Schemas["SetQuotePolicyRequest"];
    return withViewer(request, () =>
      HttpResponse.json({ ...statusFixture(String(params["id"])), quote_policy: body.quote_policy }),
    );
  }),
  http.get(`${apiBase}/statuses/:id/quotes`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: [] } satisfies Schemas["StatusPage"])),
  ),
  http.post(`${apiBase}/statuses/:id/quotes/:quoteId/revoke`, ({ request }) =>
    withViewer(request, () => new HttpResponse(null, { status: 204 })),
  ),
];
