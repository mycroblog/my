// SPDX-License-Identifier: AGPL-3.0-only
// The health of remote servers (/moderation/instances; web-client/screens/moderation.md) with its filter, in the
// states of ADR-0112 §1.
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
import { instancesApi, storyInstances } from "./test/instanceStories";
import { moderatorViewer, showing, withoutPermission } from "./test/stories";

const meta = {
  title: "Screens/Moderation/Instances",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: {
      path: "/moderation/instances",
      signedIn: true,
      handlers: [moderatorViewer, ...instancesApi(storyInstances)],
    },
  },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Servers unavailable for days, one on a long domain, and available ones with and without recent failures. */
export const Normal: Story = showing("flaky.example");

/** Only the unavailable servers. */
export const Unavailable: Story = { parameters: { screen: { path: "/moderation/instances?state=unavailable" } } };

/** The server knows no other server yet. */
export const Empty: Story = answering(...instancesApi([]));

/** The servers have not arrived yet. */
export const Loading: Story = loadingOn("get", "/admin/instances");

/** The server failed to answer the servers. */
export const ServerError: Story = failingOn("get", "/admin/instances");

/** A member without manage_domains is told why. */
export const WithoutPermission: Story = withoutPermission;

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
