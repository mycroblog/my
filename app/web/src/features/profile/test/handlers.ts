// SPDX-License-Identifier: AGPL-3.0-only
// The default MSW handlers of the profile feature (web-client/README.md §7): GET /accounts/lookup finds Alice (the
// viewer) and Bob with their follow counts, and remoteAccountFixture; the viewer has no relationship with
// anyone; each change (follow, block, mute and their removal) returns the relationship after it; an
// account's posts are the timeline fixture (the remote account's, remoteStatusFixture) and it pinned none,
// and each follow list holds the other local account.
import { http, HttpResponse } from "msw";
import {
  otherAccountFixture,
  relationshipFixture,
  remoteAccountFixture,
  remoteStatusFixture,
  type Schemas,
  statusPage,
  timelineStatusFixture,
  viewerFixture,
} from "../../../test/fixtures";
import { apiBase, authorized, problem, unauthenticated, withViewer } from "../../../test/msw/http";

/** The relationship that each change of the default handlers returns. */
const relationshipAfter: Readonly<Record<string, Partial<Schemas["Relationship"]>>> = {
  follow: { following: true },
  unfollow: {},
  block: { blocking: true },
  unblock: {},
  mute: { muting: true, muting_notifications: true },
  unmute: {},
};

const known: Schemas["Account"][] = [
  { ...viewerFixture, followers_count: 1, following_count: 1 },
  { ...otherAccountFixture, followers_count: 1, following_count: 1 },
  remoteAccountFixture,
];

const followList = (id: string): Schemas["AccountPage"] => ({
  accounts: [id === viewerFixture.id ? otherAccountFixture : viewerFixture],
});

export const handlers = [
  http.get(`${apiBase}/accounts/lookup`, ({ request }) => {
    const acct = (new URL(request.url).searchParams.get("acct") ?? "").replace(/^@/, "").toLowerCase();
    const found = known.find((a) => a.acct === acct);
    return found === undefined ? problem(404, "about:blank", "Not Found") : HttpResponse.json(found);
  }),
  http.get(`${apiBase}/accounts/relationships`, ({ request }) =>
    withViewer(request, () =>
      HttpResponse.json(new URL(request.url).searchParams.getAll("id").map((id) => relationshipFixture(id))),
    ),
  ),
  http.post(`${apiBase}/accounts/:id/:action`, ({ request, params }) => {
    if (!authorized(request)) return unauthenticated(request);
    const after = relationshipAfter[String(params["action"])];
    return after === undefined
      ? problem(404, "about:blank", "Not Found")
      : HttpResponse.json(relationshipFixture(String(params["id"]), after));
  }),
  http.get(`${apiBase}/accounts/:id/statuses`, ({ params, request }) =>
    new URL(request.url).searchParams.get("pinned") === "true"
      ? HttpResponse.json(statusPage([]))
      : HttpResponse.json(
          statusPage([params["id"] === remoteAccountFixture.id ? remoteStatusFixture : timelineStatusFixture]),
        ),
  ),
  http.get(`${apiBase}/accounts/:id/followers`, ({ params }) => HttpResponse.json(followList(String(params["id"])))),
  http.get(`${apiBase}/accounts/:id/following`, ({ params }) => HttpResponse.json(followList(String(params["id"])))),
];
