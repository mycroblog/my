// SPDX-License-Identifier: AGPL-3.0-only

// What a post's text and an article's draw from the HTML the server rendered: each block in its own direction, links
// that leave the app in a new tab and hashtags of this server that stay in it, custom emoji as images, and nothing of
// what the allowlist drops. What the allowlist keeps and drops is the parser's (html.test.ts, article-content.test.ts).

import { describe, expect, test } from "bun:test";
import { cleanup, render, screen, within } from "@testing-library/react";
import { PostContent, ServerDomain, type ContentEmoji } from "./post-content";

const blobcat: ContentEmoji = {
  shortcode: "blobcat",
  url: "https://m.example/blobcat.gif",
  static_url: "https://m.example/blobcat.png",
};

/** The text of `html` as a post draws it, alone in the document. */
function drawn(html: string, emojis?: ContentEmoji[]) {
  cleanup();
  return render(<PostContent html={html} emojis={emojis} lang="en" />).container;
}

/** An article's text of `html`, alone in the document. */
function article(html: string, emojis?: ContentEmoji[]) {
  cleanup();
  return render(<PostContent html={html} emojis={emojis} lang="en" article />).container;
}

/** The names of the elements of `root`, in the document's order. */
const tags = (root: Element) => Array.from(root.querySelectorAll("*"), (element) => element.tagName.toLowerCase());

/** What a run of code holds: its text, and `<wbr>` for each place it may break at. */
const breaks = (code: Element) =>
  Array.from(code.childNodes, (node) =>
    node instanceof HTMLElement ? `<${node.tagName.toLowerCase()}>` : node.textContent,
  );

describe("a post's text", () => {
  test("is in its language, each paragraph in its own direction, a line break kept", () => {
    const container = drawn("<p>Hello!<br>Line two</p><p>مرحبا</p>");
    const [first, second] = screen.getAllByRole("paragraph");

    expect(first?.textContent).toBe("Hello!Line two");
    expect(first !== undefined && tags(first)).toEqual(["br"]);
    expect([first?.dir, second?.dir]).toEqual(["auto", "auto"]);
    expect(first?.closest("[lang]")?.getAttribute("lang")).toBe("en");
    expect(container.textContent).toBe("Hello!Line twoمرحبا");
  });

  test("each block reads in its own direction, whatever the page's", () => {
    const container = drawn(
      "<p>مرحبا بالعالم!</p><blockquote><p>Hello!</p></blockquote><ul><li>אחד</li><li>two</li></ul><ol><li>one</li></ol><pre>code</pre>",
    );

    const blocks = Array.from(container.querySelectorAll("p, blockquote, ul, ol, li, pre"));
    expect(blocks.map((block) => block.tagName.toLowerCase())).toEqual([
      "p",
      "blockquote",
      "p",
      "ul",
      "li",
      "li",
      "ol",
      "li",
      "pre",
    ]);
    expect(blocks.map((block) => block.getAttribute("dir"))).toEqual(Array.from(blocks, () => "auto"));
    expect(within(screen.getAllByRole("list")[0] as HTMLElement).getAllByRole("listitem")).toHaveLength(2);
  });

  test("text outside a block is a paragraph of its own; blank text between blocks is none; an empty text draws no block", () => {
    drawn("Just text<br>and a line");
    expect(screen.getAllByRole("paragraph").map((p) => p.textContent)).toEqual(["Just textand a line"]);

    drawn("<p>one</p>\n<p>two</p>");
    expect(screen.getAllByRole("paragraph").map((p) => p.textContent)).toEqual(["one", "two"]);

    expect(drawn("").textContent).toBe("");
    expect(screen.queryAllByRole("paragraph")).toHaveLength(0);
  });

  test("a list keeps its numbering, and marked words their marks", () => {
    drawn(
      '<ol start="3" reversed><li value="7">x</li></ol><p lang="ja"><del>a</del><s>b</s><code>c</code><b>d</b><strong>e</strong><u>f</u><i>g</i><em>h</em></p><p><ruby>漢<rp>(</rp><rt>かん</rt><rp>)</rp></ruby></p>',
    );
    const list = screen.getByRole<HTMLOListElement>("list");

    expect([list.start, list.reversed]).toEqual([3, true]);
    expect(within(list).getByRole<HTMLLIElement>("listitem").value).toBe(7);
    const [marked, ruby] = screen.getAllByRole("paragraph");
    expect(marked?.lang).toBe("ja");
    expect(marked !== undefined && tags(marked)).toEqual(["del", "s", "code", "b", "strong", "u", "i", "em"]);
    expect(ruby !== undefined && tags(ruby)).toEqual(["ruby", "rp", "rt", "rp"]);
  });

  test("a link leaves the app in a new tab, without telling where it came from, whatever the HTML said of that", () => {
    drawn(
      '<p><span class="h-card" translate="no"><a href="https://mastodon.example/@alice" class="u-url mention">@<span>alice</span></a></span> and <a href="https://e.example/" target="_self" rel="opener">a page</a></p>',
    );
    const mention = screen.getByRole<HTMLAnchorElement>("link", { name: /^@\s*alice$/ });
    const page = screen.getByRole<HTMLAnchorElement>("link", { name: "a page" });

    for (const [link, href] of [
      [mention, "https://mastodon.example/@alice"],
      [page, "https://e.example/"],
    ] as const) {
      expect(link.getAttribute("href")).toBe(href);
      expect(link.target).toBe("_blank");
      expect(link.rel.split(" ").sort()).toEqual(["nofollow", "noopener", "noreferrer"]);
    }
    // A mention is not to be translated with the text around it.
    expect(mention.closest("[translate]")?.getAttribute("translate")).toBe("no");
  });

  test("a shortened URL is a link to the whole address, whose text is the whole address", () => {
    drawn(
      '<p><a href="https://example.jp/blog/2026/11/small-server-from-scratch"><span class="invisible">https://</span><span class="ellipsis">example.jp/blog/2026/11/small-</span><span class="invisible">server-from-scratch</span></a></p>',
    );

    const link = screen.getByRole("link");
    expect(link.getAttribute("href")).toBe("https://example.jp/blog/2026/11/small-server-from-scratch");
    expect(link.textContent).toBe("https://example.jp/blog/2026/11/small-server-from-scratch");
  });

  test("what the allowlist drops is not drawn: markup that would run or load something draws as its text, or nothing", () => {
    const container = drawn(
      '<p onclick="alert(1)" style="position:fixed" id="x">a<script>alert(1)</script><img src="https://evil.example/x.png" onerror="alert(1)"><iframe src="https://evil.example/"></iframe><a href="javascript:alert(1)">b</a> &lt;script&gt;alert(2)&lt;/script&gt;</p><style>p{color:red}</style>',
    );

    expect(tags(container).filter((tag) => tag !== "div")).toEqual(["p"]);
    expect(screen.getByRole("paragraph").textContent).toBe("ab <script>alert(2)</script>");
    const attributes = Array.from(container.querySelectorAll("*")).flatMap((element) => element.getAttributeNames());
    expect(
      attributes.filter((name) => name.startsWith("on") || name === "style" || name === "id" || name === "src"),
    ).toEqual([]);
  });
});

