// SPDX-License-Identifier: AGPL-3.0-only
// The reports of the moderation screen stories: a queue with every category, assignee and outcome, filed by local
// and remote accounts with long and right-to-left names, and the handlers that serve them as a list and one by one.
import { http, HttpResponse, type RequestHandler } from "msw";
import {
  instanceFixture,
  longDomainAccountFixture,
  longNameAccountFixture,
  otherAccountFixture,
  remoteAccountFixture,
  rtlNameAccountFixture,
  type Schemas,
  statusFixture,
  viewerFixture,
} from "../../../test/fixtures";
import { apiBase, problem, withViewer } from "../../../test/msw/http";
import { reportFixture } from "./handlers";
import { hoursAgo, monaAccount, storyId } from "./stories";

/** The rules of the server that the stories' reports cite. */
export const storyRules: Schemas["InstanceRule"][] = [
  { id: storyId("f0", 1), text: "Be kind to each other", hint: "Disagree with ideas, not with people." },
  { id: storyId("f0", 2), text: "No advertising or spam" },
  { id: storyId("f0", 3), text: "Mark sensitive media as sensitive" },
];

/** GET /instance with the stories' rules in effect. */
export const instanceWithStoryRules = http.get(`${apiBase}/instance`, () =>
  HttpResponse.json({ ...instanceFixture, rules: storyRules }),
);

function post(n: number, content: string): Schemas["Status"] {
  return { ...statusFixture(storyId("b0", n)), content: `<p>${content}</p>` };
}

/** The report the detail stories open: a rule violation with three posts, one deleted, notes and a history. */
export const detailedReport = reportFixture({
  id: storyId("c0", 1),
  category: "violation",
  comment:
    "This account keeps replying to people who asked to be left alone, and posts the same insult under every thread about the city council meeting. I have muted it, but newcomers keep seeing it.",
  filed_at: hoursAgo(50),
  state_changed_at: hoursAgo(50),
  assignee_account_id: viewerFixture.id,
  reporter_account_id: remoteAccountFixture.id,
  reporter_account: remoteAccountFixture,
  target_account_id: otherAccountFixture.id,
  target_account: otherAccountFixture,
  forwarded: true,
  uri: "https://remote.example/flags/7f3a",
  status_ids: [storyId("b0", 1), storyId("b0", 2), storyId("b0", 3)],
  statuses: [
    { status: post(1, "Nobody asked for your opinion, and nobody ever will.") },
    { status: post(2, "Same reply again, under another thread about the council meeting.") },
    { status: post(3, "A third reply, which its author deleted."), deleted_at: hoursAgo(30) },
  ],
  rule_ids: [storyId("f0", 1), storyId("f0", 2)],
  notes: [
    {
      id: storyId("e0", 1),
      author_account_id: monaAccount.id,
      body: "Second report about this account this month. The first one was dismissed for lack of posts.",
      created_at: hoursAgo(40),
    },
    {
      id: storyId("e0", 2),
      author_account_id: viewerFixture.id,
      body: "Taking this one; I will look at the thread tonight.",
      created_at: hoursAgo(20),
    },
  ],
  target_history: { reports_count: 3, actions_count: 1 },
});

/** The open and resolved reports of the queue, newest first. */
export const storyReports: Schemas["AdminReportDetail"][] = [
  reportFixture({
    id: storyId("c0", 2),
    category: "legal",
    comment: "This post shares a private address and phone number.",
    filed_at: hoursAgo(1),
    state_changed_at: hoursAgo(1),
    target_account_id: longNameAccountFixture.id,
    target_account: longNameAccountFixture,
  }),
  reportFixture({
    id: storyId("c0", 3),
    category: "spam",
    comment: "",
    filed_at: hoursAgo(5),
    state_changed_at: hoursAgo(5),
    reporter_account_id: viewerFixture.id,
    reporter_account: viewerFixture,
    target_account_id: longDomainAccountFixture.id,
    target_account: longDomainAccountFixture,
    status_ids: [],
    statuses: [],
  }),
  { ...detailedReport },
  reportFixture({
    id: storyId("c0", 4),
    category: "other",
    comment: "هذا الحساب ينتحل شخصية صحفي معروف.",
    filed_at: hoursAgo(80),
    state_changed_at: hoursAgo(80),
    assignee_account_id: monaAccount.id,
    reporter_account_id: rtlNameAccountFixture.id,
    reporter_account: rtlNameAccountFixture,
    target_account_id: remoteAccountFixture.id,
    target_account: remoteAccountFixture,
    forwarded: true,
  }),
  reportFixture({
    id: storyId("c0", 5),
    category: "spam",
    filed_at: hoursAgo(130),
    state_changed_at: hoursAgo(130),
  }),
  reportFixture({
    id: storyId("c0", 6),
    category: "violation",
    rule_ids: [storyId("f0", 3)],
    state: "resolved",
    outcome: "actioned",
    filed_at: hoursAgo(200),
    state_changed_at: hoursAgo(190),
    assignee_account_id: monaAccount.id,
  }),
  reportFixture({
    id: storyId("c0", 7),
    category: "spam",
    comment: "Probably a bot.",
    state: "resolved",
    outcome: "dismissed",
    filed_at: hoursAgo(260),
    state_changed_at: hoursAgo(250),
    target_account_id: rtlNameAccountFixture.id,
    target_account: rtlNameAccountFixture,
  }),
];

/** The fields of a report that the queue lists; the details stay on its own page. */
export function reportSummary({
  notes: _notes,
  target_history: _history,
  statuses: _statuses,
  ...report
}: Schemas["AdminReportDetail"]): Schemas["AdminReport"] {
  return report;
}

/**
 * The admin reports API over `reports`: the list filtered by state, category and assignee and ordered as asked,
 * and each report by its ID.
 */
export function reportsApi(reports: Schemas["AdminReportDetail"][]): RequestHandler[] {
  return [
    http.get(`${apiBase}/admin/reports`, ({ request }) => {
      const query = new URL(request.url).searchParams;
      const assignee = query.get("assignee");
      const category = query.get("category");
      const kept = reports.filter(
        (report) =>
          report.state === (query.get("state") ?? "open") &&
          (category === null || report.category === category) &&
          (assignee === null ||
            (assignee === "me" ? report.assignee_account_id === viewerFixture.id : !report.assignee_account_id)),
      );
      const ordered = query.get("order") === "oldest" ? kept.toReversed() : kept;
      return withViewer(request, () => HttpResponse.json({ items: ordered.map(reportSummary) }));
    }),
    http.get(`${apiBase}/admin/reports/:id`, ({ request, params }) => {
      const report = reports.find((r) => r.id === params["id"]);
      return withViewer(request, () =>
        report === undefined ? problem(404, "/api/v1/problems/not-found", "Not Found") : HttpResponse.json(report),
      );
    }),
  ];
}
