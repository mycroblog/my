// SPDX-License-Identifier: AGPL-3.0-only

// The reader's bookmarks, a place of the navigation: a story per state every screen is shown in (app.samples.tsx),
// with a few posts bookmarked besides the scenario's.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta } from "./app.samples";

const at = "/bookmarks";
const server = { bookmarks: ["layla-jokes", "marco-hello", "sam-poll"] };

const meta = {
  title: "Bookmarks",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at, server }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const Normal: Story = {};
export const Empty: Story = { parameters: screen({ at, server, state: "empty" }) };
export const Loading: Story = { parameters: screen({ at, server, state: "loading" }) };
export const Failed: Story = { parameters: screen({ at, server, state: "error" }) };
export const LongText: Story = { parameters: screen({ at, server, state: "long" }) };
