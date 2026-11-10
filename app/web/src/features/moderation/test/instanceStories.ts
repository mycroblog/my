// SPDX-License-Identifier: AGPL-3.0-only
// The remote servers of the moderation screen stories: available and unavailable ones, a long domain among them,
// with the history of their availability and the deliveries to them, and the handlers that serve them as a list and
// one by one.
import { http, HttpResponse, type RequestHandler } from "msw";
import { longDomainAccountFixture, type Schemas } from "../../../test/fixtures";
import { apiBase, problem, withViewer } from "../../../test/msw/http";
import { hoursAgo, storyId } from "./stories";

function day(hours: number): string {
  return hoursAgo(hours).slice(0, 10);
}

/** The servers, unavailable ones first. */
export const storyInstances: Schemas["AdminInstanceHealth"][] = [
  {
    domain: "gone.example",
    state: "unavailable",
    state_changed_at: hoursAgo(24 * 9),
    failure_days: 9,
    last_failure_day: day(2),
  },
  {
    domain: longDomainAccountFixture.domain,
    state: "unavailable",
    state_changed_at: hoursAgo(24 * 7),
    failure_days: 7,
    last_failure_day: day(1),
  },
  { domain: "flaky.example", state: "available", failure_days: 3, last_failure_day: day(4) },
  { domain: "remote.example", state: "available", state_changed_at: hoursAgo(24 * 2), failure_days: 0 },
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

/**
 * The deliveries to a server, newest first: a public post to the shared inbox that went through at once, a direct
 * post (masked) that failed once and then went through, a follow of an account that approves its followers (masked)
 * that the server refused, and an answer that was not sent at all.
 */
export const storyDeliveries: Schemas["AdminDelivery"][] = [
  {
    id: storyId("d1", 1),
    domain: "gone.example",
    activity_type: "Create",
    activity_created_at: hoursAgo(1),
    created_at: hoursAgo(1),
    shared_inbox: true,
    sender: { account_id: storyId("a1", 1), username: "hana" },
    activity_uri: "https://my.example/users/hana/statuses/0192f0c4-7a8e-7000-8000-d10000000001/activity",
    inbox_url: "https://gone.example/inbox",
    attempts: [
      {
        id: storyId("d2", 1),
        outcome: "succeeded",
        signature_scheme: "rfc9421",
        status_code: 202,
        attempted_at: hoursAgo(1),
      },
    ],
  },
  {
    id: storyId("d1", 2),
    domain: "gone.example",
    activity_type: "Create",
    activity_created_at: hoursAgo(5),
    created_at: hoursAgo(5),
    shared_inbox: false,
    mask: "direct",
    attempts: [
      { id: storyId("d2", 2), outcome: "retryable_failure", error: "dial tcp: i/o timeout", attempted_at: hoursAgo(5) },
      {
        id: storyId("d2", 3),
        outcome: "succeeded",
        signature_scheme: "cavage",
        status_code: 202,
        attempted_at: hoursAgo(4),
      },
    ],
  },
  {
    id: storyId("d1", 3),
    domain: "gone.example",
    activity_type: "Follow",
    activity_created_at: hoursAgo(30),
    created_at: hoursAgo(30),
    shared_inbox: false,
    mask: "approves_followers",
    attempts: [
      {
        id: storyId("d2", 4),
        outcome: "permanent_failure",
        signature_scheme: "cavage",
        status_code: 410,
        attempted_at: hoursAgo(30),
      },
    ],
  },
  {
    id: storyId("d1", 4),
    domain: "gone.example",
    activity_type: "Accept",
    activity_created_at: hoursAgo(50),
    created_at: hoursAgo(50),
    shared_inbox: false,
    sender: { account_id: storyId("a1", 2) },
    activity_uri: "https://my.example/activities/0192f0c4-7a8e-7000-8000-d10000000004",
    inbox_url: "https://gone.example/users/aoi/inbox",
    attempts: [{ id: storyId("d2", 5), outcome: "skipped", error: "the server is paused", attempted_at: hoursAgo(50) }],
  },
];

/** What the reveal of a delivery among `storyDeliveries` answers. */
function storyReveal(delivery: Schemas["AdminDelivery"]): Schemas["AdminDeliveryReveal"] {
  return {
    id: delivery.id,
    ...(delivery.mask === undefined ? {} : { mask: delivery.mask }),
    sender: delivery.sender ?? { account_id: storyId("a1", 3), username: "kai" },
    activity_uri: delivery.activity_uri ?? `https://my.example/activities/${delivery.id}`,
    inbox_url: delivery.inbox_url ?? "https://gone.example/users/aoi/inbox",
  };
}

/** The deliveries API over `deliveries`: the list, filtered by activity URI, and the reveal of each. */
export function deliveriesApi(deliveries: Schemas["AdminDelivery"][] = storyDeliveries): RequestHandler[] {
  return [
    http.get(`${apiBase}/admin/instances/:domain/deliveries`, ({ request }) => {
      const uri = new URL(request.url).searchParams.get("activity_uri");
      const kept = deliveries.filter((delivery) => uri === null || delivery.activity_uri === uri);
      return withViewer(request, () => HttpResponse.json({ items: kept }));
    }),
    http.post(`${apiBase}/admin/instances/:domain/deliveries/:id/reveal`, ({ request, params }) => {
      const found = deliveries.find((delivery) => delivery.id === params["id"]);
      return withViewer(request, () =>
        found === undefined
          ? problem(404, "/api/v1/problems/not-found", "Not Found")
          : HttpResponse.json(storyReveal(found)),
      );
    }),
  ];
}
