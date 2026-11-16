// SPDX-License-Identifier: AGPL-3.0-only

// The follow requests to the reader (FR-302), the screen the row over the notifications opens: ren, a locked account,
// asked by sam (the scenario's) and by marco and mio (`followRequests`), each with Decline and Approve; each state every
// screen is shown in (app.samples.tsx); the notifications with the row of the requests above what waits for approval;
// and the screen right after Approve and after Decline, the answer in the request's place with the focus on it.

import type { Meta, StoryContext, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { minimumWidth, rightToLeft, screen, screenMeta } from "./app.samples";

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
export const RightToLeft: Story = { globals: rightToLeft };
export const MinimumWidth: Story = { globals: minimumWidth };
/** The notifications of ren, with the row of the follow requests over them. */
export const InNotifications: Story = { parameters: screen({ at: "/notifications", viewer, server }) };

/**
 * Approves the newest request (mio's, first on the screen) as the reader presses it, in the toolbar's locale, and
 * waits until its answer took the focus.
 */
async function approved(context: StoryContext) {
  await screenMeta.play(context);
  const main = context.canvasElement.querySelector("main") ?? context.canvasElement;
  // Approve is the primary button of the first request, the second of its pair in any locale.
  const approve = await waitFor(() => {
    const found = within(main).getAllByRole("button");
    const second = found.at(2);
    if (found.length < 3 || second === undefined) throw new Error("the requests have not come yet");
    return second;
  });
  const row = approve.parentElement;
  await userEvent.click(approve);
  await waitFor(() => {
    const focused = context.canvasElement.ownerDocument.activeElement;
    if (focused === null || row === null || !row.contains(focused) || focused.tagName === "BUTTON")
      throw new Error("the answer has not taken the focus yet");
    // Approved (notifications.approved) in en and ja; the pseudo-locale of right to left says it in its own words.
    const lang = context.canvasElement.ownerDocument.documentElement.lang;
    const said = lang.startsWith("ja") ? "承認しました" : lang.startsWith("en") ? "Approved" : undefined;
    if (said === undefined ? focused.textContent === "" : focused.textContent !== said)
      throw new Error(`the focused answer says "${focused.textContent}", not ${said ?? "anything"}`);
  });
}

/** Right after Approve on the newest request: its answer in place of its buttons, with the focus on it. */
export const Approved: Story = { play: approved };

/**
 * Right after Decline on the newest request (mio's), in English: its answer in place of its buttons, with the focus on
 * it, and the other requests still waiting.
 */
export const Declined: Story = {
  globals: { locale: "en" },
  play: async (context) => {
    await screenMeta.play(context);
    const main = within(context.canvasElement.querySelector("main") ?? context.canvasElement);
    const [decline] = await main.findAllByRole("button", { name: "Decline" });
    if (decline === undefined) throw new Error("no Decline");
    await userEvent.click(decline);
    const said = await main.findByText("Declined");
    await waitFor(() => expect(said).toHaveFocus());
    await expect(main.getAllByRole("button", { name: "Decline" })).toHaveLength(2);
    await expect(main.getAllByRole("button", { name: "Approve" })).toHaveLength(2);
  },
};
