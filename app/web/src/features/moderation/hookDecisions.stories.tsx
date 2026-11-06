// SPDX-License-Identifier: AGPL-3.0-only
// The automated reviews (/moderation/hook-decisions; web-client/screens/moderation-hook-decisions.md): each external review
// with its counts, what they stopped, and the authors' appeals of refused posts, in the states of ADR-0079 §13.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { remoteAccountFixture } from "../../test/fixtures";
import { apiBase, problem } from "../../test/msw/http";
import {
  answering,
  answeringWithErrorPage,
  inLocale,
  narrow,
  renderScreen,
  screenLoader,
  settled,
} from "../../test/stories";
import { hookAppealFixture, hookDecisionFixture, hookSummaryFixture } from "./test/handlers";
import { hoursAgo, listing, moderatorViewer, withoutPermission } from "./test/stories";

const decisions = listing("/admin/hook_decisions", [
  hookDecisionFixture({
    id: "0192f0c4-7a8e-7000-8000-0000000002b3",
    subject_key: "https://remote.example/users/spammer/statuses/1/activity",
    account_id: remoteAccountFixture.id,
    reason: "spam",
    note: "Same link posted to many accounts",
    decided_at: hoursAgo(2),
  }),
  hookDecisionFixture({
    id: "0192f0c4-7a8e-7000-8000-0000000002b2",
    subject_key: "https://remote.example/users/someone/statuses/2/activity",
    mode: "observe",
    outcome: "flag",
    overridable: false,
    decided_at: hoursAgo(6),
  }),
  hookDecisionFixture({
    id: "0192f0c4-7a8e-7000-8000-0000000002b1",
    hook: "inbound.follow",
    subject_key: "https://remote.example/users/someone/follows/3",
    outcome: "timeout",
    latency_ms: 3000,
    overridable: false,
    decided_at: hoursAgo(30),
  }),
]);
const summary = http.get(`${apiBase}/admin/hook_decisions/summary`, () => HttpResponse.json(hookSummaryFixture()));
// Two appeals of refused posts await review; the one in Arabic has no comment. Without this handler the feature's
// default one answers that none await review.
const appeals = listing("/admin/hook_appeals", [
  hookAppealFixture({ submitted_at: hoursAgo(1) }),
  {
    ...hookAppealFixture({ id: "0192f0c4-7a8e-7000-8000-0000000003b2", submitted_at: hoursAgo(5) }),
    status_text: "حفلة في الحي يوم السبت",
    comment: "",
  },
]);

const meta = {
  title: "Screens/Moderation/HookDecisions",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: {
      path: "/moderation/hook-decisions",
      signedIn: true,
      handlers: [moderatorViewer, summary, decisions, appeals],
    },
  },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** The appeals awaiting review, the reviews, and what they stopped, would have stopped, or could not check. */
export const Normal: Story = {};

/** Nothing was stopped, and no appeal awaits review. */
export const Empty: Story = answering(moderatorViewer, summary, listing("/admin/hook_decisions", []));

/** The list cannot be read. */
export const Failed: Story = answeringWithErrorPage(
  moderatorViewer,
  summary,
  http.get(`${apiBase}/admin/hook_decisions`, () => problem(500, "about:blank", "Internal Server Error")),
);

/** Without manage_reports. */
export const WithoutPermission: Story = withoutPermission;

/** On a phone. */
export const Narrow: Story = narrow;

/** In Japanese. */
export const Japanese: Story = inLocale("ja");
