// SPDX-License-Identifier: AGPL-3.0-only
// The default MSW handlers of the hashtags feature (web-client/README.md §7): the viewer follows #sourdough and no other
// hashtag, and no account features any; every change succeeds. Following and the lists need a token.
import { http, HttpResponse } from "msw";
import { instanceFixture, type Schemas } from "../../../test/fixtures";
import { apiBase, withViewer } from "../../../test/msw/http";

/** The page of the hashtag `name`, as the server names it. */
function tagURL(name: string): string {
  return `https://${instanceFixture.domain}/tags/${encodeURIComponent(name.toLowerCase())}`;
}

/** The hashtag of a path parameter, which the client sends with its `#`. */
function nameOf(param: unknown): string {
  return String(param).replace(/^#/u, "");
}

/** A hashtag and whether the viewer follows it. */
export function hashtagFixture(name: string, following = false): Schemas["Hashtag"] {
  return { name, url: tagURL(name), following };
}

/** A hashtag the viewer follows, followed at `followedAt`. */
export function followedTagFixture(name: string, followedAt = "2026-11-01T09:00:00Z"): Schemas["FollowedHashtag"] {
  return { name, url: tagURL(name), followed_at: followedAt };
}

/** A hashtag an account features, on `count` of its posts, the last one at `lastAt`. */
export function featuredTagFixture(name: string, count = 0, lastAt?: string): Schemas["FeaturedHashtag"] {
  return { name, url: tagURL(name), statuses_count: count, ...(lastAt !== undefined && { last_status_at: lastAt }) };
}

/** The hashtag the viewer follows in the default handlers. */
export const followedSourdough = followedTagFixture("sourdough");

export const handlers = [
  http.get(`${apiBase}/tags/:hashtag`, ({ params }) => {
    // GET /tags/search is the search of hashtags (the compose feature's handler): an answer of nothing passes it on.
    if (params["hashtag"] === "search") return undefined;
    const name = nameOf(params["hashtag"]);
    return HttpResponse.json(hashtagFixture(name, name.toLowerCase() === followedSourdough.name));
  }),
  http.post(`${apiBase}/tags/:hashtag/follow`, ({ request, params }) =>
    withViewer(request, () => HttpResponse.json(hashtagFixture(nameOf(params["hashtag"]), true))),
  ),
  http.delete(`${apiBase}/tags/:hashtag/follow`, ({ request, params }) =>
    withViewer(request, () => HttpResponse.json(hashtagFixture(nameOf(params["hashtag"]), false))),
  ),
  http.get(`${apiBase}/followed_tags`, ({ request }) =>
    withViewer(request, () =>
      HttpResponse.json({ items: [followedSourdough] } satisfies Schemas["FollowedHashtagCollection"]),
    ),
  ),
  http.get(`${apiBase}/featured_tags`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: [] } satisfies Schemas["FeaturedHashtagCollection"])),
  ),
  http.put(`${apiBase}/featured_tags`, async ({ request }) => {
    const body = (await request.json()) as Schemas["SetFeaturedTagsRequest"];
    return withViewer(request, () =>
      HttpResponse.json({
        items: body.names.map((name) => featuredTagFixture(nameOf(name))),
      } satisfies Schemas["FeaturedHashtagCollection"]),
    );
  }),
  http.get(`${apiBase}/accounts/:id/featured_tags`, () =>
    HttpResponse.json({ items: [] } satisfies Schemas["FeaturedHashtagCollection"]),
  ),
];
