// SPDX-License-Identifier: AGPL-3.0-only

// Who reacted to a post with one emoji, a story per state: a Unicode emoji, a custom one (its image in the title), and
// each state every screen is shown in (app.samples.tsx).
// More of who reacted than a page holds, before and after Show more (pages.samples.ts, `more`).

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta } from "./app.samples";
import { manyReactors } from "./pages.samples";

const party = "reactions:yuki-ridge:🎉";

const meta = {
  title: "Reactions",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: party }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const Unicode: Story = {};
export const CustomEmoji: Story = { parameters: screen({ at: "reactions:yuki-ridge:sparkle" }) };
export const Empty: Story = { parameters: screen({ at: party, state: "empty" }) };
export const Loading: Story = { parameters: screen({ at: party, state: "loading" }) };
export const Failed: Story = { parameters: screen({ at: party, state: "error" }) };
export const LongText: Story = { parameters: screen({ at: party, state: "long" }) };
/** More who reacted than a page holds: Show more under the first twenty. */
export const Many: Story = { parameters: screen({ at: party, server: manyReactors }) };
/** The same after Show more: the rest under the first twenty, and no Show more. */
export const ManyMore: Story = { parameters: screen({ at: party, server: manyReactors, more: true }) };
