// SPDX-License-Identifier: AGPL-3.0-only

// The timeline of a hashtag, reached from a hashtag in a post: a story per state every screen is shown in
// (app.samples.tsx), of #baking; a hashtag written in another script; a visitor's; and the minimum width and right to
// left.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta } from "./app.samples";

const at = "/tags/baking";

const meta = {
  title: "Hashtag",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const Normal: Story = {};
export const OtherScript: Story = { parameters: screen({ at: `/tags/${encodeURIComponent("山")}` }) };
export const Empty: Story = { parameters: screen({ at, state: "empty" }) };
export const Loading: Story = { parameters: screen({ at, state: "loading" }) };
export const Failed: Story = { parameters: screen({ at, state: "error" }) };
export const LongText: Story = { parameters: screen({ at, state: "long" }) };
export const Visitor: Story = { parameters: screen({ at, signedOut: true }) };
