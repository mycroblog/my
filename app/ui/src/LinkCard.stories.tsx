// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { LinkCard, type LinkCardImage } from "./LinkCard";

/** A placeholder picture as an SVG data URL, so stories load no network images. */
const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1200 630"><rect width="1200" height="630" fill="#4f6d8f"/><circle cx="600" cy="315" r="160" fill="#f2c14e"/></svg>`;
const image: LinkCardImage = {
  src: `data:image/svg+xml,${encodeURIComponent(svg)}`,
  width: 1200,
  height: 630,
  blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj",
  alt: "A yellow sun over a blue sky",
};

const meta = {
  title: "Timeline/LinkCard",
  component: LinkCard,
  args: {
    href: "https://news.example/articles/sunrise",
    title: "The sun rose again this morning",
    description: "Observers around the world report that the sun rose in the east, as it has every day so far.",
    siteName: "Example News",
    lang: "en",
    revealLabel: "Show sensitive media",
  },
} satisfies Meta<typeof LinkCard>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Without an image: the site name, the title as the card's one link, and the description. */
export const TextOnly: Story = {
  play: async ({ canvas }) => {
    const link = canvas.getByRole("link", { name: "The sun rose again this morning" });
    await expect(link).toHaveAttribute("href", "https://news.example/articles/sunrise");
    await expect(link).toHaveAttribute("target", "_blank");
    await expect(link).toHaveAttribute("rel", "noopener noreferrer nofollow");
  },
};

export const WithImage: Story = {
  args: { image, byline: "By Alice Example · Jan 2, 2026" },
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("img", { name: "A yellow sun over a blue sky" })).toBeVisible();
  },
};

/** A sensitive image is not loaded until the toggle is pressed; its blur stands in for it. */
export const Sensitive: Story = {
  args: { image, sensitive: true },
  play: async ({ canvas, userEvent }) => {
    await expect(canvas.queryByRole("img")).not.toBeInTheDocument();
    const toggle = canvas.getByRole("button", { name: "Show sensitive media" });
    await userEvent.click(toggle);
    await expect(toggle).toHaveAttribute("aria-pressed", "true");
    await expect(canvas.getByRole("img")).toBeInTheDocument();
    // Back to concealed, so the screenshots show the concealed state.
    await userEvent.click(toggle);
    await expect(canvas.queryByRole("img")).not.toBeInTheDocument();
  },
};

/** Long texts wrap and the description is cut to two lines. */
export const LongText: Story = {
  args: {
    title:
      "A very long title of a page that goes on and on, much longer than the room of one line on a phone, to check that it wraps",
    description: "A description that is long enough to be cut. ".repeat(10),
    siteName: "a-very-long-site-name-without-any-spaces-at-all.example",
  },
};

/** A page in a right-to-left language keeps its own direction. */
export const RightToLeft: Story = {
  args: {
    title: "شروق الشمس مرة أخرى",
    description: "أشرقت الشمس من الشرق كما في كل يوم.",
    siteName: "أخبار",
    lang: "ar",
  },
};

/** The small layout: a square image at the end of the row. */
export const Small: Story = {
  args: { size: "small", image, byline: "By Alice Example · Jan 2, 2026" },
  play: async ({ canvas }) => {
    const picture = canvas.getByRole("img", { name: "A yellow sun over a blue sky" });
    await expect(picture).toBeVisible();
    // The title's link stays the hit area of the whole card, beside the image too.
    const link = canvas.getByRole("link", { name: "The sun rose again this morning" });
    const box = link.closest("[data-size]")?.getBoundingClientRect();
    if (box === undefined) throw new Error("the card is not there");
    const hit = document.elementFromPoint(box.left + box.width / 2, box.bottom - 4);
    await expect(hit === link || link.contains(hit)).toBe(true);
    const shown = picture.getBoundingClientRect();
    await expect(Math.round(shown.width)).toBe(Math.round(shown.height));
  },
};

/** The small layout of a page without an image: only the text. */
export const SmallTextOnly: Story = {
  args: { size: "small" },
};

/** The small layout with long texts, cut short beside the image. */
export const SmallLongText: Story = {
  args: {
    size: "small",
    image,
    title:
      "A very long title that goes on and on about the sun rising again this morning, as it has every day, over and over",
    description:
      "Observers around the world report that the sun rose in the east, as it has every day, and that it will most likely do so tomorrow, and on the day after that.",
    byline: "By Alice Example, Bob Example, Carol Example, and many other people · Jan 2, 2026",
  },
};

/** A concealed sensitive image in the small layout: only the icon fits, and the button keeps its name. */
export const SmallSensitive: Story = {
  args: { size: "small", image, sensitive: true },
  play: async ({ canvas, userEvent }) => {
    await expect(canvas.queryByRole("img")).not.toBeInTheDocument();
    await userEvent.click(canvas.getByRole("button", { name: "Show sensitive media" }));
    await expect(canvas.getByRole("img", { name: "A yellow sun over a blue sky" })).toBeVisible();
  },
};
