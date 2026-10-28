// SPDX-License-Identifier: AGPL-3.0-only
// The appeals of moderation actions (/moderation/appeals; web-client/screens/moderation.md), pending and decided, in
// the states of ADR-0112 §1.
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
import { appealsApi, storyAppeals } from "./test/appealStories";
import { moderatorViewer, showing, storyAccountsHandler, withoutPermission } from "./test/stories";

const meta = {
  title: "Screens/Moderation/Appeals",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: {
      path: "/moderation/appeals",
      signedIn: true,
      handlers: [moderatorViewer, storyAccountsHandler, ...appealsApi(storyAppeals)],
    },
  },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Two pending appeals, of a suspension and of posts marked sensitive. */
export const Normal: Story = showing(/Maximiliana Theodora Wolfharte/);

/** Every appeal, the decided ones too: one upheld, one overturned. */
export const Decided: Story = { parameters: { screen: { path: "/moderation/appeals?state=decided" } } };

/** No appeal waits. */
export const Empty: Story = answering(...appealsApi([]));

/** The appeals have not arrived yet. */
export const Loading: Story = loadingOn("get", "/admin/appeals");

/** The server failed to answer the appeals. */
export const ServerError: Story = failingOn("get", "/admin/appeals");

/** A member without manage_accounts is told why. */
export const WithoutPermission: Story = withoutPermission;

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
