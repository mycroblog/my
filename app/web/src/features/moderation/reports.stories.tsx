// SPDX-License-Identifier: AGPL-3.0-only
// The report queue (/moderation/reports; web-client/screens/moderation.md) with its tabs and filters, in the states
// of ADR-0079 §13
import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  answering,
  failingOn,
  inLocale,
  loadingOn,
  narrow,
  renderScreen,
  screenLoader,
  settled,
} from "../../test/stories";
import { accountFixture, type Schemas } from "../../test/fixtures";
import { alice, hana, kofi, lena, minori, showcaseSettled, tomas } from "../../test/showcase";
import { reportFixture } from "./test/handlers";
import { reportsApi, storyReports } from "./test/reportStories";
import {
  hoursAgo,
  monaAccount,
  moderatorViewer,
  showing,
  storyAccountsHandler,
  storyId,
  withoutPermission,
} from "./test/stories";

/** An account of another server that the showcase's reports are about. */
function reported(n: number, username: string, displayName: string, domain: string): Schemas["Account"] {
  return { ...accountFixture(n, username, domain), display_name: displayName };
}

/** Open report `n` of the showcase, filed `hours` ago by `reporter` about `target`, assigned to `assignee` if any. */
function showcaseReport(
  n: number,
  hours: number,
  reporter: Schemas["Account"],
  target: Schemas["Account"],
  overrides: Partial<Schemas["AdminReportDetail"]>,
): Schemas["AdminReportDetail"] {
  return reportFixture({
    id: storyId("c1", n),
    filed_at: hoursAgo(hours),
    state_changed_at: hoursAgo(hours),
    reporter_account_id: reporter.id,
    reporter_account: reporter,
    target_account_id: target.id,
    target_account: target,
    ...overrides,
  });
}

/**
 * The open reports of a busy server (test/showcase.ts): filed by its people about accounts of other servers, in each
 * category, two of them assigned.
 */
const showcaseReports: Schemas["AdminReportDetail"][] = [
  showcaseReport(1, 1, minori, reported(94, "bestdeals", "Best Deals Daily", "deals.example"), {
    category: "spam",
    comment: "Posts the same shop link under every popular post.",
  }),
  showcaseReport(2, 4, tomas, reported(95, "nightowl", "night owl", "chatter.example"), {
    category: "violation",
    comment: "Keeps replying to people who asked to be left alone.",
    rule_ids: [storyId("f0", 1)],
    assignee_account_id: alice.id,
  }),
  showcaseReport(3, 9, hana, reported(96, "leaky", "Leaky", "dump.example"), {
    category: "legal",
    comment: "This post shares someone's home address.",
  }),
  showcaseReport(4, 30, lena, reported(97, "admin_support", "Admin Support", "phish.example"), {
    category: "other",
    comment: "Pretends to be the admin of this server and asks for passwords.",
    assignee_account_id: monaAccount.id,
    forwarded: true,
  }),
  showcaseReport(5, 52, kofi, reported(98, "follow4follow", "Follow Back", "bots.example"), {
    category: "spam",
    comment: "Probably a bot: a hundred follows a minute.",
  }),
];

const meta = {
  title: "Screens/Moderation/Reports",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: {
      path: "/moderation/reports",
      signedIn: true,
      handlers: [moderatorViewer, storyAccountsHandler, ...reportsApi(storyReports)],
    },
  },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** The open reports, newest first: every category, assigned to the viewer, to another moderator, or to no one. */
export const Normal: Story = showing(/@maximiliana/);

/** The queue of a busy server, without the edge cases of Normal. The documentation site's screenshots show it. */
export const Showcase: Story = {
  ...answering(moderatorViewer, storyAccountsHandler, ...reportsApi(showcaseReports)),
  play: showcaseSettled,
};

/** The resolved tab: one report handled by an action, one dismissed. */
export const Resolved: Story = { parameters: { screen: { path: "/moderation/reports?state=resolved" } } };

/** Filtered to the unassigned spam reports, oldest first. */
export const Filtered: Story = {
  parameters: { screen: { path: "/moderation/reports?category=spam&assignee=none&order=oldest" } },
};

/** Filters that nothing matches. */
export const NoMatch: Story = {
  parameters: { screen: { path: "/moderation/reports?category=legal&assignee=me" } },
};

/** No report is open. */
export const Empty: Story = answering(...reportsApi([]));

/** The reports have not arrived yet. */
export const Loading: Story = loadingOn("get", "/admin/reports");

/** The server failed to answer the reports. */
export const ServerError: Story = failingOn("get", "/admin/reports");

/** A member without manage_reports is told why. */
export const WithoutPermission: Story = withoutPermission;

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
