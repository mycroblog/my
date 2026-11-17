// SPDX-License-Identifier: AGPL-3.0-only

// The follow requests to the reader (FR-302), the screen the row over the notifications opens: ren, a locked account,
// asked by sam (the scenario's) and by marco and mio (`followRequests`), each with Decline and Approve; each state every
// screen is shown in (app.samples.tsx); the notifications with the row of the requests above what waits for approval;
// and the screen right after Approve and after Decline, the answer in the request's place with the focus on it, which
// a play presses: no address or answer of the server gives it.

import type { Meta, StoryContext, StoryObj } from "@storybook/react-vite";
import { userEvent, waitFor, within } from "storybook/test";
import { screen, screenMeta } from "./app.samples";

const at = "/notifications/follow_requests";
const viewer = "ren";
const server = { followRequests: { from: ["marco", "mio"] } };

const meta = {
  title: "Follow requests",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at, viewer, server }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const Normal: Story = {};
export const Empty: Story = { parameters: screen({ at, viewer, server, state: "empty" }) };
export const Loading: Story = { parameters: screen({ at, viewer, server, state: "loading" }) };
export const Failed: Story = { parameters: screen({ at, viewer, server, state: "error" }) };
export const LongText: Story = { parameters: screen({ at, viewer, server, state: "long" }) };
/** The notifications of ren, with the row of the follow requests over them. */
export const InNotifications: Story = { parameters: screen({ at: "/notifications", viewer, server }) };

/**
 * Answers the newest request (mio's, first on the screen) as the reader presses it, in the toolbar's locale, and waits
 * until the answer stands in the place of its buttons: Decline is the first of its pair in any locale, Approve the
 * second, after the screen's own Back.
 */
const answered = (answer: "decline" | "approve") => async (context: StoryContext) => {
  await screenMeta.play(context);
  const main = context.canvasElement.querySelector("main") ?? context.canvasElement;
  const waiting = () => within(main).getAllByRole("button").length;
  const button = await waitFor(() => {
    const found = within(main)
      .getAllByRole("button")
      .at(answer === "decline" ? 1 : 2);
    if (waiting() < 3 || found === undefined) throw new Error("the requests have not come yet");
    return found;
  });
  const before = waiting();
  await userEvent.click(button);
  await waitFor(() => {
    if (waiting() !== before - 2) throw new Error("the answer is not in place yet");
  });
};

/** Right after Approve on the newest request: its answer in place of its buttons, with the focus on it. */
export const Approved: Story = { play: answered("approve") };

/**
 * Right after Decline on the newest request (mio's): its answer in place of its buttons, with the focus on it, and the
 * other requests still waiting.
 */
export const Declined: Story = { play: answered("decline") };
