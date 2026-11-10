// SPDX-License-Identifier: AGPL-3.0-only
import { describe, expect, test } from "vitest";
import { insertImages, insertLink, parseTags, tagsText, toggleBold } from "./markdown";

describe("toggleBold", () => {
  test("wraps the selection and keeps it selected", () => {
    expect(toggleBold("a word here", 2, 6)).toEqual({ value: "a **word** here", selectionStart: 4, selectionEnd: 8 });
  });

  test("puts the caret between the marks when nothing is selected", () => {
    expect(toggleBold("ab", 1, 1)).toEqual({ value: "a****b", selectionStart: 3, selectionEnd: 3 });
  });

  test("unwraps a bold selection", () => {
    expect(toggleBold("a **word** here", 4, 8)).toEqual({ value: "a word here", selectionStart: 2, selectionEnd: 6 });
  });
});

describe("insertLink", () => {
  test("makes the selection the text of a link and selects the address", () => {
    const edit = insertLink("see the docs", 8, 12);
    expect(edit.value).toBe("see the [docs](https://)");
    expect(edit.value.slice(edit.selectionStart, edit.selectionEnd)).toBe("https://");
  });

  test("puts the caret between the brackets when nothing is selected", () => {
    expect(insertLink("x", 1, 1)).toEqual({ value: "x[](https://)", selectionStart: 2, selectionEnd: 2 });
  });
});

describe("insertImages", () => {
  test("puts each image in a paragraph of its own and the caret after them", () => {
    const edit = insertImages("Before\nAfter", 6, 6, ["m1", "m2"]);
    expect(edit.value).toBe("Before\n\n![](media:m1)\n\n![](media:m2)\n\nAfter");
    expect(edit.value.slice(edit.selectionStart)).toBe("\nAfter");
  });

  test("adds no blank lines at the start or the end of the text", () => {
    expect(insertImages("", 0, 0, ["m1"])).toEqual({ value: "![](media:m1)", selectionStart: 13, selectionEnd: 13 });
  });

  test("replaces the selection", () => {
    expect(insertImages("one\n\ntwo\n\nthree", 5, 8, ["m1"]).value).toBe("one\n\n![](media:m1)\n\nthree");
  });
});

describe("parseTags", () => {
  test("reads hashtags separated by spaces or commas, with or without #, each once", () => {
    expect(parseTags("#garden tomatoes, #Garden  ＃balcony、summer")).toEqual([
      "garden",
      "tomatoes",
      "balcony",
      "summer",
    ]);
  });

  test("reads nothing from blank text", () => {
    expect(parseTags("  ,  ")).toEqual([]);
  });

  test("writes saved hashtags back as the field shows them", () => {
    expect(tagsText(["garden", "#summer"])).toBe("#garden #summer");
  });
});
