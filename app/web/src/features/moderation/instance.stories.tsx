// SPDX-License-Identifier: AGPL-3.0-only
// One remote server's page (/moderation/instances/{domain}; web-client/screens/moderation.md): its health, the
// deliveries to it with their attempts, the history of its availability, and the refetch of its posts, in the states
// of ADR-0079 §13
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
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
import { deliveriesApi, instancesApi, storyInstances } from "./test/instanceStories";
import { moderatorViewer, showing, withoutPermission } from "./test/stories";

const meta = {
  title: "Screens/Moderation/Instance",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: {
      path: "/moderation/instances/gone.example",
      signedIn: true,
      handlers: [moderatorViewer, ...instancesApi(storyInstances), ...deliveriesApi()],
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
export const NotFound: Story = {
  parameters: { screen: { path: "/moderation/instances/unknown.example", errorPage: "request" } },
};

/** The server's health has not arrived yet. */
export const Loading: Story = loadingOn("get", "/admin/instances/:domain");

/** The server failed to answer the health. */
export const ServerError: Story = failingOn("get", "/admin/instances/:domain");

/** The masked fields of a direct post's delivery, shown after the moderator asked for them. */
export const DeliveryRevealed: Story = {
  play: async (context) => {
    const { canvas, userEvent } = context;
    await settled(context);
    const [reveal] = await canvas.findAllByRole("button", { name: "Show masked fields" }, { timeout: 10_000 });
    if (reveal === undefined) throw new Error("no delivery is masked");
    await userEvent.click(reveal);
    await expect(
      await canvas.findByText("The masked fields are shown. Seeing them is recorded in the audit log."),
    ).toBeVisible();
  },
};

/** Nothing was delivered to the server. */
export const NoDeliveries: Story = {
  ...answering(...deliveriesApi([])),
  ...showing("No deliveries to this server"),
};

/** The deliveries have not arrived yet; the rest of the page is there. */
export const DeliveriesLoading: Story = loadingOn("get", "/admin/instances/:domain/deliveries");

/** The server failed to answer the deliveries; their region says so and offers to try again. */
export const DeliveriesError: Story = failingOn("get", "/admin/instances/:domain/deliveries");

/** A member without manage_domains is told why. */
export const WithoutPermission: Story = withoutPermission;

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
