// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { Avatar } from "./Avatar";
import { PostCardAuthorLink, PostCardTimeLink } from "./PostCardParts";
import { PostContent, serverSanitizedHtml } from "./PostContent";
import { QuoteCard } from "./QuoteCard";

const meta = {
  title: "Timeline/QuoteCard",
  component: QuoteCard,
  args: {
    label: "Quoted post",
    avatar: <Avatar name="Carol" size="sm" decorative />,
    author: <PostCardAuthorLink href="/@carol@mastodon.example" name="Carol" handle="@carol@mastodon.example" />,
    time: (
      <PostCardTimeLink href="/@carol@mastodon.example/status/1" dense>
        <time dateTime="2026-10-17T06:52:12Z">2h</time>
      </PostCardTimeLink>
    ),
    children: (
      <PostContent html={serverSanitizedHtml("<p>The sun rose again this morning, as it does.</p>")} lang="en" />
    ),
  },
} satisfies Meta<typeof QuoteCard>;

export default meta;
type Story = StoryObj<typeof meta>;

/** The quoted author, the time as the link to the quoted post, and its body, in a named group. */
export const Shown: Story = {
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("group", { name: "Quoted post" })).toBeVisible();
    await expect(canvas.getByRole("link", { name: "2h" })).toHaveAttribute("href", "/@carol@mastodon.example/status/1");
  },
};

/** A quoted post that cannot be shown leaves the notice of why. */
export const Unavailable: Story = {
  args: {
    avatar: undefined,
    author: undefined,
    time: undefined,
    children: undefined,
    notice: "The quoted post was deleted.",
  },
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("group", { name: "Quoted post" })).toHaveTextContent("The quoted post was deleted.");
  },
};

/** The post of a muted author is behind the notice until the reader shows it. */
export const Concealed: Story = {
  args: { notice: "You muted the author of the quoted post.", revealLabel: "Show the quoted post" },
  play: async ({ canvas, userEvent }) => {
    const toggle = canvas.getByRole("button", { name: "Show the quoted post" });
    await expect(toggle).toHaveAttribute("aria-expanded", "false");
    await expect(canvas.queryByRole("link")).not.toBeInTheDocument();
    await userEvent.click(toggle);
    await expect(canvas.getByRole("link", { name: "2h" })).toBeVisible();
    // Back to concealed, so the screenshots show the concealed state.
    await userEvent.click(toggle);
  },
};
