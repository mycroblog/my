// SPDX-License-Identifier: AGPL-3.0-only

// A post's page, a story per state: a reply in its thread, a post alone with no replies, a thread folded (both folds),
// and each state every screen is shown in (app.samples.tsx).

import type { Meta, StoryObj } from "@storybook/react-vite";
import { minimumWidth, rightToLeft, screen, screenMeta } from "./app.samples";

const thread = "yuki-ridge-reply";

const meta = {
  title: "Post",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: thread }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const Thread: Story = {};
export const Alone: Story = { parameters: screen({ at: "mio-hello" }) };
/** With both folds: the earlier posts and the replies the page does not show at first. */
export const Folded: Story = { parameters: screen({ at: thread, state: "folded" }) };
export const Empty: Story = { parameters: screen({ at: thread, state: "empty" }) };
export const Loading: Story = { parameters: screen({ at: thread, state: "loading" }) };
export const Failed: Story = { parameters: screen({ at: thread, state: "error" }) };
export const LongText: Story = { parameters: screen({ at: thread, state: "long" }) };
export const RightToLeft: Story = { globals: rightToLeft };
export const MinimumWidth: Story = { globals: minimumWidth };
