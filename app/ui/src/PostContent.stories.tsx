// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { PostContent, serverSanitizedHtml } from "./PostContent";

const meta = {
  title: "Timeline/PostContent",
  component: PostContent,
  args: {
    html: serverSanitizedHtml(
      '<p>Trying out the new instance. Say hi to <a href="https://example.com/@bob">@bob</a> and follow <a href="https://example.com/tags/fediverse">#fediverse</a>.</p><p>Second paragraph.</p>',
    ),
    lang: "en",
  },
} satisfies Meta<typeof PostContent>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("link", { name: "@bob" })).toBeVisible();
    await expect(canvas.getByText(/Trying out/).parentElement).toHaveAttribute("dir", "auto");
  },
};

/** A post in a right-to-left script reads right to left in any page direction (`dir="auto"`). */
export const RightToLeftText: Story = {
  args: { html: serverSanitizedHtml("<p>مرحبا بالجميع! هذا أول منشور لي هنا.</p>"), lang: "ar" },
};

/** Lists, quotes and code from the sanitizer's allowlist. */
export const RichText: Story = {
  args: {
    html: serverSanitizedHtml(
      "<p>Release notes:</p><ul><li>Feeds</li><li>Content warnings</li></ul><blockquote><p>Quoted text</p></blockquote><p><code>make check</code> passes.</p>",
    ),
  },
};

/** A long URL wraps instead of making the page scroll sideways at 320 px. */
export const LongLink: Story = {
  args: {
    html: serverSanitizedHtml(
      '<p>See <a href="https://example.com/a/very/long/path/that/does/not/fit/on/a/phone/screen/at/all">https://example.com/a/very/long/path/that/does/not/fit/on/a/phone/screen/at/all</a></p>',
    ),
  },
};

/**
 * A URL link as the server writes it: the scheme and the cut rest are hidden spans that take no room (the link
 * starts right after the text), and the shown part ends in an ellipsis. The hidden parts stay in the copied text.
 */
export const ShortenedLink: Story = {
  args: {
    html: serverSanitizedHtml(
      '<p>Read <a href="https://example.com/a/very/long/path/that/is/cut/short" rel="nofollow noopener" target="_blank"><span class="invisible">https://</span><span class="ellipsis">example.com/a/very/long/path/t</span><span class="invisible">hat/is/cut/short</span></a></p>',
    ),
  },
  play: async ({ canvas }) => {
    const link = canvas.getByRole("link");
    for (const hidden of link.querySelectorAll(".invisible")) {
      const box = hidden.getBoundingClientRect();
      await expect(box.width).toBe(0);
      await expect(box.height).toBe(0);
    }
    const shown = link.querySelector(".ellipsis");
    if (shown === null) throw new Error("no ellipsis span");
    await expect(getComputedStyle(shown, "::after").content).toBe('"…"');
    // The link's text, and so what a copy of it takes, is still the whole URL.
    await expect(link.textContent).toBe("https://example.com/a/very/long/path/that/is/cut/short");
    // The link starts right after the text before it: no room is left for the hidden scheme.
    const read = link.parentElement?.firstChild;
    if (read === null || read === undefined) throw new Error("no text before the link");
    const range = document.createRange();
    range.selectNodeContents(read);
    await expect(Math.abs(link.getBoundingClientRect().left - range.getBoundingClientRect().right)).toBeLessThan(1);
  },
};

/** `size="sm" lineClamp={3}`: an excerpt of a post in a form, cut after three lines. */
export const Excerpt: Story = {
  args: {
    size: "sm",
    lineClamp: 3,
    html: serverSanitizedHtml("<p>One</p><p>Two</p><p>Three</p><p>Four</p><p>Five</p>"),
  },
};

/** A quote and a code block in a right-to-left script: the boxes and their padding mirror with the text. */
export const RightToLeftQuote: Story = {
  args: {
    html: serverSanitizedHtml(
      "<p>مرحبا</p><blockquote><p>اقتباس من منشور آخر</p></blockquote><pre><code>make check</code></pre>",
    ),
    lang: "ar",
  },
};
