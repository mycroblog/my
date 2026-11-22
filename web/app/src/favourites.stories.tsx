// SPDX-License-Identifier: AGPL-3.0-only

// The reader's favourites, a place of the navigation: a story per state every screen is shown in (app.samples.tsx),
// with a few posts favourited besides the scenario's.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta } from "./app.samples";

const at = "/favourites";
const server = { favourites: ["marco-hello", "sam-poll", "layla-rtl"] };

const meta = {
  title: "Favourites",
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
