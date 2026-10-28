// SPDX-License-Identifier: AGPL-3.0-only
// The remote servers of the moderation screen stories: available and unavailable ones, a long domain among them,
// with the history of their availability, and the handlers that serve them as a list and one by one.
import { http, HttpResponse, type RequestHandler } from "msw";
import { longDomainAccountFixture, type Schemas } from "../../../test/fixtures";
import { apiBase, problem, withViewer } from "../../../test/msw/http";
import { hoursAgo, storyId } from "./stories";

function day(hours: number): string {
  return hoursAgo(hours).slice(0, 10);
}

/** The servers, unavailable ones first. */
export const storyInstances: Schemas["AdminInstanceHealth"][] = [
  { domain: "gone.example", state: "unavailable", since: hoursAgo(24 * 9), failure_days: 9, last_failure_day: day(2) },
  {
    domain: longDomainAccountFixture.domain,
    state: "unavailable",
    since: hoursAgo(24 * 7),
    failure_days: 7,
    last_failure_day: day(1),
  },
  { domain: "flaky.example", state: "available", failure_days: 3, last_failure_day: day(4) },
  { domain: "remote.example", state: "available", since: hoursAgo(24 * 2), failure_days: 0 },
  { domain: "mastodon.example", state: "available", failure_days: 0 },
  { domain: "xn--r8jz45g.example", state: "available", failure_days: 1, last_failure_day: day(30) },
];

/** The availability history of every server: marked unavailable, then available again, twice. */
export const storyAvailabilityEvents: Schemas["AdminAvailabilityEvent"][] = [
  { id: storyId("e1", 1), kind: "marked_unavailable", reason: "failure_days", occurred_at: hoursAgo(24 * 9) },
  { id: storyId("e1", 2), kind: "marked_available", reason: "operator", occurred_at: hoursAgo(24 * 20) },
  { id: storyId("e1", 3), kind: "marked_unavailable", reason: "failure_days", occurred_at: hoursAgo(24 * 30) },
  { id: storyId("e1", 4), kind: "marked_available", reason: "inbound_request", occurred_at: hoursAgo(24 * 60) },
];

/** The admin instances API over `instances`: the list filtered by state, each server, and `events` of each. */
export function instancesApi(
  instances: Schemas["AdminInstanceHealth"][],
  events: Schemas["AdminAvailabilityEvent"][] = storyAvailabilityEvents,
): RequestHandler[] {
  return [
    http.get(`${apiBase}/admin/instances`, ({ request }) => {
      const state = new URL(request.url).searchParams.get("state");
      const kept = instances.filter((instance) => state === null || instance.state === state);
      return withViewer(request, () => HttpResponse.json({ items: kept }));
    }),
    http.get(`${apiBase}/admin/instances/:domain`, ({ request, params }) => {
      const found = instances.find((instance) => instance.domain === params["domain"]);
      return withViewer(request, () =>
        found === undefined ? problem(404, "/api/v1/problems/not-found", "Not Found") : HttpResponse.json(found),
      );
    }),
    http.get(`${apiBase}/admin/instances/:domain/availability_events`, ({ request }) =>
      withViewer(request, () => HttpResponse.json({ items: events })),
    ),
  ];
}
