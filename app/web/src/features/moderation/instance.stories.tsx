// SPDX-License-Identifier: AGPL-3.0-only
// One remote server's page (/moderation/instances/{domain}; web-client/screens/moderation.md): its health, the
// history of its availability, and the refetch of its posts, in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { longDomainAccountFixture } from "../../test/fixtures";
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
  title: "Screens/Moderation/Instance",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: {
      path: "/moderation/instances/gone.example",
      signedIn: true,
      handlers: [moderatorViewer, ...instancesApi(storyInstances)],
    },
  },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** A server unavailable for nine days, with the history of its availability. */
export const Normal: Story = showing(/gone\.example/);

/** An available server on a long domain, without any history. */
export const LongDomain: Story = {
  ...answering(...instancesApi([{ domain: longDomainAccountFixture.domain, state: "available", failure_days: 0 }], [])),
  parameters: { screen: { path: `/moderation/instances/${longDomainAccountFixture.domain}` } },
};

/** A server this one does not know. */
export const NotFound: Story = { parameters: { screen: { path: "/moderation/instances/unknown.example" } } };

/** The server's health has not arrived yet. */
export const Loading: Story = loadingOn("get", "/admin/instances/:domain");

/** The server failed to answer the health. */
export const ServerError: Story = failingOn("get", "/admin/instances/:domain");

/** A member without manage_domains is told why. */
export const WithoutPermission: Story = withoutPermission;

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
