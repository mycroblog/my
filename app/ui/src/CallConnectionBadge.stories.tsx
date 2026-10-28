// SPDX-License-Identifier: AGPL-3.0-only
// CallConnectionBadge stories: the user's connection to a call while connecting, connected, and reconnecting.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { CallConnectionBadge } from "./CallConnectionBadge";
import { Cluster } from "./Cluster";

const meta = {
  title: "Voice/CallConnectionBadge",
  component: CallConnectionBadge,
  args: { state: "connected", label: "Connected" },
} satisfies Meta<typeof CallConnectionBadge>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Connected: Story = {
  play: async ({ canvas }) => {
    await expect(canvas.getByText("Connected")).toBeVisible();
  },
};

/** Each state has its own shape and tone beside its name. */
export const AllStates: Story = {
  render: () => (
    <Cluster gap="2">
      <CallConnectionBadge state="connecting" label="Connecting…" />
      <CallConnectionBadge state="connected" label="Connected" />
      <CallConnectionBadge state="reconnecting" label="Reconnecting…" />
    </Cluster>
  ),
  play: async ({ canvas }) => {
    await expect(canvas.getByText("Reconnecting…")).toHaveAttribute("data-connection", "reconnecting");
  },
};
