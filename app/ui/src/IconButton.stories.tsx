// SPDX-License-Identifier: AGPL-3.0-only
// IconButton stories. The play functions test that the label is both the button's name and its tooltip, which keyboard
// focus opens and Esc closes while the focus stays on the button.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, fn, waitFor } from "storybook/test";
import { IconButton, IconToggleButton } from "./IconButton";
import { MoreIcon, ReplyIcon, StarIcon } from "./Icons";

const meta = {
  title: "Forms/IconButton",
  component: IconButton,
  decorators: [
    (Story) => (
      <div className="grid min-h-24 place-items-center">
        <Story />
      </div>
    ),
  ],
  args: { label: "More actions", onPress: fn(), children: <MoreIcon /> },
} satisfies Meta<typeof IconButton>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Keyboard focus opens the tooltip with the button's name; Esc closes it and leaves the focus on the button. */
export const Default: Story = {
  play: async ({ canvas, userEvent, args }) => {
    const button = canvas.getByRole("button", { name: "More actions" });
    await userEvent.tab();
    await expect(button).toHaveFocus();
    const tooltip = await canvas.findByRole("tooltip");
    await expect(tooltip).toHaveTextContent("More actions");
    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(canvas.queryByRole("tooltip")).not.toBeInTheDocument());
    await expect(button).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await expect(args.onPress).toHaveBeenCalledOnce();
  },
};

/** A toggle of a post's action bar: the icon's shape and `aria-pressed` change, and the name has the count it shows. */
function Favourite() {
  const [isSelected, setIsSelected] = useState(false);
  const count = isSelected ? 13 : 12;
  return (
    <IconToggleButton
      label="Favourite"
      countedLabel={`Favourite, ${count}`}
      tone="warning"
      isSelected={isSelected}
      onChange={setIsSelected}
    >
      <StarIcon filled={isSelected} />
      <span aria-hidden="true">{count}</span>
    </IconToggleButton>
  );
}

export const Toggle: Story = {
  render: () => <Favourite />,
  play: async ({ canvas, userEvent }) => {
    const button = canvas.getByRole("button", { name: "Favourite, 12", pressed: false });
    await userEvent.tab();
    await expect(await canvas.findByRole("tooltip")).toHaveTextContent("Favourite");
    await userEvent.keyboard(" ");
    await expect(canvas.getByRole("button", { name: "Favourite, 13", pressed: true })).toBe(button);
    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(canvas.queryByRole("tooltip")).not.toBeInTheDocument());
  },
};

/** The sizes of a row of actions: the `subtle` toolbar buttons of a post beside the default `ghost` icon button. */
export const Sizes: Story = {
  render: () => (
    <div className="flex items-center gap-2">
      <IconButton label="Reply" variant="subtle" size="toolbar">
        <ReplyIcon />
      </IconButton>
      <IconButton label="More actions">
        <MoreIcon />
      </IconButton>
      <IconButton label="More actions" size="compact">
        <MoreIcon />
      </IconButton>
    </div>
  ),
};
