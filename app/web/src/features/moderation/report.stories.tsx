// SPDX-License-Identifier: AGPL-3.0-only
// A report's page (/moderation/reports/{id}; web-client/screens/moderation.md): the report, its posts, its target
// and the target's history, the notes, and the action form, in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { failingOn, inLocale, loadingOn, narrow, renderScreen, screenLoader, settled } from "../../test/stories";
import { accountActionFixture } from "./test/handlers";
import { detailedReport, instanceWithStoryRules, reportsApi, storyReports } from "./test/reportStories";
import {
  hoursAgo,
  listing,
  moderatorViewer,
  showing,
  storyAccountsHandler,
  storyId,
  withoutPermission,
} from "./test/stories";

/** The target's earlier action: a limit that expired. */
const targetActions = listing("/admin/accounts/:id/actions", [
  accountActionFixture({
    id: storyId("d0", 1),
    kind: "limit",
    reason_category: "violation",
    public_reason: "Insults in replies.",
    state: "expired",
    issued_at: hoursAgo(24 * 40),
    expires_at: hoursAgo(24 * 33),
  }),
]);

const meta = {
  title: "Screens/Moderation/Report",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: {
      path: `/moderation/reports/${detailedReport.id}`,
      signedIn: true,
      handlers: [
        moderatorViewer,
        storyAccountsHandler,
        instanceWithStoryRules,
        targetActions,
        ...reportsApi(storyReports),
      ],
    },
  },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** An open rule violation the viewer handles: three posts, one deleted, two notes, and the target's history. */
export const Normal: Story = showing("Taking this one; I will look at the thread tonight.");

/** A report of spam without a comment or posts, on an account of a long domain. */
export const Sparse: Story = { parameters: { screen: { path: `/moderation/reports/${storyId("c0", 3)}` } } };

/** A report resolved by an action. */
export const Resolved: Story = { parameters: { screen: { path: `/moderation/reports/${storyId("c0", 6)}` } } };

/** A report that does not exist. */
export const NotFound: Story = { parameters: { screen: { path: `/moderation/reports/${storyId("c0", 404)}`, errorPage: "request" } } };

/** The report has not arrived yet. */
export const Loading: Story = loadingOn("get", "/admin/reports/:id");

/** The server failed to answer the report. */
export const ServerError: Story = failingOn("get", "/admin/reports/:id");

/** A member without manage_reports is told why. */
export const WithoutPermission: Story = withoutPermission;

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
