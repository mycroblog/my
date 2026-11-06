// SPDX-License-Identifier: AGPL-3.0-only
// The appeals page of the automated reviews (/moderation/hook-decisions/appeals;
// web-client/screens/moderation-hook-appeals.md): the authors' appeals of refused posts, in the states of
// ADR-0079 §13.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { answering, failingOn, inLocale, narrow, renderScreen, screenLoader, settled } from "../../test/stories";
import { hookReviewsApi, storyHookAppeals, storyHookDecisions } from "./test/hookStories";
import { moderatorViewer, showing, storyAccountsHandler, withoutPermission } from "./test/stories";

const meta = {
  title: "Screens/Moderation/HookAppeals",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: {
      path: "/moderation/hook-decisions/appeals",
      signedIn: true,
      handlers: [moderatorViewer, storyAccountsHandler, ...hookReviewsApi(storyHookDecisions, storyHookAppeals)],
    },
  },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Two appeals await review, one in Arabic by an account with a right-to-left name. */
export const Normal: Story = showing(/Come to my bake sale/);

/** Every appeal, the upheld one too. */
export const All: Story = { parameters: { screen: { path: "/moderation/hook-decisions/appeals?state=all" } } };

/** No appeal awaits review. */
export const Empty: Story = answering(moderatorViewer, ...hookReviewsApi(storyHookDecisions, []));

/** The list cannot be read. */
export const ServerError: Story = failingOn("get", "/admin/hook_appeals");

/** Without manage_reports. */
export const WithoutPermission: Story = withoutPermission;

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
