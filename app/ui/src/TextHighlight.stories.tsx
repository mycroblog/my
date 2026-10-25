// SPDX-License-Identifier: AGPL-3.0-only
// The highlight layer of the text fields (ADR-0092 §4): the backgrounds of mentions, hashtags, URLs, `:shortcode:`s and
// the text over the limit, painted on the aria-hidden mirror behind the transparent textarea or input, which draws the
// text. The screenshots (3 widths × light / dark × LTR / RTL) show the backgrounds under the textarea's own glyphs, and
// the play functions check what a screenshot cannot: the layer is hidden from assistive technology, copies the
// control's line height and scroll, and its marks are the ranges given.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor } from "storybook/test";
import { Button } from "./Button";
import { EmojiIcon } from "./Icons";
import { TextArea } from "./TextArea";
import type { TextHighlight } from "./TextAreaMirror";
import { TextField } from "./TextField";

// Stories may spell out English; apps pass react-intl messages instead (web-client/README.md A9).
const formatRemaining = (remaining: number) => (remaining < 0 ? `${String(-remaining)} characters too many` : `${String(remaining)} characters left`);

/** A stand-in for the web's textHighlights (lib/textHighlights.ts): the ranges of the given substrings. */
function rangesOf(text: string, marks: readonly (readonly [TextHighlight["kind"], string])[], limit?: number): TextHighlight[] {
  const highlights: TextHighlight[] = marks.map(([kind, part]) => {
    const start = text.indexOf(part);
    return { kind, start, end: start + part.length };
  });
  if (limit !== undefined && Array.from(text).length > limit) {
    highlights.push({ kind: "over", start: Array.from(text).slice(0, limit).join("").length, end: text.length });
  }
  return highlights;
}

const post =
  "こんにちは @alice@example.org! #fediverse の話をしよう 🐱 :blobcat: https://example.com/a/very/long/path/that/wraps/in/a/narrow/field?with=query and the end of the post runs over the limit.";
const postLimit = 150;
const postHighlights = rangesOf(
  post,
  [
    ["mention", "@alice@example.org"],
    ["hashtag", "#fediverse"],
    ["shortcode", ":blobcat:"],
    ["url", "https://example.com/a/very/long/path/that/wraps/in/a/narrow/field?with=query"],
  ],
  postLimit,
);

const emojiButton = (
  <Button variant="ghost" size="icon" aria-label="Add emoji">
    <EmojiIcon />
  </Button>
);

/** The mirror behind a control: the element right before it in the same box. */
function layerOf(control: HTMLElement): HTMLElement {
  const layer = control.previousElementSibling;
  if (!(layer instanceof HTMLElement)) throw new Error("the control has no layer before it");
  return layer;
}

function marksOf(layer: HTMLElement) {
  return [...layer.querySelectorAll("[data-highlight]")].map((mark) => [mark.getAttribute("data-highlight"), mark.textContent]);
}

const meta = {
  title: "Forms/TextHighlight",
  component: TextArea,
  args: { label: "Post", rows: 4 },
} satisfies Meta<typeof TextArea>;

export default meta;
type Story = StoryObj<typeof meta>;

/**
 * A post with every kind of range, wrapping lines (a long URL, CJK, an emoji) and text over the limit, beside the end
 * action. The marks are the given ranges; the text over the limit wins over the URL it overlaps.
 */
export const Post: Story = {
  args: { defaultValue: post, highlights: postHighlights, characterLimit: postLimit, formatRemaining, endAction: emojiButton },
  play: async ({ canvas }) => {
    const textbox = canvas.getByRole<HTMLTextAreaElement>("textbox", { name: "Post" });
    const layer = layerOf(textbox);
    await expect(layer).toHaveAttribute("aria-hidden", "true");
    await expect(layer).toHaveAttribute("dir", "auto");
    await expect(textbox).toHaveAttribute("dir", "auto");
    // The layer copies the textarea's layout, and the line height is whole pixels (#1152).
    // The layer copies after fonts load and on resize, so it may trail the textarea for a frame: wait for it to settle.
    await document.fonts.ready;
    const textareaStyle = getComputedStyle(textbox);
    const layerStyle = getComputedStyle(layer);
    await waitFor(() => expect(Number.isInteger(Number.parseFloat(textareaStyle.lineHeight))).toBe(true));
    for (const property of ["font-family", "font-size", "line-height", "letter-spacing", "padding-inline-start", "padding-inline-end", "padding-block-start"]) {
      await waitFor(() => expect(layerStyle.getPropertyValue(property)).toBe(textareaStyle.getPropertyValue(property)));
    }
    // The textarea draws the text; the layer's text is transparent and only its marks have a background.
    await expect(textareaStyle.backgroundColor).toBe("rgba(0, 0, 0, 0)");
    await expect(layerStyle.color).toBe("rgba(0, 0, 0, 0)");
    await expect(marksOf(layer)).toEqual([
      ["mention", "@alice@example.org"],
      ["hashtag", "#fediverse"],
      ["shortcode", ":blobcat:"],
      ["url", "https://example.com/a/very/long/path/that/wraps/in/a/narrow/field?with=query"],
      ["over", Array.from(post).slice(postLimit).join("")],
    ]);
  },
};

