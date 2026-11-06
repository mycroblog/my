// SPDX-License-Identifier: AGPL-3.0-only
// A decision's page of the automated reviews (/moderation/hook-decisions/{id};
// web-client/screens/moderation-hook-decision.md): every fact of a decision and its cancellation, in the states of
// ADR-0079 §13.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { failingOn, inLocale, loadingOn, narrow, renderScreen, screenLoader, settled } from "../../test/stories";
import { hookReviewsApi, storyHookAppeals, storyHookDecisions } from "./test/hookStories";
import { moderatorViewer, showing, storyAccountsHandler, storyId, withoutPermission } from "./test/stories";

const [dropped, observed] = storyHookDecisions;

function at(decision: { id: string } | undefined) {
  return { parameters: { screen: { path: `/moderation/hook-decisions/${decision?.id ?? ""}` } } };
}

const meta = {
  title: "Screens/Moderation/HookDecision",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: {
      path: `/moderation/hook-decisions/${dropped?.id ?? ""}`,
      signedIn: true,
      handlers: [moderatorViewer, storyAccountsHandler, ...hookReviewsApi(storyHookDecisions, storyHookAppeals)],
    },
  },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** A drop in force with its account, reason and note, which can be cancelled. */
export const Normal: Story = showing(/Same link posted to many accounts/);

/** A flag in observe mode, which had no effect and has nothing to cancel. */
export const NoEffect: Story = at(observed);

/** A decision that does not exist. */
export const NotFound: Story = {
  parameters: { screen: { ...at({ id: storyId("b2", 404) }).parameters.screen, errorPage: "request" } },
};

/** The decision has not arrived yet. */
export const Loading: Story = loadingOn("get", "/admin/hook_decisions/:id");

/** The server failed to answer the decision. */
export const ServerError: Story = failingOn("get", "/admin/hook_decisions/:id");

/** Without manage_reports. */
export const WithoutPermission: Story = withoutPermission;

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
