// SPDX-License-Identifier: AGPL-3.0-only
// An appeal's page (/moderation/appeals/{id}; web-client/screens/moderation.md): the appeal, the action it is about,
// and the decision, in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { failingOn, inLocale, loadingOn, narrow, renderScreen, screenLoader, settled } from "../../test/stories";
import { appealsApi, storyAppeals } from "./test/appealStories";
import { moderatorViewer, showing, storyAccountsHandler, storyId, withoutPermission } from "./test/stories";

const [pending, , upheld, overturned] = storyAppeals;

function at(appeal: { id: string } | undefined) {
  return { parameters: { screen: { path: `/moderation/appeals/${appeal?.id ?? ""}` } } };
}

const meta = {
  title: "Screens/Moderation/Appeal",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: {
      path: `/moderation/appeals/${pending?.id ?? ""}`,
      signedIn: true,
      handlers: [moderatorViewer, storyAccountsHandler, ...appealsApi(storyAppeals)],
    },
  },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** A pending appeal of a suspension, with a long text, awaiting the decision. */
export const Normal: Story = showing(/I posted the link to my own blog once/);

/** An appeal upheld, with the reason, by an account with a right-to-left name. */
export const Upheld: Story = at(upheld);

/** An appeal overturned, which lifted the action. */
export const Overturned: Story = at(overturned);

/** An appeal that does not exist. */
export const NotFound: Story = {
  parameters: { screen: { ...at({ id: storyId("f1", 404) }).parameters.screen, errorPage: "request" } },
};

/** The appeal has not arrived yet. */
export const Loading: Story = loadingOn("get", "/admin/appeals/:id");

/** The server failed to answer the appeal. */
export const ServerError: Story = failingOn("get", "/admin/appeals/:id");

/** A member without manage_accounts is told why. */
export const WithoutPermission: Story = withoutPermission;

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