const scrolledText = `${"A line of text to scroll past.\n".repeat(6)}The last line mentions @bob and #tags.`;

/** A text longer than the box, scrolled to its end: the layer scrolls with the textarea. */
export const Scrolled: Story = {
  args: {
    rows: 3,
    defaultValue: scrolledText,
    highlights: rangesOf(scrolledText, [
      ["mention", "@bob"],
      ["hashtag", "#tags"],
    ]),
  },
  play: async ({ canvas }) => {
    const textbox = canvas.getByRole<HTMLTextAreaElement>("textbox", { name: "Post" });
    const layer = layerOf(textbox);
    textbox.scrollTop = textbox.scrollHeight;
    await waitFor(() => expect(textbox.scrollTop).toBeGreaterThan(0));
    await waitFor(() => expect(layer.scrollTop).toBe(textbox.scrollTop));
  },
};

/** Arabic text with a Latin mention: the layer and the textarea both take their direction from the text. */
export const RightToLeftText: Story = {
  args: {
    defaultValue: "مرحبا @alice كيف حالك؟ #فيديفيرس :blobcat:",
    highlights: rangesOf("مرحبا @alice كيف حالك؟ #فيديفيرس :blobcat:", [
      ["mention", "@alice"],
      ["hashtag", "#فيديفيرس"],
      ["shortcode", ":blobcat:"],
    ]),
    endAction: emojiButton,
  },
  play: async ({ canvas }) => {
    const textbox = canvas.getByRole<HTMLTextAreaElement>("textbox", { name: "Post" });
    await expect(getComputedStyle(textbox).direction).toBe("rtl");
    await expect(getComputedStyle(layerOf(textbox)).direction).toBe("rtl");
  },
};

/** In forced-colors mode the backgrounds are gone, so the ranges are underlined (dashed over the limit); captured so. */
export const ForcedColors: Story = {
  tags: ["forced-colors"],
  args: { defaultValue: post, highlights: postHighlights, characterLimit: postLimit, formatRemaining },
  play: async ({ canvas }) => {
    // Outside forced colors, the marks have backgrounds and no line under them.
    const marks = [...layerOf(canvas.getByRole("textbox", { name: "Post" })).querySelectorAll("[data-highlight]")];
    for (const mark of marks) {
      await expect(getComputedStyle(mark).borderBlockEndWidth).toBe("0px");
      await expect(getComputedStyle(mark).backgroundColor).not.toBe("rgba(0, 0, 0, 0)");
    }
  },
};

const name = "Alice :blobcat: :unknown: Liddell";

/** A single-line field (a display name or a content warning) with only `:shortcode:`s highlighted. */
export const SingleLine: Story = {
  render: () => <TextField label="Display name" defaultValue={name} highlights={rangesOf(name, [["shortcode", ":blobcat:"]])} endAction={emojiButton} />,
  play: async ({ canvas }) => {
    const input = canvas.getByRole<HTMLInputElement>("textbox", { name: "Display name" });
    const layer = layerOf(input);
    await expect(layer).toHaveAttribute("aria-hidden", "true");
    await expect(input).toHaveAttribute("dir", "auto");
    await expect(Number.isInteger(Number.parseFloat(getComputedStyle(input).lineHeight))).toBe(true);
    await expect(marksOf(layer)).toEqual([["shortcode", ":blobcat:"]]);
  },
};

const longValue =
  "See https://example.com/a/very/long/path/to/a/page/about/@alice, which is longer than the widest field, and then some more words to read before the #tags at the end of the value";

/** A single line longer than the field, scrolled to its end: the layer scrolls sideways with the input. */
export const SingleLineScrolled: Story = {
  render: () => (
    <TextField
      label="Value"
      defaultValue={longValue}
      highlights={rangesOf(longValue, [
        ["url", "https://example.com/a/very/long/path/to/a/page/about/@alice"],
        ["hashtag", "#tags"],
      ])}
    />
  ),
  play: async ({ canvas }) => {
    const input = canvas.getByRole<HTMLInputElement>("textbox", { name: "Value" });
    const layer = layerOf(input);
    // The caret at the end scrolls the input (an input blurred scrolls back to its start); the text runs left to right
    // on either page.
    input.focus();
    input.setSelectionRange(longValue.length, longValue.length);
    await waitFor(() => expect(input.scrollLeft).toBeGreaterThan(0));
    await waitFor(() => expect(layer.scrollLeft).toBe(input.scrollLeft));
  },
};
