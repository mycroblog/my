// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { Pagination } from "./Pagination";

const meta = {
  title: "Navigation/Pagination",
  component: Pagination,
  args: {
    label: "Pages",
    previousLabel: "Previous",
    nextLabel: "Next",
    pageLabel: (page: number) => `Page ${page}`,
    page: 2,
    pageCount: 4,
    onPageChange: () => {},
  },
} satisfies Meta<typeof Pagination>;

export default meta;
type Story = StoryObj<typeof meta>;

/** A few pages: every number shows, and the current one is announced as the current page. */
export const Few: Story = {
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("navigation", { name: "Pages" })).toBeInTheDocument();
    await expect(canvas.getByRole("button", { name: "Page 2" })).toHaveAttribute("aria-current", "page");
  },
};

/** Many pages fold into an ellipsis on each side of the current page. */
export const Many: Story = { args: { page: 10, pageCount: 40 } };

/** On the first page the previous end is disabled. */
export const First: Story = {
  args: { page: 1, pageCount: 40 },
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("button", { name: "Previous" })).toBeDisabled();
  },
};

/** On the last page the next end is disabled. */
export const Last: Story = {
  args: { page: 40, pageCount: 40 },
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("button", { name: "Next" })).toBeDisabled();
  },
};

/** Pages as links, for a list whose page is in the address. */
export const Links: Story = {
  args: { getHref: (page: number) => `?page=${page}` },
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("link", { name: "Page 3" })).toHaveAttribute("href", "?page=3");
  },
};

/** A list that pages by cursor shows only the ends. */
export const CursorOnly: Story = {
  render: (args) => (
    <Pagination
      label={args.label}
      previousLabel="Previous"
      nextLabel="Next"
      previous={null}
      next={{ onPress: () => {} }}
    />
  ),
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("button", { name: "Previous" })).toBeDisabled();
    await expect(canvas.getByRole("button", { name: "Next" })).toBeEnabled();
  },
};

/** Right to left: the ends swap sides and their arrows turn around. */
export const RightToLeft: Story = {
  args: { page: 10, pageCount: 40 },
  decorators: [
    (Story) => (
      <div dir="rtl">
        <Story />
      </div>
    ),
  ],
};
