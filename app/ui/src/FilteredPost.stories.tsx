// SPDX-License-Identifier: AGPL-3.0-only
// FilteredPost stories: the line and the button that stand in for a post that keyword filters fold.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn } from "storybook/test";
import { FilteredPost } from "./FilteredPost";

const meta = {
  title: "Timeline/FilteredPost",
  component: FilteredPost,
  args: { label: "Filtered: Spoilers", showLabel: "Show", onShow: fn() },
} satisfies Meta<typeof FilteredPost>;

export default meta;
type Story = StoryObj<typeof meta>;

/** The article is named by the line; pressing the button asks for the post. */
export const Folded: Story = {
  play: async ({ args, canvas, userEvent }) => {
    await expect(canvas.getByRole("article", { name: "Filtered: Spoilers" })).toBeInTheDocument();
    await userEvent.click(canvas.getByRole("button", { name: "Show" }));
    await expect(args.onShow).toHaveBeenCalledOnce();
  },
};

/** Several filters, whose long names wrap inside the line instead of widening the card. */
export const ManyFilters: Story = {
  args: {
    label: "Filtered: Spoilers for the season finale of a very long-running drama, Politics, and Sports results",
  },
};

/** The names of the filters are the user's text: a right-to-left name keeps its own direction. */
export const RightToLeftName: Story = {
  args: { label: "Filtered: ⁨حرق الأحداث⁩" },
};
