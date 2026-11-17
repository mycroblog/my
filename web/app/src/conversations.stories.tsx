// SPDX-License-Identifier: AGPL-3.0-only

// The direct messages (FR-212): the list of the reader's conversations, one unread and one read; the list with none;
// the list just after the reader hid one, which follows a press: a play hides it; a conversation of two and one of a
// group, with the reply in place at its end; and the states every screen is shown in (app.samples.tsx).

import type { Meta, StoryContext, StoryObj } from "@storybook/react-vite";
import { userEvent, waitFor, within } from "storybook/test";
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

/** Hides the first conversation of the list from its more menu, and waits for the line that says it comes back. */
async function hideFirst(context: StoryContext) {
  await screenMeta.play(context);
  const { canvasElement } = context;
  const more = canvasElement.querySelector<HTMLElement>('main article button[aria-haspopup="true"]');
  if (more === null) throw new Error("no conversation offers more");
  await userEvent.click(more);
  const [hide] = await within(canvasElement.ownerDocument.body).findAllByRole("menuitem");
  if (hide === undefined) throw new Error("the menu offers nothing");
  await userEvent.click(hide);
  await waitFor(() => {
    if (
      canvasElement.querySelector("main article") === null ||
      canvasElement.querySelectorAll("main article").length > 1
    )
      throw new Error("the conversation is not hidden yet");
  });
}

/** The list: the group's conversation, then Sam's, which is unread and which the navigation counts. */
export const List: Story = {};
/** No conversation: a line says how to send a direct message. */
export const Empty: Story = { parameters: screen({ at: list, state: "empty" }) };
/** Just after the group's conversation was hidden: Sam's alone, under a line that names the group's people. */
export const Hidden: Story = { play: hideFirst };
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
