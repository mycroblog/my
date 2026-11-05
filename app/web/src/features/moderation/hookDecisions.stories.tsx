// SPDX-License-Identifier: AGPL-3.0-only
// The automated reviews (/moderation/hook-decisions; web-client/screens/moderation-hook-decisions.md): each external review
// with its counts, and what they stopped, in the states of ADR-0079 §13.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { apiBase } from "../../test/msw/http";
import { answering, failingOn, inLocale, narrow, renderScreen, screenLoader, settled } from "../../test/stories";
import { hookDecisionFixture, hookSummaryFixture } from "./test/handlers";
import { hoursAgo, listing, moderatorViewer, withoutPermission } from "./test/stories";

const decisions = listing("/admin/hook_decisions", [
  hookDecisionFixture({
    id: "0192f0c4-7a8e-7000-8000-0000000002b3",
    subject_key: "https://remote.example/users/spammer/statuses/1/activity",
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

const meta = {
  title: "Screens/Moderation/HookDecisions",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: { path: "/moderation/hook-decisions", signedIn: true, handlers: [moderatorViewer, summary, decisions] },
  },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** The reviews and what they stopped, would have stopped, or could not check. */
export const Normal: Story = {};

/** Nothing was stopped. */
export const Empty: Story = answering(listing("/admin/hook_decisions", []));

/** The list cannot be read. */
export const Failed: Story = failingOn("get", "/admin/hook_decisions");

/** Without manage_reports. */
export const WithoutPermission: Story = withoutPermission;

/** On a phone. */
export const Narrow: Story = narrow;

/** In Japanese. */
export const Japanese: Story = inLocale("ja");
