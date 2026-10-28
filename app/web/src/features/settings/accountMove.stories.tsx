// SPDX-License-Identifier: AGPL-3.0-only
// Moving the account to another one (/settings/move; web-client/screens/account-move.md) in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { remoteAccountFixture } from "../../test/fixtures";
import { failingOn, inLocale, loadingOn, narrow, renderScreen, screenLoader, settled, viewerWith } from "../../test/stories";

const meta = {
  title: "Screens/Settings/Move",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/settings/move", signedIn: true } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** An account that has not moved: the form to look the new account up. */
export const Normal: Story = {};

/** An account that moved, with where it moved to and the way to cancel the move. */
export const Moved: Story = { parameters: { screen: { handlers: [viewerWith({ moved: remoteAccountFixture })] } } };

/** The state of the password has not arrived yet. */
export const Loading: Story = loadingOn("get", "/accounts/password");

/** The server failed to answer the state of the password. */
export const ServerError: Story = failingOn("get", "/accounts/password");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
