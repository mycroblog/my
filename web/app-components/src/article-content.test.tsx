// SPDX-License-Identifier: AGPL-3.0-only

// An article's text (ADR-0160 §4, §10, #3089): the article's allowlist, which a post's never keeps, and the blocks it
// draws as long reading; and the excerpt of an article, its title.

import { describe, expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import { articleBlockTags, blockTags, parseContent, type ContentNode } from "./html";
import { excerptHtml, PostContent, type ContentEmoji } from "./post-content";

/** The article's text as PostContent draws it, without the design system's classes. */
const article = (html: string, emojis?: ContentEmoji[]) =>
  renderToStaticMarkup(<PostContent html={html} emojis={emojis} lang="en" article />).replace(/ class="[^"]*"/g, "");

/** A post's text as PostContent draws it, without the design system's classes and the body around it. */
const post = (html: string) =>
  renderToStaticMarkup(<PostContent html={html} lang="en" />)
    .replace(/ class="[^"]*"/g, "")
    .replace(/^<div lang="en">|<\/div>$/g, "");

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

const blobcat: ContentEmoji = {
  shortcode: "blobcat",
  url: "https://m.example/blobcat.gif",
  static_url: "https://m.example/blobcat.png",
};

// What the server renders of the scenario's article (internal/adapter/markdown), the shape every test reads.
const local =
  "<p>Seven days ago I mixed flour and water in a jar.</p>" +
  "<h2>The first three days</h2>" +
  "<p>A smell like <strong>old cheese</strong>.</p>" +
  "<ul><li>Day 1: 50 g flour</li><li>Day 2: the same</li></ul>" +
  "<blockquote><p>A starter is ready when it doubles.</p></blockquote>" +
  '<table><thead><tr><th>Day</th><th align="right">Flour</th></tr></thead><tbody><tr><td>1</td><td align="right">50 g</td></tr></tbody></table>' +
  "<h3>The mistakes</h3>" +
  '<p><img src="https://my.example/media/large/loaf.webp" alt="A loaf" width="1200" height="800" loading="lazy"></p>' +
  "<pre><code>feed: 1 starter : 1 flour</code></pre>" +
  "<hr>" +
  "<p>Next week: the first loaf.</p>";

describe("an article's allowlist", () => {
  test("headings h2 to h6, a rule, an image and a table are kept", () => {
    const tags = parseContent(
      '<h2>a</h2><h3>b</h3><h4>c</h4><h5>d</h5><h6>e</h6><hr><p><img src="https://my.example/a.png" alt="x"></p><table><tbody><tr><td>f</td></tr></tbody></table>',
      "article",
    ).map((n) => (typeof n === "string" ? n : n.tag));
    expect(tags).toEqual(["h2", "h3", "h4", "h5", "h6", "hr", "p", "table"]);
  });

  test("a post's text never keeps them: headings and cells are their text, a rule and an image are nothing", () => {
    expect(post('<h2>Title</h2><hr><p>a<img src="https://my.example/a.png" alt="pic">b</p>')).toBe(
      '<p dir="auto">Title</p><p dir="auto">ab</p>',
    );
    expect(
      post('<table><thead><tr><th align="right">h</th></tr></thead><tbody><tr><td>c</td></tr></tbody></table>'),
    ).toBe('<p dir="auto">hc</p>');
    expect(parseContent("<h3>x</h3><hr>")).toEqual(["x"]);
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
      expect(imageOf(nodes)).toBeUndefined();
      expect(article(`<p>see <img src="${src}" alt="the shop on a map"> here</p>`)).toBe(
        '<p dir="auto">see the shop on a map here</p>',
      );
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
      article(
        '<p>a<script>alert(1)</script>b<iframe src="https://evil.example/">f</iframe>c<a href="javascript:x">d</a></p>',
      ),
    ).toBe('<p dir="auto">abcd</p>');
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

describe("an article's text as PostContent draws it", () => {
  test("its blocks, each in its own direction, with no body of a post around them", () => {
    const html = article(local);
    expect(html).toStartWith('<p dir="auto">Seven days ago');
    expect(html).not.toContain('<div lang="en">');
    expect(html).toContain('<h2 dir="auto">The first three days</h2>');
    expect(html).toContain('<h3 dir="auto">The mistakes</h3>');
    expect(html).toContain('<p dir="auto">A smell like <strong>old cheese</strong>.</p>');
    expect(html).toContain('<ul dir="auto"><li dir="auto">Day 1: 50 g flour</li>');
    expect(html).toContain('<blockquote dir="auto"><p dir="auto">A starter is ready when it doubles.</p></blockquote>');
    expect(html).toContain("<hr");
    expect(html).toEndWith('<p dir="auto">Next week: the first loaf.</p>');
  });

  test("a table in a box that scrolls and takes the focus, read in its own direction, its alignment kept", () => {
    // Its language is the holder's, ArticleText's, as the rest of the text's.
    const html = article(local);
    expect(html).toMatch(/<div tabindex="0"><table dir="auto"><thead><tr><th>Day<\/th><th align="right">Flour<\/th>/);
    expect(html).toContain('<td align="right">50 g</td>');
  });

  test("an image with its source, size and description", () => {
    const html = article(local);
    expect(html).toMatch(
      /<img [^>]*src="https:\/\/my.example\/media\/large\/loaf.webp"[^>]*alt="A loaf"[^>]*width="1200"[^>]*height="800"[^>]*loading="lazy"/,
    );
  });

  test("code breaks as in a post, and a custom emoji draws in its line but not in code", () => {
    expect(article("<pre><code>a://b?c|d/</code></pre>")).toBe(
      '<pre dir="auto"><code>a://<wbr/>b?<wbr/>c<wbr/>|d/</code></pre>',
    );
    const html = article("<h2>:blobcat: day</h2><p><code>:blobcat:</code></p>", [blobcat]);
    expect(html).toMatch(/^<h2 dir="auto"><picture>.*<\/picture> day<\/h2>/);
    expect(html).toContain("<code>:blobcat:</code>");
  });

  test("a hashtag and a mention are links as in a post", () => {
    const html = article(
      '<p><a href="https://my.example/tags/bread" class="mention hashtag" rel="tag">#<span>bread</span></a> <span class="h-card"><a href="https://my.example/@sam" class="u-url mention">@<span>sam</span></a></span></p>',
    );
    expect(html).toContain(
      '<a href="https://my.example/tags/bread" rel="nofollow noopener noreferrer" target="_blank">#<span>bread</span></a>',
    );
    expect(html).toContain(
      '<a href="https://my.example/@sam" rel="nofollow noopener noreferrer" target="_blank">@<span>sam</span></a>',
    );
  });

  test("text outside a block is a paragraph, and an empty text is nothing", () => {
    expect(article("just text")).toBe('<p dir="auto">just text</p>');
    expect(article("")).toBe("");
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
