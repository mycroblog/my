// SPDX-License-Identifier: AGPL-3.0-only
// The decisions and the appeals of refused posts of the automated reviews stories: a drop with an account, a note and a
// reason, an observed flag, a timed-out follow, and appeals awaiting review (one in Arabic without a comment) and
// decided, with the handlers that serve them as a list and one by one.
import { http, HttpResponse, type RequestHandler } from "msw";
import { remoteAccountFixture, rtlNameAccountFixture, type Schemas } from "../../../test/fixtures";
import { apiBase, problem, withViewer } from "../../../test/msw/http";
import { decidedHookAppeal, hookAppealFixture, hookDecisionFixture, hookSummaryFixture } from "./handlers";
import { hoursAgo, hoursFromNow, storyId } from "./stories";

/** The decisions, newest first. */
export const storyHookDecisions: Schemas["AdminHookDecision"][] = [
  hookDecisionFixture({
    id: storyId("b2", 3),
    subject_key: "https://remote.example/users/spammer/statuses/1/activity",
    account_id: remoteAccountFixture.id,
    reason: "spam",
    note: "Same link posted to many accounts",
    decided_at: hoursAgo(2),
    restorable_until: hoursFromNow(24 * 5),
  }),
  hookDecisionFixture({
    id: storyId("b2", 2),
    subject_key: "https://remote.example/users/someone/statuses/2/activity",
    mode: "observe",
    outcome: "flag",
    overridable: false,
    decided_at: hoursAgo(6),
  }),
  hookDecisionFixture({
    id: storyId("b2", 1),
    hook: "inbound.follow",
    subject_key: "https://remote.example/users/someone/follows/3",
    outcome: "timeout",
    latency_ms: 3000,
    overridable: false,
    decided_at: hoursAgo(30),
  }),
];

/** The appeals of refused posts, newest first: two await review, one is upheld. */
export const storyHookAppeals: Schemas["AdminHookAppeal"][] = [
  hookAppealFixture({ submitted_at: hoursAgo(1) }),
  hookAppealFixture({
    id: storyId("b3", 2),
    account_id: rtlNameAccountFixture.id,
    status_text: "حفلة في الحي يوم السبت",
    comment: "",
    submitted_at: hoursAgo(5),
  }),
  decidedHookAppeal(hookAppealFixture({ id: storyId("b3", 1), submitted_at: hoursAgo(50) }), "uphold"),
];

/**
 * The admin API of the automated reviews over `decisions` and `appeals`: the summary, the lists (the pending appeals
 * for `state=pending`) and each record by its ID.
 */
export function hookReviewsApi(
  decisions: Schemas["AdminHookDecision"][],
  appeals: Schemas["AdminHookAppeal"][],
): RequestHandler[] {
  const one = (items: readonly { id: string }[], id: unknown, request: Request) => {
    const found = items.find((item) => item.id === id);
    return withViewer(request, () =>
      found === undefined ? problem(404, "/api/v1/problems/not-found", "Not Found") : HttpResponse.json(found),
    );
  };
  return [
    http.get(`${apiBase}/admin/hook_decisions/summary`, ({ request }) =>
      withViewer(request, () => HttpResponse.json(hookSummaryFixture())),
    ),
    http.get(`${apiBase}/admin/hook_decisions/:id`, ({ request, params }) => one(decisions, params["id"], request)),
    http.get(`${apiBase}/admin/hook_appeals`, ({ request }) => {
      const state = new URL(request.url).searchParams.get("state");
      const kept = appeals.filter((appeal) => state === null || appeal.state === state);
      return withViewer(request, () => HttpResponse.json({ items: kept }));
    }),
    http.get(`${apiBase}/admin/hook_appeals/:id`, ({ request, params }) => one(appeals, params["id"], request)),
  ];
}
