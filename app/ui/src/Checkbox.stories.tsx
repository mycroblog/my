// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor } from "storybook/test";
import { Checkbox } from "./Checkbox";

const meta = {
  title: "Forms/Checkbox",
  component: Checkbox,
  args: { children: "Mark media as sensitive" },
} satisfies Meta<typeof Checkbox>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Unchecked: Story = {};
export const Checked: Story = { args: { defaultSelected: true } };
export const Indeterminate: Story = { args: { isIndeterminate: true, children: "Select all" } };
export const WithDescription: Story = { args: { description: "Media is hidden behind a warning until opened." } };

export const Invalid: Story = {
  args: { children: "I accept the server rules", isInvalid: true, errorMessage: "Accept the rules to continue." },
};

export const InvalidChecked: Story = {
  args: { children: "I accept the server rules", isInvalid: true, defaultSelected: true },
};

/**
 * What the pointer meets (web-client/16-look-states-and-wording.md G26, G27): the hand over the checkbox and its label,
 * and a halo of the selection tint around the box under the pointer.
 */
export const PointerStates: Story = {
  tags: ["no-screenshot"],
  play: async ({ canvas, userEvent }) => {
    // The control is a label: the visually hidden input, the box, then the text.
    const label = canvas.getByText("Mark media as sensitive");
    const box = [...label.children].find((child) => child.querySelector("input") === null);
    if (!(box instanceof HTMLElement)) throw new Error("the checkbox has no box");
    await expect(getComputedStyle(label).cursor).toBe("pointer");
    const resting = getComputedStyle(box).boxShadow;
    await userEvent.hover(label);
    await waitFor(() => expect(getComputedStyle(box).boxShadow).not.toBe(resting));
  },
};

/** The native input lies over the box, so that a press on the checkbox itself reaches it (src/ToggleInput.ts). */
export const InputOverBox: Story = {
  tags: ["no-screenshot"],
  play: async ({ canvas, userEvent }) => {
    const input = canvas.getByRole("checkbox");
    const box = input.closest("label")?.querySelector(":scope > span:not(:has(input))")?.getBoundingClientRect();
    if (box === undefined) throw new Error("the checkbox has no box");
    await expect(document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2)).toBe(input);
    await userEvent.click(input);
    await expect(input).toBeChecked();
  },
};

export const Disabled: Story = {
  render: (args) => (
    <div className="flex flex-col gap-2">
      <Checkbox {...args} isDisabled />
      <Checkbox {...args} isDisabled defaultSelected />
    </div>
  ),
};
