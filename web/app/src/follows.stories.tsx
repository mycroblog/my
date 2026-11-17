// SPDX-License-Identifier: AGPL-3.0-only

// The lists of an account's follows, as the reader (yuki) opens them from its profile's counts: sam's followers and
// follows; mio's, shown to mio's followers only, and kai's, shown to nobody; and a story per state
// every screen is shown in (app.samples.tsx), on sam's followers.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta } from "./app.samples";

const followers = "/@sam/followers";

const meta = {
  title: "Follows",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: followers }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const Followers: Story = {};
export const Following: Story = { parameters: screen({ at: "/@sam/following" }) };
export const FollowersOnly: Story = { parameters: screen({ at: "/@mio/followers" }) };
export const Private: Story = { parameters: screen({ at: "/@kai/following" }) };
export const ListLoading: Story = { parameters: screen({ at: followers, server: { followsLists: "waits" } }) };
export const ListFailed: Story = { parameters: screen({ at: followers, server: { followsLists: "fails" } }) };
export const Empty: Story = { parameters: screen({ at: followers, state: "empty" }) };
export const Loading: Story = { parameters: screen({ at: followers, state: "loading" }) };
export const Failed: Story = { parameters: screen({ at: followers, state: "error" }) };
export const LongText: Story = { parameters: screen({ at: followers, state: "long" }) };
export const RightToLeft: Story = {};
export const MinimumWidth: Story = {};
