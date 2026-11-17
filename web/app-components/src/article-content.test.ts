// SPDX-License-Identifier: AGPL-3.0-only

// An article's text (ADR-0160 §4, §10, #3089): the article's allowlist, which a post's never keeps (html.test.ts), as
// the parser reads it; and the excerpt of an article, its title. The blocks it draws are in post-content.dom.test.tsx.

import { describe, expect, test } from "bun:test";
import { articleBlockTags, blockTags, parseContent, type ContentNode } from "./html";
import { excerptHtml } from "./post-content";

/** The one element of an article's text `html`, a block. */
const only = (html: string): ContentNode => {
  const nodes = parseContent(html, "article");
  expect(nodes).toHaveLength(1);
  return nodes[0] ?? "";
};

/** The first image of an article's text, at any depth, or none. */
const imageOf = (nodes: ContentNode[]): ContentNode | undefined => {
  for (const node of nodes) {
    if (typeof node === "string") continue;
    if (node.tag === "img") return node;
    const inner = imageOf(node.children);
    if (inner !== undefined) return inner;
  }
  return undefined;
};

describe("an article's allowlist", () => {
  test("headings h2 to h6, a rule, an image and a table are kept", () => {
    const tags = parseContent(
      '<h2>a</h2><h3>b</h3><h4>c</h4><h5>d</h5><h6>e</h6><hr><p><img src="https://my.example/a.png" alt="x"></p><table><tbody><tr><td>f</td></tr></tbody></table>',
      "article",
    ).map((n) => (typeof n === "string" ? n : n.tag));
    expect(tags).toEqual(["h2", "h3", "h4", "h5", "h6", "hr", "p", "table"]);
  });

  test("an image keeps an http or https src, its alt, a positive integer size and when it loads", () => {
    const image = imageOf(
      parseContent(
        '<p><img src="https://my.example/a.png" alt="A loaf" width="1200" height="800" loading="lazy"></p>',
        "article",
      ),
    );
    expect(image).toEqual({
      tag: "img",
      attrs: { src: "https://my.example/a.png", alt: "A loaf", width: "1200", height: "800", loading: "lazy" },
      children: [],
    });
    const http = imageOf(parseContent('<p><img src="http://my.example/a.png" alt="x" loading="eager"></p>', "article"));
    expect(http).toEqual({
      tag: "img",
      attrs: { src: "http://my.example/a.png", alt: "x", loading: "eager" },
      children: [],
    });
  });

  test("an image drops its other attributes, and a size or loading the allowlist does not know", () => {
    for (const [width, height] of [
      ["0", "-1"],
      ["1.5", "abc"],
      ["100%", "1e3"],
      ["", " "],
    ]) {
      const image = imageOf(
        parseContent(
          `<p><img src="https://my.example/a.png" alt="x" width="${width}" height="${height}" loading="auto" onerror="alert(1)" style="position:fixed" srcset="https://evil.example/a.png 2x" class="big" id="i" title="t"></p>`,
          "article",
        ),
      );
      expect(image).toEqual({ tag: "img", attrs: { src: "https://my.example/a.png", alt: "x" }, children: [] });
    }
  });

  test("an image with no http or https src reads as its alt text", () => {
    for (const src of [
      "javascript:alert(1)",
      "JaVaScRiPt:alert(1)",
      "data:image/png;base64,iVBORw0KGgo=",
      "file:///etc/passwd",
      "/media/a.png",
      "//evil.example/a.png",
      "a.png",
      "",
    ]) {
      const nodes = parseContent(`<p>see <img src="${src}" alt="the shop on a map"> here</p>`, "article");
      expect([src, nodes]).toEqual([src, [{ tag: "p", attrs: {}, children: ["see the shop on a map here"] }]]);
    }
    expect(imageOf(parseContent('<p><img alt="no src"></p>', "article"))).toBeUndefined();
  });

  test("a cell keeps an alignment of left, center or right, and nothing else", () => {
    for (const align of ["left", "center", "right"] as const) {
      const table = only(
        `<table><tbody><tr><td align="${align}">a</td><th align="${align}">b</th></tr></tbody></table>`,
      );
      expect(table).toEqual({
        tag: "table",
        attrs: {},
        children: [
          {
            tag: "tbody",
            attrs: {},
            children: [
              {
                tag: "tr",
                attrs: {},
                children: [
                  { tag: "td", attrs: { align }, children: ["a"] },
                  { tag: "th", attrs: { align }, children: ["b"] },
                ],
              },
            ],
          },
        ],
      });
    }
    for (const align of ["justify", "char", "middle", "", "right; color: red"]) {
      const table = only(
        `<table style="x" border="1"><tbody><tr onclick="x"><td align="${align}" colspan="2" style="color:red">a</td></tr></tbody></table>`,
      );
      expect(table).toEqual({
        tag: "table",
        attrs: {},
        children: [
          {
            tag: "tbody",
            attrs: {},
            children: [{ tag: "tr", attrs: {}, children: [{ tag: "td", attrs: {}, children: ["a"] }] }],
          },
        ],
      });
    }
  });

  test("a row directly in a table goes in a tbody", () => {
    expect(only("<table><tr><td>a</td></tr><tr><td>b</td></tr></table>")).toEqual({
      tag: "table",
      attrs: {},
      children: [
        {
          tag: "tbody",
          attrs: {},
          children: [
            { tag: "tr", attrs: {}, children: [{ tag: "td", attrs: {}, children: ["a"] }] },
            { tag: "tr", attrs: {}, children: [{ tag: "td", attrs: {}, children: ["b"] }] },
          ],
        },
      ],
    });
  });

  test("what neither allowlist keeps goes as in a post: a script with its text, an iframe, a link of another scheme", () => {
    expect(
      parseContent(
        '<p>a<script>alert(1)</script>b<iframe src="https://evil.example/">f</iframe>c<a href="javascript:x">d</a></p>',
        "article",
      ),
    ).toEqual([{ tag: "p", attrs: {}, children: ["abcd"] }]);
  });

  test("the article's blocks are a post's, its headings, its rule and its table", () => {
    expect([...articleBlockTags].sort()).toEqual([...blockTags, "h2", "h3", "h4", "h5", "h6", "hr", "table"].sort());
  });

  test("a heading closes an open paragraph, as a post's block does", () => {
    expect(parseContent("<p>a<h2>b</h2>", "article")).toEqual([
      { tag: "p", attrs: {}, children: ["a"] },
      { tag: "h2", attrs: {}, children: ["b"] },
    ]);
  });
});

describe("the excerpt of a status", () => {
  test("a post's is its text", () => {
    expect(excerptHtml({ content: "<p>Day 3</p>" })).toBe("<p>Day 3</p>");
  });

  test("an article's is its title as a paragraph, in place of the empty text a list gives", () => {
    expect(excerptHtml({ content: "", article: { title: "A sourdough starter in seven days" } })).toBe(
      "<p>A sourdough starter in seven days</p>",
    );
  });

  test("the title is text: markup in it never draws", () => {
    const html = excerptHtml({ content: "", article: { title: "<script>alert(1)</script> & <b>bold</b>" } });
    expect(parseContent(html)).toEqual([
      { tag: "p", attrs: {}, children: ["<script>alert(1)</script> & <b>bold</b>"] },
    ]);
  });
});
