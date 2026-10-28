// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, waitFor } from "storybook/test";
import { DurationSelect } from "./DurationSelect";

// Stories may spell out English; apps pass react-intl messages instead (web-client/README.md A9).
const day = 86_400;
const formatDuration = (seconds: number) => (seconds === day ? "1 day" : `${String(seconds / day)} days`);

const meta = {
  title: "Forms/DurationSelect",
  component: DurationSelect,
  args: {
    label: "Duration",
    placeholder: "Choose a duration",
    durations: [day, 7 * day, 30 * day],
    formatDuration,
    indefiniteLabel: "Indefinitely",
    onChange: fn(),
  },
} satisfies Meta<typeof DurationSelect>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Placeholder: Story = {};

export const Selected: Story = {
  args: { defaultValue: 7 * day, description: "The suspension ends by itself after this." },
};

export const Indefinitely: Story = { args: { defaultValue: null } };

/** Without `indefiniteLabel`, every choice has an end. */
export const AlwaysEnds: Story = {
  args: { indefiniteLabel: undefined, defaultValue: day },
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(canvas.getByRole("button", { name: /Duration/ }));
    const listbox = await canvas.findByRole("listbox");
    await waitFor(() => expect(listbox).toBeVisible());
    await expect(canvas.queryByRole("option", { name: "Indefinitely" })).not.toBeInTheDocument();
    await userEvent.keyboard("{Escape}");
  },
};

/** Choosing "Indefinitely" reports null; a duration reports its seconds. */
export const Choose: Story = {
  play: async ({ args, canvas, userEvent }) => {
    await userEvent.click(canvas.getByRole("button", { name: /Duration/ }));
    await userEvent.click(await canvas.findByRole("option", { name: "Indefinitely" }));
    await expect(args.onChange).toHaveBeenLastCalledWith(null);
    await userEvent.click(canvas.getByRole("button", { name: /Duration/ }));
    await userEvent.click(await canvas.findByRole("option", { name: "30 days" }));
    await expect(args.onChange).toHaveBeenLastCalledWith(30 * day);
  },
};

export const Disabled: Story = { args: { defaultValue: day, isDisabled: true } };

/** The open list, for the accessibility check of the options. */
export const Open: Story = { args: { defaultValue: 7 * day, defaultOpen: true } };
