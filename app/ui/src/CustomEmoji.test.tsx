// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { CustomEmoji, type CustomEmojiImage, EmojiText, insertCustomEmojis, splitCustomEmoji } from "./CustomEmoji";
import { AnimationsPlayContext, AutoplayContext } from "./Picture";
import { PostContent, serverSanitizedHtml } from "./PostContent";

const blobcat: CustomEmojiImage = {
  shortcode: "blobcat",
  url: "https://example.com/blobcat.gif",
  static_url: "https://example.com/blobcat.png",
};
const still: CustomEmojiImage = {
  shortcode: "still",
  url: "https://example.com/still.png",
  static_url: "https://example.com/still.png",
};

describe("splitCustomEmoji", () => {
  it("replaces only the shortcodes of the given emoji", () => {
    expect(splitCustomEmoji("hi :blobcat: and :unknown: :still:!", [blobcat, still])).toEqual([
      { start: 0, text: "hi " },
      { start: 3, emoji: blobcat },
      { start: 12, text: " and :unknown: " },
      { start: 27, emoji: still },
      { start: 34, text: "!" },
    ]);
  });

  it("finds emoji next to each other and after a colon that opens no emoji", () => {
    expect(splitCustomEmoji(":blobcat::still:", [blobcat, still])).toEqual([
      { start: 0, emoji: blobcat },
      { start: 9, emoji: still },
    ]);
    expect(splitCustomEmoji("12:30:blobcat:", [blobcat])).toEqual([
      { start: 0, text: "12:30" },
      { start: 5, emoji: blobcat },
    ]);
  });

  it("keeps the text whole without emoji", () => {
    expect(splitCustomEmoji(":blobcat:", [])).toEqual([{ start: 0, text: ":blobcat:" }]);
    expect(splitCustomEmoji("", [blobcat])).toEqual([{ start: 0, text: "" }]);
  });
});

describe("CustomEmoji", () => {
  it("is an image read as its shortcode, animated only for those who allow motion", () => {
    render(<CustomEmoji emoji={blobcat} />);
    const image = screen.getByRole("img", { name: ":blobcat:" });
    expect(image).toHaveAttribute("src", blobcat.static_url);
    expect(image).toHaveAttribute("title", ":blobcat:");
    expect(image).toHaveAttribute("draggable", "false");
    expect(renderToStaticMarkup(<CustomEmoji emoji={blobcat} />)).toContain(
      `<picture><source media="(prefers-reduced-motion: no-preference)" srcSet="${blobcat.url}"/>`,
    );
  });

  it("shows only the still image when it must not move or has no animation", () => {
    expect(renderToStaticMarkup(<CustomEmoji emoji={blobcat} allowMotion={false} />)).not.toContain("<picture");
    expect(renderToStaticMarkup(<CustomEmoji emoji={still} />)).not.toContain("<picture");
  });

  it("follows the app's autoplay setting unless told otherwise", () => {
    const off = (children: ReactNode) =>
      renderToStaticMarkup(<AutoplayContext value={false}>{children}</AutoplayContext>);
    expect(off(<CustomEmoji emoji={blobcat} />)).not.toContain("<picture");
    expect(off(<CustomEmoji emoji={blobcat} allowMotion />)).toContain("<picture");
  });

  it("moves while the post or profile it is in plays, still behind the reduced-motion query", () => {
    const view = renderToStaticMarkup(
      <AutoplayContext value={false}>
        <AnimationsPlayContext value>
          <CustomEmoji emoji={blobcat} />
          <CustomEmoji emoji={blobcat} allowMotion={false} />
        </AnimationsPlayContext>
      </AutoplayContext>,
    );
    expect(view.match(/<picture>/gu)).toHaveLength(1);
    expect(view).toContain('media="(prefers-reduced-motion: no-preference)"');
  });
});

describe("EmojiText", () => {
  it("draws the emoji of a plain text in place of their shortcodes", () => {
    render(
      <p>
        <EmojiText text="Alice :blobcat: :nope:" emojis={[blobcat]} />
      </p>,
    );
    expect(screen.getByRole("paragraph")).toHaveTextContent("Alice :nope:");
    expect(screen.getByRole("img", { name: ":blobcat:" })).toBeInTheDocument();
  });
});

describe("insertCustomEmojis", () => {
  it("draws emoji in the text of HTML outside code, and puts the text back", () => {
    const root = document.createElement("div");
    document.body.append(root);
    root.innerHTML = "<p>hi :blobcat: <a href='#'>:still:</a></p><pre>:blobcat:</pre><code>:still:</code>";
    const restore = insertCustomEmojis(root, [blobcat, still]);
    const view = within(root);
    expect(view.getAllByRole("img").map((image) => image.getAttribute("alt"))).toEqual([":blobcat:", ":still:"]);
    expect(within(view.getByRole("link")).getByRole("img", { name: ":still:" })).toBeInTheDocument();
    expect(view.getByText(":blobcat:")).toBeInTheDocument();
    expect(view.getByText(":still:")).toBeInTheDocument();
    restore();
    expect(root.innerHTML).toBe('<p>hi :blobcat: <a href="#">:still:</a></p><pre>:blobcat:</pre><code>:still:</code>');
    root.remove();
  });
});

describe("PostContent with emoji", () => {
  it("draws the emoji of the body, and draws them again in a new body", () => {
    const { rerender } = render(
      <PostContent html={serverSanitizedHtml("<p>hello :blobcat:</p>")} emojis={[blobcat]} />,
    );
    expect(screen.getByRole("img", { name: ":blobcat:" })).toBeInTheDocument();

    rerender(<PostContent html={serverSanitizedHtml("<p>edited :blobcat: :still:</p>")} emojis={[blobcat, still]} />);
    expect(screen.getAllByRole("img").map((image) => image.getAttribute("alt"))).toEqual([":blobcat:", ":still:"]);
    expect(screen.getByText(/edited/)).toBeInTheDocument();

    rerender(<PostContent html={serverSanitizedHtml("<p>edited :blobcat: :still:</p>")} emojis={[]} />);
    expect(screen.queryByRole("img")).toBeNull();
    expect(screen.getByText("edited :blobcat: :still:")).toBeInTheDocument();
  });
});
