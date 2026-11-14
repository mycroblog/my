// SPDX-License-Identifier: AGPL-3.0-only

// A profile, a story per relationship the reader (yuki) meets it in: their own, following each other, locked and
// followed, following them, blocked and muted; and a story per state every screen is shown in (app.samples.tsx), on a
// profile of another.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { minimumWidth, rightToLeft, screen, screenMeta } from "./app.samples";

const other = "/@sam";

const meta = {
  title: "Profile",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: "/profile" }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const Own: Story = {};
export const FollowingEachOther: Story = { parameters: screen({ at: other }) };
export const LockedFollowing: Story = { parameters: screen({ at: "/@ren" }) };
export const FollowsYou: Story = { parameters: screen({ at: "/@mio" }) };
export const Blocked: Story = { parameters: screen({ at: "/@quickdeals" }) };
export const Muted: Story = { parameters: screen({ at: "/@kai" }) };
export const Empty: Story = { parameters: screen({ at: other, state: "empty" }) };
export const Loading: Story = { parameters: screen({ at: other, state: "loading" }) };
export const Failed: Story = { parameters: screen({ at: other, state: "error" }) };
export const LongText: Story = { parameters: screen({ at: other, state: "long" }) };
export const RightToLeft: Story = { parameters: screen({ at: other }), globals: rightToLeft };
export const MinimumWidth: Story = { parameters: screen({ at: other }), globals: minimumWidth };
