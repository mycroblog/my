// SPDX-License-Identifier: AGPL-3.0-only
// The report queue (/moderation/reports; web-client/screens/moderation.md) with its tabs and filters, in the states
// of ADR-0112 §1.
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
import { reportsApi, storyReports } from "./test/reportStories";
import { moderatorViewer, showing, storyAccountsHandler, withoutPermission } from "./test/stories";

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
