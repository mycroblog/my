// SPDX-License-Identifier: AGPL-3.0-only

// What waits for the reader's approval (ADR-0155), the screen the row over the notifications opens: a reply, a boost
// and a quote of the reader's post, each with Decline and Approve, on the fake server's posts that approve by hand
// (`approvals`); and each state every screen is shown in (app.samples.tsx). The answer in a request's place, right
// after Approve or Decline, is the row's own state (the design system's Follow requests).

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta } from "./app.samples";

const at = "/notifications/waiting";
const server = { approvals: true };

const meta = {
  title: "Waiting for approval",
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
