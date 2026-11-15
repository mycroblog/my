// SPDX-License-Identifier: AGPL-3.0-only

// The direct messages (FR-212): the list of the reader's conversations, one unread and one read; the list with none;
// the list just after the reader hid one; a conversation of two and one of a group, with the reply in place at its end;
// each at the minimum width and at a PC's, in English and Japanese, with a play that checks what it shows
// (conversations.plays.ts); and the states every screen is shown in (app.samples.tsx).

import type { Meta, StoryContext, StoryObj } from "@storybook/react-vite";
import { userEvent, waitFor, within } from "storybook/test";
import { minimumWidth, rightToLeft, screen, screenMeta } from "./app.samples";
import * as plays from "./conversations.plays";

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

const pc = { viewport: { value: "pc" } };
const ja = { locale: "ja" };

/** The story of `at` (in `state`) at the minimum width and a PC's, in English and Japanese. */
const sizes = (parameters: ReturnType<typeof screen>, play?: Story["play"]) => {
  const played = play === undefined ? {} : { play };
  return {
    minimum: { parameters, globals: minimumWidth, ...played } satisfies Story,
    minimumJa: { parameters, globals: { ...minimumWidth, ...ja }, ...played } satisfies Story,
    pc: { parameters, globals: pc, ...played } satisfies Story,
    pcJa: { parameters, globals: { ...pc, ...ja }, ...played } satisfies Story,
  };
};

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

const listed = sizes(screen({ at: list }), plays.listed);
export const List: Story = listed.minimum;
export const ListJa: Story = listed.minimumJa;
export const ListPc: Story = listed.pc;
export const ListPcJa: Story = listed.pcJa;

const empty = sizes(screen({ at: list, state: "empty" }), plays.none);
export const Empty: Story = empty.minimum;
export const EmptyJa: Story = empty.minimumJa;
export const EmptyPc: Story = empty.pc;
export const EmptyPcJa: Story = empty.pcJa;

const hidden = sizes(screen({ at: list }), async (context) => {
  await hideFirst(context);
  await plays.hiddenOne(context);
});
export const Hidden: Story = hidden.minimum;
export const HiddenJa: Story = hidden.minimumJa;
export const HiddenPc: Story = hidden.pc;
export const HiddenPcJa: Story = hidden.pcJa;

const ofTwo = sizes(screen({ at: two }), plays.withSam);
export const Conversation: Story = ofTwo.minimum;
export const ConversationJa: Story = ofTwo.minimumJa;
export const ConversationPc: Story = ofTwo.pc;
export const ConversationPcJa: Story = ofTwo.pcJa;

const ofGroup = sizes(screen({ at: group }), plays.withGroup);
export const Group: Story = ofGroup.minimum;
export const GroupJa: Story = ofGroup.minimumJa;
export const GroupPc: Story = ofGroup.pc;
export const GroupPcJa: Story = ofGroup.pcJa;

export const Loading: Story = { parameters: screen({ at: list, state: "loading" }) };
export const Failed: Story = { parameters: screen({ at: list, state: "error" }) };
export const LongText: Story = { parameters: screen({ at: list, state: "long" }) };
export const RightToLeft: Story = { globals: rightToLeft };
export const ConversationLoading: Story = { parameters: screen({ at: two, state: "loading" }) };
export const ConversationFailed: Story = { parameters: screen({ at: two, state: "error" }) };
export const ConversationLongText: Story = { parameters: screen({ at: group, state: "long" }) };
export const ConversationRightToLeft: Story = { parameters: screen({ at: group }), globals: rightToLeft };
