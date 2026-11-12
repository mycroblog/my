// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { ArticleCard, type ArticleCoverImage, ArticleCover } from "./ArticleCard";

/** A placeholder picture as an SVG data URL, so stories load no network images. */
const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1600 900"><rect width="1600" height="900" fill="#5c7a4f"/><circle cx="800" cy="450" r="220" fill="#e0533d"/></svg>`;
const cover: ArticleCoverImage = {
  sources: { still: [{ url: `data:image/svg+xml,${encodeURIComponent(svg)}` }] },
  width: 1600,
  height: 900,
  blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj",
  alt: "A red tomato on green leaves",
};

const meta = {
  title: "Timeline/ArticleCard",
  component: ArticleCard,
  args: {
    href: "/@alice/status/0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f",
    title: "What I learned growing tomatoes on a balcony",
    untitledLabel: "Untitled article",
    summary: "Three summers, two pots and one stubborn plant: notes on soil, water and patience.",
    lang: "en",
    kindLabel: "Article",
    revealLabel: "Show sensitive media",
  },
} satisfies Meta<typeof ArticleCard>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Without a cover: the kind, the title as the card's one link, and the summary. */
export const TextOnly: Story = {
  play: async ({ canvas }) => {
    const link = canvas.getByRole("link", { name: "What I learned growing tomatoes on a balcony" });
    await expect(link).toHaveAttribute("href", "/@alice/status/0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f");
    await expect(canvas.getAllByRole("link")).toHaveLength(1);
  },
};

export const WithCover: Story = {
  args: { cover },
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("img", { name: "A red tomato on green leaves" })).toBeVisible();
  },
};

/** A sensitive article shows its title and summary; its cover is not loaded until the toggle is pressed. */
export const Sensitive: Story = {
  args: { cover, sensitive: true },
  play: async ({ canvas, userEvent }) => {
    await expect(canvas.queryByRole("img")).not.toBeInTheDocument();
    await expect(canvas.getByText(/Three summers/)).toBeVisible();
    const toggle = canvas.getByRole("button", { name: "Show sensitive media" });
    await userEvent.click(toggle);
    await expect(toggle).toHaveAttribute("aria-pressed", "true");
    await expect(canvas.getByRole("img")).toBeInTheDocument();
    // Pressed again, it conceals the media once more.
    await userEvent.click(toggle);
    await expect(canvas.queryByRole("img")).not.toBeInTheDocument();
  },
};

/** An article from another server without a title: the link says so in the app's words. */
export const Untitled: Story = {
  args: { title: null, summary: undefined },
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("link", { name: "Untitled article" })).toBeVisible();
  },
};

/** Long texts wrap and the summary is cut to three lines. */
export const LongText: Story = {
  args: {
    title:
      "A very long title of an article that goes on and on, much longer than the room of one line on a phone, to check that it wraps",
    summary: "A summary that is long enough to be cut after three lines of the card. ".repeat(8),
  },
};

/** An article in a right-to-left language keeps its own direction. */
export const RightToLeft: Story = {
  args: {
    title: "ما تعلمته من زراعة الطماطم",
    summary: "ثلاثة فصول صيف ووعاءان ونبتة عنيدة.",
    lang: "ar",
    cover,
  },
};

/** The cover alone, on the article's page: rounded, and concealed while the article is sensitive. */
export const Cover: Story = {
  render: () => <ArticleCover image={cover} lang="en" rounded sensitive revealLabel="Show sensitive media" />,
  play: async ({ canvas, userEvent }) => {
    await expect(canvas.queryByRole("img")).not.toBeInTheDocument();
    const toggle = canvas.getByRole("button", { name: "Show sensitive media" });
    await userEvent.click(toggle);
    await expect(canvas.getByRole("img", { name: "A red tomato on green leaves" })).toBeVisible();
    // Pressed again, it conceals the cover once more.
    await userEvent.click(toggle);
    await expect(canvas.queryByRole("img")).not.toBeInTheDocument();
  },
};