describe("a hashtag in a post", () => {
  const html =
    '<p><a href="https://my.example/tags/fediverse" class="mention hashtag" rel="tag">#<span>fediverse</span></a> ' +
    '<a href="https://other.example/tags/cats" class="mention hashtag" rel="tag">#<span>cats</span></a></p>';
  /** The hashtags' links, where the server's domain is `domain`. */
  function hashtags(domain: string | undefined) {
    cleanup();
    render(
      <ServerDomain value={domain}>
        <PostContent html={html} lang="en" />
      </ServerDomain>,
    );
    return {
      fediverse: screen.getByRole<HTMLAnchorElement>("link", { name: /^#\s*fediverse$/ }),
      cats: screen.getByRole<HTMLAnchorElement>("link", { name: /^#\s*cats$/ }),
    };
  }

  test("of this server opens the app's hashtag screen in place; another server's opens in a new tab", () => {
    const { fediverse, cats } = hashtags("my.example");

    expect([fediverse.getAttribute("href"), fediverse.target]).toEqual(["/tags/fediverse", ""]);
    expect([cats.getAttribute("href"), cats.target]).toEqual(["https://other.example/tags/cats", "_blank"]);
  });

  test("before the server's domain is known, opens as the server wrote it", () => {
    const { fediverse } = hashtags(undefined);

    expect([fediverse.getAttribute("href"), fediverse.target]).toEqual(["https://my.example/tags/fediverse", "_blank"]);
  });
});

describe("custom emoji", () => {
  test("a shortcode of the text's emoji draws as its image, named by its shortcode, and moves only for a reader who allows motion", () => {
    drawn("<p>look :blobcat:</p>", [blobcat]);
    const image = screen.getByRole<HTMLImageElement>("img", { name: ":blobcat:" });

    expect(image.getAttribute("src")).toBe("https://m.example/blobcat.png");
    expect(image.title).toBe(":blobcat:");
    const moving = image.closest("picture")?.querySelector("source");
    expect(moving?.getAttribute("srcset")).toBe("https://m.example/blobcat.gif");
    expect(moving?.media).toBe("(prefers-reduced-motion: no-preference)");
  });

  test("only a shortcode of the text's emoji draws as an image; another stays text", () => {
    drawn("<p>:blobcat: and :unknown: and :blobcat:</p>", [blobcat]);

    expect(screen.getAllByRole("img", { name: ":blobcat:" })).toHaveLength(2);
    expect(screen.getByRole("paragraph").textContent).toBe(" and :unknown: and ");
  });

  test("code shows what its author typed, colons included", () => {
    const container = drawn("<p><code>:blobcat:</code></p><pre>:blobcat:</pre>", [blobcat]);

    expect(screen.queryAllByRole("img")).toHaveLength(0);
    expect(container.textContent).toBe(":blobcat::blobcat:");
  });

  test("an emoji with no animation has no source to move", () => {
    drawn("<p>:blobcat:</p>", [{ ...blobcat, url: blobcat.static_url }]);

    expect(
      screen.getByRole("img", { name: ":blobcat:" }).closest("picture")?.querySelector("source") ?? null,
    ).toBeNull();
  });
});

describe("code", () => {
  test("may break after a slash or a question mark and before a pipe, and only there; its text stays whole", () => {
    const block = drawn("<pre><code>a://b?c|d/</code></pre>").querySelector("code");
    expect(block !== null && breaks(block)).toEqual(["a://", "<wbr>", "b?", "<wbr>", "c", "<wbr>", "|d/"]);
    expect(block?.textContent).toBe("a://b?c|d/");

    const inline = drawn("<p>a/b <code>e/f</code></p>");
    expect(tags(screen.getByRole("paragraph"))).toEqual(["code", "wbr"]);
    expect(inline.textContent).toBe("a/b e/f");
  });
});

describe("an article's text", () => {
  // What the server renders of the scenario's article (internal/adapter/markdown).
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

  test("its blocks, each in its own direction, with no body of a post around them", () => {
    const container = article(local);

    expect(Array.from(container.children, (block) => block.tagName.toLowerCase())).toEqual([
      "p",
      "h2",
      "p",
      "ul",
      "blockquote",
      "div",
      "h3",
      "p",
      "pre",
      "hr",
      "p",
    ]);
    expect(screen.getByRole("heading", { level: 2, name: "The first three days" }).dir).toBe("auto");
    expect(screen.getByRole("heading", { level: 3, name: "The mistakes" }).dir).toBe("auto");
    expect(screen.getByText("Seven days ago I mixed flour and water in a jar.").dir).toBe("auto");
    expect(screen.getByRole("separator")).not.toBeNull();
    // Its language is its holder's, ArticleText's, not its own.
    expect(container.querySelector("[lang]")).toBeNull();
  });

  test("a table with its header and its cells, a cell's alignment kept", () => {
    article(local);
    const table = within(screen.getByRole("table"));

    expect(table.getAllByRole("columnheader").map((cell) => [cell.textContent, cell.getAttribute("align")])).toEqual([
      ["Day", null],
      ["Flour", "right"],
    ]);
    expect(table.getAllByRole("cell").map((cell) => [cell.textContent, cell.getAttribute("align")])).toEqual([
      ["1", null],
      ["50 g", "right"],
    ]);
  });

  test("an image with its source, size and description, loading as the server said", () => {
    article(local);
    const image = screen.getByRole<HTMLImageElement>("img", { name: "A loaf" });

    expect(image.getAttribute("src")).toBe("https://my.example/media/large/loaf.webp");
    expect([image.getAttribute("width"), image.getAttribute("height"), image.getAttribute("loading")]).toEqual([
      "1200",
      "800",
      "lazy",
    ]);
  });

  test("an image with no http or https source reads as its description", () => {
    article('<p>see <img src="data:image/png;base64,iVBORw0KGgo=" alt="the shop on a map"> here</p>');

    expect(screen.queryAllByRole("img")).toHaveLength(0);
    expect(screen.getByRole("paragraph").textContent).toBe("see the shop on a map here");
  });

  test("code breaks as in a post, and a custom emoji draws in its line but not in code", () => {
    const code = article("<pre><code>a://b?c|d/</code></pre>").querySelector("code");
    expect(code !== null && breaks(code)).toEqual(["a://", "<wbr>", "b?", "<wbr>", "c", "<wbr>", "|d/"]);

    article("<h2>:blobcat: day</h2><p><code>:blobcat:</code></p>", [blobcat]);
    expect(within(screen.getByRole("heading", { level: 2 })).getAllByRole("img", { name: ":blobcat:" })).toHaveLength(
      1,
    );
    expect(within(screen.getByRole("paragraph")).queryAllByRole("img")).toHaveLength(0);
  });

  test("a hashtag and a mention are links as in a post", () => {
    article(
      '<p><a href="https://my.example/tags/bread" class="mention hashtag" rel="tag">#<span>bread</span></a> <span class="h-card"><a href="https://my.example/@sam" class="u-url mention">@<span>sam</span></a></span></p>',
    );

    for (const [name, href] of [
      [/^#\s*bread$/, "https://my.example/tags/bread"],
      [/^@\s*sam$/, "https://my.example/@sam"],
    ] as const) {
      const link = screen.getByRole<HTMLAnchorElement>("link", { name });
      expect([link.getAttribute("href"), link.target]).toEqual([href, "_blank"]);
      expect(link.rel.split(" ").sort()).toEqual(["nofollow", "noopener", "noreferrer"]);
    }
  });

  test("text outside a block is a paragraph, and an empty text is nothing", () => {
    article("just text");
    expect(screen.getAllByRole("paragraph").map((p) => [p.textContent, p.dir])).toEqual([["just text", "auto"]]);

    expect(article("").childElementCount).toBe(0);
  });
});
