// SPDX-License-Identifier: AGPL-3.0-only
// The default MSW handlers of the events feature (web-client/README.md §7): no event is coming up, so the list, the
// third column and the profiles show none until a test serves some. Bob's event of the fixtures is readable by its
// ID, with nobody answering yet. The fixtures build the events that a test serves.
import { http, HttpResponse } from "msw";
import { otherAccountFixture, type Schemas } from "../../../test/fixtures";
import { apiBase, problem, withViewer } from "../../../test/msw/http";

/** The ID of the event of the fixtures. */
export const eventId = "0192f0c4-7a8e-7000-8000-0000000000e1";

/**
 * Bob's picnic for every local user, on Saturday 2026-12-05 from 21:00 to 23:00 in Tokyo (12:00 to 14:00 UTC), with
 * a URL place that the user sees and a place in person shown only to approved attendees; `overrides` replace fields.
 */
export function eventFixture(overrides: Partial<Schemas["Event"]> = {}): Schemas["Event"] {
  return {
    id: eventId,
    organizer: otherAccountFixture,
    coorganizers: [],
    title: "Night picnic",
    description: "Bring a blanket. Map: https://maps.example/park",
    visibility: "public",
    time_zone: "Asia/Tokyo",
    place_disclosure: "approved",
    capacity: 0,
    places: [{ kind: "url", name: "Live stream", url: "https://stream.example/live" }],
    hidden_places: ["physical"],
    starts_at: "2026-12-05T12:00:00Z",
    ends_at: "2026-12-05T14:00:00Z",
    state: "scheduled",
    going_count: 0,
    created_at: "2026-11-01T00:00:00Z",
    revised_at: "2026-11-01T00:00:00Z",
    revision_count: 1,
    viewer: { organizer: false, coorganizer: false, invited: false, approved: false },
    ...overrides,
  };
}

const notFound = () => problem(404, "/api/v1/problems/not-found", "Not Found");

export const handlers = [
  http.get(`${apiBase}/events`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: [] } satisfies Schemas["EventPage"])),
  ),
  http.get(`${apiBase}/events/:id`, ({ request, params }) =>
    withViewer(request, () => (params["id"] === eventId ? HttpResponse.json(eventFixture()) : notFound())),
  ),
  http.get(`${apiBase}/events/:id/attendees`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: [] } satisfies Schemas["EventAttendeePage"])),
  ),
  http.get(`${apiBase}/events/:id/invitations`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: [] } satisfies Schemas["EventInvitationPage"])),
  ),
];
