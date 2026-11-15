// SPDX-License-Identifier: AGPL-3.0-only

// The reader's lists, which the menu leads to, and a list's timeline: a story per state every screen is shown in
// (app.samples.tsx), of each; a list deleted; and the minimum width and right to left.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { minimumWidth, rightToLeft, screen, screenMeta } from "./app.samples";

const index = "/lists";
const list = "list:0";

const meta = {
  title: "Lists",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: index }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const Index: Story = {};
export const IndexEmpty: Story = { parameters: screen({ at: index, state: "empty" }) };
export const IndexLoading: Story = { parameters: screen({ at: index, state: "loading" }) };
export const IndexFailed: Story = { parameters: screen({ at: index, state: "error" }) };
export const IndexMinimumWidth: Story = { globals: minimumWidth };
export const ListTimeline: Story = { parameters: screen({ at: list }) };
export const ListEmpty: Story = { parameters: screen({ at: list, state: "empty" }) };
export const ListLoading: Story = { parameters: screen({ at: list, state: "loading" }) };
export const ListFailed: Story = { parameters: screen({ at: list, state: "error" }) };
export const ListGone: Story = { parameters: screen({ at: "/lists/gone" }) };
export const ListLongText: Story = { parameters: screen({ at: "list:2", state: "long" }) };
export const ListRightToLeft: Story = { parameters: screen({ at: list }), globals: rightToLeft };
export const ListMinimumWidth: Story = { parameters: screen({ at: "list:2" }), globals: minimumWidth };
