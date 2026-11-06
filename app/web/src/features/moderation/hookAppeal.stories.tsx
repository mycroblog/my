// SPDX-License-Identifier: AGPL-3.0-only
// An appeal's page of the automated reviews (/moderation/hook-decisions/appeals/{id};
// web-client/screens/moderation-hook-appeal.md): the appeal, its decision and the review, in the states of
// ADR-0079 §13.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { failingOn, inLocale, loadingOn, narrow, renderScreen, screenLoader, settled } from "../../test/stories";
import { hookReviewsApi, storyHookAppeals, storyHookDecisions } from "./test/hookStories";
import { moderatorViewer, showing, storyAccountsHandler, storyId, withoutPermission } from "./test/stories";

const [pending, arabic, upheld] = storyHookAppeals;

function at(appeal: { id: string } | undefined) {
  return { parameters: { screen: { path: `/moderation/hook-decisions/appeals/${appeal?.id ?? ""}` } } };
}

const meta = {
  title: "Screens/Moderation/HookAppeal",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: {
      path: `/moderation/hook-decisions/appeals/${pending?.id ?? ""}`,
      signedIn: true,
      handlers: [moderatorViewer, storyAccountsHandler, ...hookReviewsApi(storyHookDecisions, storyHookAppeals)],
    },
  },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** An appeal awaiting review, with the text and the comment. */
export const Normal: Story = showing(/Come to my bake sale/);

/** An appeal in Arabic without a comment. */
export const RightToLeftText: Story = at(arabic);

/** An upheld appeal. */
export const Upheld: Story = at(upheld);

/** An appeal that does not exist. */
export const NotFound: Story = {
  parameters: { screen: { ...at({ id: storyId("b3", 404) }).parameters.screen, errorPage: "request" } },
};

/** The appeal has not arrived yet. */
export const Loading: Story = loadingOn("get", "/admin/hook_appeals/:id");

/** The server failed to answer the appeal. */
export const ServerError: Story = failingOn("get", "/admin/hook_appeals/:id");

/** Without manage_reports. */
export const WithoutPermission: Story = withoutPermission;

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
