// SPDX-License-Identifier: AGPL-3.0-only
import { isInaccessible, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { TextArea } from "./TextArea";
import { highlightRuns } from "./TextAreaMirror";
import { TextField } from "./TextField";

describe("highlightRuns", () => {
  it("splits the text at the edges of the highlights and at the caret", () => {
    expect(highlightRuns(12, [{ kind: "mention", start: 3, end: 9 }], 5)).toEqual([
      { start: 0, end: 3, kind: null },
      { start: 3, end: 5, kind: "mention" },
      { start: 5, end: 9, kind: "mention" },
      { start: 9, end: 12, kind: null },
    ]);
  });

  it("keeps text over the limit `over` where it overlaps a token, and clamps ranges to the text", () => {
    expect(
      highlightRuns(
        10,
        [
          { kind: "over", start: 6, end: 10 },
          { kind: "url", start: 4, end: 20 },
        ],
        null,
      ),
    ).toEqual([
      { start: 0, end: 4, kind: null },
      { start: 4, end: 6, kind: "url" },
      { start: 6, end: 10, kind: "over" },
    ]);
  });

  it("has no runs for empty text", () => {
    expect(highlightRuns(0, [], 0)).toEqual([]);
  });
});

/** The marks of the highlight layer: their kind, their text, and whether assistive technology skips them. */
function marks() {
  return screen
    .queryAllByText((_, element) => element?.getAttribute("data-highlight") != null)
    .map((mark) => ({
      kind: mark.getAttribute("data-highlight"),
      text: mark.textContent,
      hidden: isInaccessible(mark),
    }));
}

describe("the highlight layer", () => {
  it("paints the ranges behind a text area on an aria-hidden layer and makes the textarea follow its text's direction", () => {
    render(
      <TextArea
        label="Post"
        defaultValue="Hi @alice and #tag"
        highlights={[
          { kind: "mention", start: 3, end: 9 },
          { kind: "hashtag", start: 14, end: 18 },
        ]}
      />,
    );
    expect(screen.getByRole("textbox", { name: "Post" })).toHaveAttribute("dir", "auto");
    expect(marks()).toEqual([
      { kind: "mention", text: "@alice", hidden: true },
      { kind: "hashtag", text: "#tag", hidden: true },
    ]);
  });

  it("mirrors the value of an uncontrolled text field and paints its ranges", () => {
    const { rerender } = render(<TextField label="Name" defaultValue="Alice" highlights={[]} />);
    expect(screen.getByRole("textbox", { name: "Name" })).toHaveAttribute("dir", "auto");
    expect(marks()).toEqual([]);
    rerender(<TextField label="Name" defaultValue="Alice" highlights={[{ kind: "shortcode", start: 0, end: 5 }]} />);
    expect(marks()).toEqual([{ kind: "shortcode", text: "Alice", hidden: true }]);
  });

  it("leaves a text area without highlights as it was", () => {
    render(<TextArea label="Bio" defaultValue="Hello" />);
    // The textarea is the only element with the text: no layer copies it.
    expect(screen.getByText("Hello")).toBe(screen.getByRole("textbox", { name: "Bio" }));
    expect(screen.getByRole("textbox", { name: "Bio" })).not.toHaveAttribute("dir");
  });
});
