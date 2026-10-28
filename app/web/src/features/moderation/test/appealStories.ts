// SPDX-License-Identifier: AGPL-3.0-only
// The appeals of the moderation screen stories: pending, upheld and overturned ones of several kinds of action, by
// accounts with long and right-to-left names, and the handlers that serve them as a list and one by one.
import { http, HttpResponse, type RequestHandler } from "msw";
import { longNameAccountFixture, rtlNameAccountFixture, type Schemas, viewerFixture } from "../../../test/fixtures";
import { apiBase, problem, withViewer } from "../../../test/msw/http";
import { appealFixture } from "./handlers";
import { hoursAgo, hoursFromNow, monaAccount, storyId } from "./stories";

/** The appeals, newest first. */
export const storyAppeals: Schemas["AdminAppeal"][] = [
  appealFixture({
    submitted_at: hoursAgo(3),
    text: "I posted the link to my own blog once, in a thread where someone asked for it. I did not advertise anything, and I would like the suspension lifted so I can keep talking to the people I follow.",
  }),
  appealFixture({
    id: storyId("f1", 2),
    account_id: longNameAccountFixture.id,
    text: "The photo was of a painting in a museum.",
    submitted_at: hoursAgo(30),
    action: {
      ...appealFixture().action,
      id: storyId("d1", 2),
      account_id: longNameAccountFixture.id,
      kind: "mark_statuses_sensitive",
      reason_category: "violation",
      public_reason: "Media without a content warning.",
      issued_by_account_id: monaAccount.id,
      issued_at: hoursAgo(40),
      expires_at: hoursFromNow(24 * 7),
    },
  }),
  appealFixture({
    id: storyId("f1", 3),
    account_id: rtlNameAccountFixture.id,
    text: "لم أكن أعلم أن هذا مخالف للقواعد.",
    state: "upheld",
    submitted_at: hoursAgo(120),
    decided_at: hoursAgo(100),
    decided_by_account_id: viewerFixture.id,
    decision_reason: "The posts break the rules as written; the limit stays.",
    action: {
      ...appealFixture().action,
      id: storyId("d1", 3),
      account_id: rtlNameAccountFixture.id,
      kind: "limit",
      issued_at: hoursAgo(130),
    },
  }),
  appealFixture({
    id: storyId("f1", 4),
    text: "My account was taken over; I have changed the password and turned on two-factor authentication.",
    state: "overturned",
    submitted_at: hoursAgo(300),
    decided_at: hoursAgo(290),
    decided_by_account_id: monaAccount.id,
    decision_reason: "Confirmed with the sign-in history.",
    action: { ...appealFixture().action, id: storyId("d1", 4), state: "lifted", lifted_at: hoursAgo(290) },
  }),
];

/** The admin appeals API over `appeals`: the list, the pending ones for `state=pending`, and each appeal by its ID. */
export function appealsApi(appeals: Schemas["AdminAppeal"][]): RequestHandler[] {
  return [
    http.get(`${apiBase}/admin/appeals`, ({ request }) => {
      const state = new URL(request.url).searchParams.get("state");
      const kept = appeals.filter((appeal) => state === null || appeal.state === state);
      return withViewer(request, () => HttpResponse.json({ items: kept }));
    }),
    http.get(`${apiBase}/admin/appeals/:id`, ({ request, params }) => {
      const appeal = appeals.find((a) => a.id === params["id"]);
      return withViewer(request, () =>
        appeal === undefined ? problem(404, "/api/v1/problems/not-found", "Not Found") : HttpResponse.json(appeal),
      );
    }),
  ];
}
