// SPDX-License-Identifier: AGPL-3.0-only

// What waits for the reader's approval (ADR-0155), the screen the row over the notifications opens: a reply, a boost
// and a quote of the reader's post, each with Decline and Approve, on the fake server's posts that approve by hand
// (`approvals`); each state every screen is shown in (app.samples.tsx); and the screen right after Approve, the answer
// in the reply's place with the focus on it.

import type { Meta, StoryContext, StoryObj } from "@storybook/react-vite";
import { userEvent, waitFor, within } from "storybook/test";
import { minimumWidth, rightToLeft, screen, screenMeta } from "./app.samples";

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
export const RightToLeft: Story = { globals: rightToLeft };
export const MinimumWidth: Story = { globals: minimumWidth };

/**
 * Approves the reply (the oldest request, last on the screen) as the reader presses it, in the toolbar's locale, and
 * waits until its answer took the focus.
 */
async function approved(context: StoryContext) {
  await screenMeta.play(context);
  const main = within(context.canvasElement.querySelector("main") ?? context.canvasElement);
  const feed = await main.findByRole("feed");
  const entries = await waitFor(() => {
    const found = within(feed).getAllByRole("article");
    if (found.length < 3) throw new Error("the requests have not all come yet");
    return found;
  });
  const reply = entries.at(-1);
  if (reply === undefined) throw new Error("no request");
  // Approve is the request's primary button, the second of its pair in any locale.
  const approve = within(reply).getAllByRole("button").at(-1);
  if (approve === undefined) throw new Error("the request has no buttons");
  await userEvent.click(approve);
  await waitFor(() => {
    const focused = context.canvasElement.ownerDocument.activeElement;
    if (focused === null || !reply.contains(focused) || within(reply).queryAllByRole("button").length > 0)
      throw new Error("the answer has not taken the focus yet");
    // The answer it took the focus with, Approved (notifications.approved), in en and ja; any other locale (the
    // pseudo-locale of right to left) says it in its own words.
    const lang = context.canvasElement.ownerDocument.documentElement.lang;
    const approved = lang.startsWith("ja") ? "承認しました" : lang.startsWith("en") ? "Approved" : undefined;
    const said = focused.textContent;
    if (approved === undefined ? said === "" : said !== approved)
      throw new Error(`the focused answer says "${said}", not ${approved ?? "anything"}`);
  });
}

/** Right after Approve on the reply: its answer in place of its buttons, with the focus on it. */
export const Approved: Story = { play: approved };
