// SPDX-License-Identifier: AGPL-3.0-only
// The moderation overview (/moderation; web-client/screens/moderation.md) in the states of ADR-0112 §1.
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
import { countsHandler } from "./test/handlers";
import { reportsApi, storyReports } from "./test/reportStories";
import { hoursAgo, memberViewer, moderatorViewer, storyAccountsHandler } from "./test/stories";

const waiting = countsHandler({
  open_reports: 5,
  pending_registrations: 3,
  pending_appeals: 2,
  oldest_open_report_filed_at: hoursAgo(130),
});

const meta = {
  title: "Screens/Moderation/Overview",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: {
      path: "/moderation",
      signedIn: true,
      handlers: [moderatorViewer, storyAccountsHandler, waiting, ...reportsApi(storyReports)],
    },
  },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Reports, registrations and appeals waiting, the oldest report for days; the reports to handle first below. */
export const Normal: Story = {};

/** Nothing waits for the moderators. */
export const Empty: Story = answering(countsHandler({ open_reports: 0 }), ...reportsApi([]));

/** The counts have not arrived yet. */
export const Loading: Story = loadingOn("get", "/admin/counts");

/** The server failed to answer the reports to handle first. */
export const ServerError: Story = failingOn("get", "/admin/reports");

/** A member without a moderation role. */
export const WithoutPermission: Story = answering(memberViewer);

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
