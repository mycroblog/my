// SPDX-License-Identifier: AGPL-3.0-only

// The home timeline, the app's first screen, a story per state every screen is shown in (app.samples.tsx), fed new
// posts and reconnections from the Stream panel; and a first page that fails once and loads on its retry.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { minimumWidth, rightToLeft, screen, screenMeta } from "./app.samples";

const meta = {
  title: "Timeline",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: "/" }, "home") },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const Normal: Story = {};
export const Empty: Story = { parameters: screen({ at: "/", state: "empty" }, "home") };
export const Loading: Story = { parameters: screen({ at: "/", state: "loading" }, "home") };
export const Failed: Story = { parameters: screen({ at: "/", state: "error" }, "home") };
export const LongText: Story = { parameters: screen({ at: "/", state: "long" }, "home") };
export const FailsFirst: Story = { parameters: screen({ at: "/", failFirst: true }, "home") };
export const RightToLeft: Story = { globals: rightToLeft };
export const MinimumWidth: Story = { globals: minimumWidth };
