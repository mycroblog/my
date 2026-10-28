// SPDX-License-Identifier: AGPL-3.0-only
import { describe, expect, test } from "vitest";
import vectors from "../../../../pkg/testkit/fixtures/text/tokens.json";
import { textHighlights } from "./textHighlights";

// The highlight layer paints exactly the tokens of the shared vectors that ParseText and ScanShortcodes read (ADR-0092 §4).
describe("textHighlights follows the shared vectors", () => {
  test.each(vectors.cases.map((c) => [c.name, c] as const))("%s", (_, c) => {
    const shortcodes = new Set(c.tokens.filter((t) => t.kind === "shortcode").map((t) => t.text.slice(1, -1)));
    expect(textHighlights(c.text, { tokens: "all", shortcodes })).toEqual(
      c.tokens.map((t) => ({ kind: t.kind, start: t.utf16[0], end: t.utf16[1] })),
    );
    expect(textHighlights(c.text, { tokens: "shortcodes", shortcodes })).toEqual(
      c.tokens.filter((t) => t.kind === "shortcode").map((t) => ({ kind: t.kind, start: t.utf16[0], end: t.utf16[1] })),
    );
  });
});

describe("textHighlights", () => {
  const shortcodes = new Set(["blobcat"]);

  test("marks only the shortcodes of the local custom emoji", () => {
    expect(textHighlights(":blobcat: :unknown:", { tokens: "all", shortcodes })).toEqual([
      { kind: "shortcode", start: 0, end: 9 },
    ]);
  });

  test("marks only shortcodes in a field whose links the server does not read", () => {
    expect(textHighlights("@alice #tag https://example.com :blobcat:", { tokens: "shortcodes", shortcodes })).toEqual([
      { kind: "shortcode", start: 32, end: 41 },
    ]);
  });

  test("marks the text after the limit, counted in code points", () => {
    // 🐱 is two UTF-16 units and one code point, as the server counts it.
    expect(textHighlights("🐱🐱abc", { tokens: "all", shortcodes, limit: 3 })).toEqual([
      { kind: "over", start: 5, end: 7 },
    ]);
    expect(textHighlights("abc", { tokens: "all", shortcodes, limit: 3 })).toEqual([]);
    expect(textHighlights("ab", { tokens: "all", shortcodes, limit: -2 })).toEqual([
      { kind: "over", start: 0, end: 2 },
    ]);
  });

  test("lets the text over the limit overlap a token", () => {
    expect(textHighlights("hi @alice", { tokens: "all", shortcodes, limit: 5 })).toEqual([
      { kind: "mention", start: 3, end: 9 },
      { kind: "over", start: 5, end: 9 },
    ]);
  });
});
