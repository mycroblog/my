// SPDX-License-Identifier: AGPL-3.0-only

// The landing page, at / for a visitor without a session (shell/landing.tsx), on the fake server (app.samples.tsx): a
// server open to anyone, whose local and federated timelines everyone may read; one whose new accounts wait for a
// moderator, one that takes them with an invite alone, and one closed to them; a summary that runs long; a server that
// shows its timelines to signed-in readers alone; its own logo, and its thumbnail as the banner; right to left; and the
// minimum width.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { minimumWidth, rightToLeft, screen, screenMeta } from "./app.samples";

const meta = {
  title: "Landing",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: "/", signedOut: true }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

const visitor = (server: NonNullable<Parameters<typeof screen>[0]["server"]>) =>
  screen({ at: "/", signedOut: true, server });

export const Open: Story = {};
export const Approval: Story = { parameters: visitor({ registrations: "approval" }) };
export const Invite: Story = { parameters: visitor({ registrations: "invite" }) };
/** Closed to new accounts: no Create an account, and Sign in is the primary action. */
export const Closed: Story = { parameters: visitor({ registrations: "closed" }) };
export const LongSummary: Story = { parameters: visitor({ longSummary: true }) };
/** No timeline shown to everyone: nothing to read without an account but search. */
export const NoPublicTimelines: Story = {
  parameters: visitor({ timelines: { local: "signed_in", federated: "signed_in" } }),
};
export const Logo: Story = { parameters: visitor({ logo: true }) };
/** The thumbnail, the image that represents the server, as the introduction's banner. */
export const Thumbnail: Story = { parameters: visitor({ thumbnail: true, logo: true, registrations: "approval" }) };
export const RightToLeft: Story = { globals: rightToLeft };
export const MinimumWidth: Story = { globals: minimumWidth };
