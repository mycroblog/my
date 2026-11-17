// SPDX-License-Identifier: AGPL-3.0-only

// The notifications the reader's notification policy holds back (ADR-0158 §7), the screen the row over the
// notifications opens: kiri (a boost and a favourite) and kobo (a follow), each with how many and Dismiss beside
// Accept (`held`); each state every screen is shown in (app.samples.tsx); kiri's held-back notifications on their own
// screen, its failure and its request gone; the notifications with the row over them; and right after Accept or
// Dismiss, the answer in place with the focus on it and the row counting again.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { minimumWidth, rightToLeft, screen, screenMeta } from "./app.samples";

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
export const Japanese: Story = { globals: { locale: "ja" } };
export const Empty: Story = { parameters: screen({ at, server, state: "empty" }) };
export const Loading: Story = { parameters: screen({ at, server, state: "loading" }) };
export const Failed: Story = { parameters: screen({ at, server, state: "error" }) };
export const LongText: Story = { parameters: screen({ at, server, state: "long" }) };
export const RightToLeft: Story = { globals: rightToLeft };
export const MinimumWidth: Story = { globals: minimumWidth };
/** One account's held-back notifications, as the list draws them, under the account and its answer. */
export const OneAccount: Story = { parameters: screen({ at: "held:kiri@far.example", server }) };
/** One account's screen when its request could not be read: the failure, with Try again. */
export const OneAccountFailed: Story = {
  parameters: screen({ at: "held:kiri@far.example", server, state: "error" }),
  globals: { locale: "en" },
  play: async (context) => {
    await screenMeta.play(context);
    const main = within(context.canvasElement.querySelector("main") ?? context.canvasElement);
    await expect(main.getByText("Held-back notifications could not be loaded.")).toBeVisible();
    await expect(main.getByRole("button", { name: "Try again" })).toBeVisible();
    await expect(main.queryByText("This person has no held-back notifications now.")).toBeNull();
  },
};
/** One account's screen once the server lists no request of it (answered elsewhere): it says it holds none. */
export const OneAccountGone: Story = {
  parameters: screen({ at: "held:sam", server }),
  globals: { locale: "en" },
  play: async (context) => {
    await screenMeta.play(context);
    const main = within(context.canvasElement.querySelector("main") ?? context.canvasElement);
    await expect(main.getByText("This person has no held-back notifications now.")).toBeVisible();
    await expect(main.queryByRole("button", { name: "Try again" })).toBeNull();
  },
};
/** The notifications, with the row of the held-back ones over them. */
export const InNotifications: Story = { parameters: screen({ at: "/notifications", server }) };

/**
 * Right after Accept on kiri's, in English: the answer in place of its buttons with the focus on it, and kobo's still
 * waiting.
 */
export const Accepted: Story = {
  globals: { locale: "en" },
  play: async (context) => {
    await screenMeta.play(context);
    const main = within(context.canvasElement.querySelector("main") ?? context.canvasElement);
    const [accept] = await main.findAllByRole("button", { name: "Accept" });
    if (accept === undefined) throw new Error("no Accept");
    await userEvent.click(accept);
    const said = await main.findByText("Accepted");
    await waitFor(() => expect(said).toHaveFocus());
    await expect(main.getAllByRole("button", { name: "Accept" })).toHaveLength(1);
  },
};

/** Right after Dismiss on kiri's, in English: the answer in place of its buttons with the focus on it. */
export const Dismissed: Story = {
  globals: { locale: "en" },
  play: async (context) => {
    await screenMeta.play(context);
    const main = within(context.canvasElement.querySelector("main") ?? context.canvasElement);
    const [dismiss] = await main.findAllByRole("button", { name: "Dismiss" });
    if (dismiss === undefined) throw new Error("no Dismiss");
    await userEvent.click(dismiss);
    const said = await main.findByText("Dismissed");
    await waitFor(() => expect(said).toHaveFocus());
    await expect(main.getAllByRole("button", { name: "Dismiss" })).toHaveLength(1);
    await expect(main.getAllByRole("button", { name: "Accept" })).toHaveLength(1);
  },
};

/** One account's screen, in English: the account is the page's head, its name the heading, over its notifications. */
export const OneAccountHead: Story = {
  parameters: screen({ at: "held:kiri@far.example", server }),
  globals: { locale: "en" },
  play: async (context) => {
    await screenMeta.play(context);
    const main = within(context.canvasElement.querySelector("main") ?? context.canvasElement);
    await expect(main.getByRole("heading", { level: 2 })).toBeVisible();
    const feed = main.getByRole("feed", { name: "Held-back notifications" });
    await expect(within(feed).getAllByRole("article")).toHaveLength(2);
    await expect(main.getByRole("button", { name: "Accept" })).toBeVisible();
    await expect(main.getByRole("button", { name: "Dismiss" })).toBeVisible();
  },
};
