// SPDX-License-Identifier: AGPL-3.0-only

// The notifications the reader's notification policy holds back (ADR-0158 §7), the screen the row over the
// notifications opens: kiri (a boost and a favourite) and kobo (a follow), each with how many and Dismiss beside
// Accept (`held`); each state every screen is shown in (app.samples.tsx); kiri's held-back notifications on their own
// screen, its failure and its request gone; the notifications with the row over them; and right after Accept or
// Dismiss, the answer in place of the buttons, which follows a press: a play presses it.

import type { Meta, StoryContext, StoryObj } from "@storybook/react-vite";
import { userEvent, waitFor, within } from "storybook/test";
import { screen, screenMeta } from "./app.samples";

const at = "/notifications/requests";
const server = { held: true };

const meta = {
  title: "Held-back notifications",
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
/** One account's held-back notifications, as the list draws them, under the account and its answer. */
export const OneAccount: Story = { parameters: screen({ at: "held:kiri@far.example", server }) };
/** One account's screen when its request could not be read: the failure, with Try again. */
export const OneAccountFailed: Story = {
  parameters: screen({ at: "held:kiri@far.example", server, state: "error" }),
};
/** One account's screen once the server lists no request of it (answered elsewhere): it says it holds none. */
export const OneAccountGone: Story = { parameters: screen({ at: "held:sam", server }) };
/** The notifications, with the row of the held-back ones over them. */
export const InNotifications: Story = { parameters: screen({ at: "/notifications", server }) };

/**
 * Presses a button of the first account's row (kiri's), and waits until its answer stands in place of its buttons.
 * After the button back to the notifications, a row's buttons are Dismiss then Accept, in any locale.
 */
async function answerFirst(context: StoryContext, button: "dismiss" | "accept") {
  await screenMeta.play(context);
  const main = within(context.canvasElement.querySelector("main") ?? context.canvasElement);
  const buttons = await main.findAllByRole("button");
  const pressed = buttons[button === "dismiss" ? 1 : 2];
  if (pressed === undefined) throw new Error("no account waits for an answer");
  await userEvent.click(pressed);
  await waitFor(() => {
    if (main.getAllByRole("button").length !== buttons.length - 2) throw new Error("the answer does not show yet");
  });
}

/** Right after Accept on kiri's: the answer in place of its buttons, and kobo's still waiting. */
export const Accepted: Story = { play: (context) => answerFirst(context, "accept") };

/** Right after Dismiss on kiri's: the answer in place of its buttons, and kobo's still waiting. */
export const Dismissed: Story = { play: (context) => answerFirst(context, "dismiss") };
