// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, fn } from "storybook/test";
import { Button } from "./Button";
import { NewItemsButton } from "./NewItemsButton";

const meta = {
  title: "Timeline/NewItemsButton",
  component: NewItemsButton,
  args: { count: 3, label: "3 new posts", onPress: fn() },
} satisfies Meta<typeof NewItemsButton>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Waiting: Story = {
  play: async ({ args, canvas, userEvent }) => {
    await expect(canvas.getByRole("status")).toHaveTextContent("3 new posts");
    const button = canvas.getByRole("button", { name: "3 new posts" });
    await userEvent.click(button);
    await expect(args.onPress).toHaveBeenCalledOnce();
    // Back to the resting state, with no hover or focus.
    await userEvent.unhover(button);
    button.blur();
  },
};

/** With nothing waiting the button is gone; the empty status region stays so the next count is read. */
export const NothingWaiting: Story = {
  args: { count: 0, label: "0 new posts" },
  render: (args) => (
    <div className="flex flex-col gap-2">
      <h2 className="text-xl font-semibold">Home</h2>
      <NewItemsButton {...args} />
    </div>
  ),
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("status")).toBeEmptyDOMElement();
  },
};

/** New posts arriving change the count without taking focus from what the reader is doing. */
export const CountChanges: Story = {
  render: function CountChanges(args) {
    const [count, setCount] = useState(1);
    return (
      <div className="flex flex-col items-start gap-3">
        <Button
          onPress={() => {
            setCount(count + 1);
          }}
        >
          Simulate a new post
        </Button>
        <NewItemsButton {...args} count={count} label={count === 1 ? "1 new post" : `${String(count)} new posts`} />
      </div>
    );
  },
  play: async ({ canvas, userEvent }) => {
    const simulate = canvas.getByRole("button", { name: "Simulate a new post" });
    await userEvent.click(simulate);
    await expect(canvas.getByRole("status")).toHaveTextContent("2 new posts");
    await expect(simulate).toHaveFocus();
  },
};

/** Floating: the region has no height, so the post below keeps its place and the button stands above its edge. */
export const Floating: Story = {
  args: { sticky: true },
  render: (args) => (
    <div className="flex flex-col pt-10">
      <NewItemsButton {...args} />
      <p className="rounded-lg bg-surface-raised p-4 shadow-sm">The newest post stays where it is.</p>
    </div>
  ),
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("button", { name: "3 new posts" })).toBeVisible();
  },
};
