// SPDX-License-Identifier: AGPL-3.0-only

// The parts of a thread place what they hold and say nothing of their own: what they are to assistive technology is
// what is in them. How far a reply is indented by its depth is a layout test's.

import { expect, test } from "bun:test";
import { isInaccessible, render, screen } from "@testing-library/react";
import { DetailLines, GlyphText, ThreadFocus, ThreadReply } from "./thread";

test("the focused post, a reply and the detail lines hold what they are given, in reach, with no role of their own", () => {
  const { container } = render(
    <>
      <ThreadFocus>
        <p>The focused post</p>
      </ThreadFocus>
      <DetailLines>
        <p>Nov 14, 2026</p>
      </DetailLines>
      <ThreadReply depth={1}>
        <p>A reply</p>
      </ThreadReply>
      <ThreadReply depth={9}>
        <p>A reply to a reply</p>
      </ThreadReply>
    </>,
  );

  for (const words of ["The focused post", "Nov 14, 2026", "A reply", "A reply to a reply"])
    expect(isInaccessible(screen.getByText(words))).toBe(false);
  expect(container.querySelector("[role], [aria-label], [aria-labelledby]")).toBeNull();
});

test("a glyph beside its words is decoration: a screen reader reads the words alone", () => {
  render(
    <DetailLines>
      <p>
        <GlyphText icon="followers">Followers only</GlyphText>
      </p>
    </DetailLines>,
  );

  expect(isInaccessible(screen.getByText("Followers only"))).toBe(false);
  expect(screen.queryByRole("img")).toBeNull();
  expect(screen.getByRole("paragraph").textContent).toBe("Followers only");
});
