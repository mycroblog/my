// SPDX-License-Identifier: AGPL-3.0-only

// The direct messages (FR-212): the list of the reader's conversations, one unread and one read; the list with none;
// a conversation of two and one of a group, with the reply in place at its end; and the states every screen is shown
// in (app.samples.tsx). The list just after the reader hid a conversation is the list's own state (the app
// components' Direct messages).

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta } from "./app.samples";

const list = "/conversations";
/** The scenario's conversations, newest first: the reader's group with Hana and Layla, then theirs with Sam. */
const group = "conversation:0";
const two = "conversation:1";

const meta = {
  title: "Direct messages",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: list }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The list: the group's conversation, then Sam's, which is unread and which the navigation counts. */
export const List: Story = {};
/** No conversation: a line says how to send a direct message. */
export const Empty: Story = { parameters: screen({ at: list, state: "empty" }) };
/** Sam's conversation: its posts oldest first, the reply in place. */
export const Conversation: Story = { parameters: screen({ at: two }) };
/** The group's conversation: Hana's, Layla's and the reader's posts, oldest first, with the reply in place. */
export const Group: Story = { parameters: screen({ at: group }) };

export const Loading: Story = { parameters: screen({ at: list, state: "loading" }) };
export const Failed: Story = { parameters: screen({ at: list, state: "error" }) };
export const LongText: Story = { parameters: screen({ at: list, state: "long" }) };
export const ConversationLoading: Story = { parameters: screen({ at: two, state: "loading" }) };
export const ConversationFailed: Story = { parameters: screen({ at: two, state: "error" }) };
export const ConversationLongText: Story = { parameters: screen({ at: group, state: "long" }) };
