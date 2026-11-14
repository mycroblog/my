// SPDX-License-Identifier: AGPL-3.0-only

// The notifications, a story per state every screen is shown in (app.samples.tsx), fed new notifications from the
// Stream panel.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { minimumWidth, rightToLeft, screen, screenMeta } from "./app.samples";

const at = "/notifications";

const meta = {
  title: "Notifications",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at }, "notifications") },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const Normal: Story = {};
export const Empty: Story = { parameters: screen({ at, state: "empty" }, "notifications") };
export const Loading: Story = { parameters: screen({ at, state: "loading" }, "notifications") };
export const Failed: Story = { parameters: screen({ at, state: "error" }, "notifications") };
export const LongText: Story = { parameters: screen({ at, state: "long" }, "notifications") };
export const RightToLeft: Story = { globals: rightToLeft };
export const MinimumWidth: Story = { globals: minimumWidth };
