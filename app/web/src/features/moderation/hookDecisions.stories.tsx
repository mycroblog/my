// SPDX-License-Identifier: AGPL-3.0-only
// The first page of the automated reviews (/moderation/hook-decisions; web-client/screens/moderation-hook-decisions.md):
// each external review with its counts and what they stopped, in the states of ADR-0079 §13.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
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
import { hookSummaryFixture } from "./test/handlers";
import { storyHookDecisions } from "./test/hookStories";
import { listing, moderatorViewer, withoutPermission } from "./test/stories";

const decisions = listing("/admin/hook_decisions", storyHookDecisions);
const summary = http.get(`${apiBase}/admin/hook_decisions/summary`, () => HttpResponse.json(hookSummaryFixture()));

const meta = {
  title: "Screens/Moderation/HookDecisions",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: {
      path: "/moderation/hook-decisions",
      signedIn: true,
      handlers: [moderatorViewer, summary, decisions],
    },
  },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** The reviews, and what they stopped, would have stopped, or could not check. */
export const Normal: Story = {};

/** Nothing was stopped. */
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
