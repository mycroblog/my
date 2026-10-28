// SPDX-License-Identifier: AGPL-3.0-only
// ContentWarning stories. The play functions test the disclosure pattern: the button's aria-expanded,
// the panel hidden (and its link out of the tab order) while closed, and focus staying on the button.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn } from "storybook/test";
import { ContentWarning } from "./ContentWarning";
import { PostContent, serverSanitizedHtml } from "./PostContent";

const meta = {
  title: "Timeline/ContentWarning",
  component: ContentWarning,
  args: {
    summary: "Spoilers for the season finale",
    summaryLang: "en",
    toggleLabel: "Show content",
    onExpandedChange: fn(),
    children: (
      <PostContent html={serverSanitizedHtml('<p>The butler did it. <a href="#details">Details</a></p>')} lang="en" />
    ),
  },
} satisfies Meta<typeof ContentWarning>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Closed: Story = {
  play: async ({ canvas, userEvent }) => {
    const button = canvas.getByRole("button", { name: "Show content" });
    await expect(button).toHaveAttribute("aria-expanded", "false");
    await expect(canvas.queryByRole("link", { name: "Details" })).not.toBeInTheDocument();
    await userEvent.tab();
    await expect(button).toHaveFocus();
    // Nothing in the closed panel takes focus.
    await userEvent.tab();
    await expect(button).not.toHaveFocus();
  },
};

export const OpenWithKeyboard: Story = {
  play: async ({ canvas, userEvent }) => {
    const button = canvas.getByRole("button", { name: "Show content" });
    await userEvent.tab();
    await userEvent.keyboard("{Enter}");
    await expect(button).toHaveAttribute("aria-expanded", "true");
    await expect(button).toHaveFocus();
    await userEvent.tab();
    await expect(canvas.getByRole("link", { name: "Details" })).toHaveFocus();
  },
};

export const Open: Story = {
  args: { defaultExpanded: true },
};

/** The warning is user content: it reads in its own direction. */
export const RightToLeftWarning: Story = {
  args: { summary: "تحذير: حرق لأحداث الحلقة الأخيرة", summaryLang: "ar" },
};

/** In a right-to-left page the icon, the text and the button swap sides. */
export const RightToLeftPage: Story = {
  args: {
    dir: "rtl",
    summary: "تحذير: حرق لأحداث الحلقة الأخيرة",
    summaryLang: "ar",
    toggleLabel: "عرض المحتوى",
    defaultExpanded: true,
  },
};
