// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import { parseContent } from "./html";
import { PostContent, type ContentEmoji } from "./post-content";

const render = (html: string, emojis?: ContentEmoji[]) =>
  renderToStaticMarkup(<PostContent html={html} emojis={emojis} lang="en" />);

/** The markup without the design system's classes, to read the structure. */
const bare = (html: string, emojis?: ContentEmoji[]) => render(html, emojis).replace(/ class="[^"]*"/g, "");

const inner = (html: string, emojis?: ContentEmoji[]) => bare(html, emojis).replace(/^<div lang="en">|<\/div>$/g, "");

const blobcat: ContentEmoji = {
  shortcode: "blobcat",
  url: "https://m.example/blobcat.gif",
  static_url: "https://m.example/blobcat.png",
};

describe("local and remote text, through the one renderer", () => {
  test("a local post as the server renders it: paragraphs, line breaks, a mention, a hashtag and a shortened URL", () => {
    // internal/app/content/text.go: ParsedText.HTML.
    const local =
      '<p>Hello <span class="h-card"><a href="https://my.example/@mika" class="u-url mention" rel="nofollow noopener noreferrer" target="_blank">@<span>mika</span></a></span>!<br>Line two</p>' +
      '<p><a href="https://my.example/tags/fediverse" class="mention hashtag" rel="nofollow noopener noreferrer" target="_blank">#<span>fediverse</span></a> ' +
      '<a href="https://example.jp/blog/2026/11/small-server-from-scratch" rel="nofollow noopener noreferrer" target="_blank"><span class="invisible">https://</span><span class="ellipsis">example.jp/blog/2026/11/small-</span><span class="invisible">server-from-scratch</span></a></p>';
    const html = render(local);
    expect(html).toMatch(/^<div class="[^"]*gap-content-paragraph[^"]*" lang="en"><p dir="auto">Hello <span>/);
    expect(html).toContain('!<br/>Line two</p><p dir="auto">');
    expect(html).toMatch(
      /<a class="[^"]*whitespace-nowrap[^"]*" href="https:\/\/my.example\/@mika"[^>]*>@<span>mika<\/span><\/a>/,
    );
    expect(html).toMatch(
      /<a class="[^"]*whitespace-nowrap[^"]*" href="https:\/\/my.example\/tags\/fediverse"[^>]*>#<span>fediverse<\/span><\/a>/,
    );
    expect(html).toMatch(/<span class="[^"]*size-0[^"]*">https:\/\/<\/span>/);
    expect(html).toMatch(/<span class="[^"]*after:content-[^"]*">example.jp\/blog\/2026\/11\/small-<\/span>/);
    expect(html).toMatch(/<span class="[^"]*size-0[^"]*">server-from-scratch<\/span>/);
  });

  test("a remote post as Mastodon writes it, kept to the allowlist", () => {
    const remote =
      '<p><span class="h-card" translate="no"><a href="https://mastodon.example/@alice" class="u-url mention">@<span>alice</span></a></span> look at <a href="https://mastodon.example/tags/cats" class="mention hashtag" rel="tag">#<span>cats</span></a> :blobcat:</p>';
    expect(inner(remote, [blobcat])).toBe(
      '<p dir="auto"><span translate="no"><a href="https://mastodon.example/@alice" rel="nofollow noopener noreferrer" target="_blank">@<span>alice</span></a></span> look at ' +
        '<a href="https://mastodon.example/tags/cats" rel="nofollow noopener noreferrer" target="_blank">#<span>cats</span></a> ' +
        '<picture><source media="(prefers-reduced-motion: no-preference)" srcSet="https://m.example/blobcat.gif"/><img src="https://m.example/blobcat.png" alt=":blobcat:" title=":blobcat:" draggable="false"/></picture></p>',
    );
  });

  test("each block reads in its own direction, whatever the page's", () => {
    const html = inner(
      "<p>مرحبا بالعالم!</p><blockquote><p>Hello!</p></blockquote><ul><li>אחד</li><li>two</li></ul><pre>code</pre>",
    );
    expect(html).toBe(
      '<p dir="auto">مرحبا بالعالم!</p><blockquote dir="auto"><p dir="auto">Hello!</p></blockquote>' +
        '<ul dir="auto"><li dir="auto">אחד</li><li dir="auto">two</li></ul><pre dir="auto">code</pre>',
    );
  });

  test("text outside a block is a paragraph of its own; blank text between blocks is none", () => {
    expect(inner("Just text<br>and a line")).toBe('<p dir="auto">Just text<br/>and a line</p>');
    expect(inner("<p>one</p>\n<p>two</p>")).toBe('<p dir="auto">one</p><p dir="auto">two</p>');
    expect(inner("")).toBe("");
  });

  test("the elements of the allowlist keep what the server allows of their attributes", () => {
    expect(inner('<ol start="3" reversed><li value="7">x</li></ol>')).toBe(
      '<ol dir="auto" start="3" reversed=""><li dir="auto" value="7">x</li></ol>',
    );
    expect(
      inner('<p lang="ja"><del>a</del><s>b</s><code>c</code><b>d</b><strong>e</strong><u>f</u><i>g</i><em>h</em></p>'),
    ).toBe(
      '<p lang="ja" dir="auto"><del>a</del><s>b</s><code>c</code><b>d</b><strong>e</strong><u>f</u><i>g</i><em>h</em></p>',
    );
    expect(inner("<p><ruby>漢<rp>(</rp><rt>かん</rt><rp>)</rp></ruby></p>")).toBe(
      '<p dir="auto"><ruby>漢<rp>(</rp><rt>かん</rt><rp>)</rp></ruby></p>',
    );
  });
});

describe("custom emoji", () => {
  test("only a shortcode of the text's emoji draws as its image; another stays text", () => {
    const html = inner("<p>:blobcat: and :unknown: and :blobcat:</p>", [blobcat]);
    expect(html.match(/<picture>/g)).toHaveLength(2);
    expect(html).toContain(" and :unknown: and ");
  });

  test("code shows what its author typed, colons included", () => {
    expect(inner("<p><code>:blobcat:</code></p><pre>:blobcat:</pre>", [blobcat])).not.toContain("<picture>");
  });

  test("code may break after a slash or a question mark and before a pipe, and only there", () => {
    expect(inner("<pre><code>a://b?c|d/</code></pre>")).toBe(
      '<pre dir="auto"><code>a://<wbr/>b?<wbr/>c<wbr/>|d/</code></pre>',
    );
    expect(inner("<p>a/b <code>e/f</code></p>")).toBe('<p dir="auto">a/b <code>e/<wbr/>f</code></p>');
  });

  test("an emoji with no animation has no source to move", () => {
    const still = { ...blobcat, url: blobcat.static_url };
    expect(inner("<p>:blobcat:</p>", [still])).not.toContain("<source");
  });
});

describe("what the server would never let through draws as nothing or as text", () => {
  test("a script, a style, an iframe and the like go with their contents", () => {
    const html = inner(
      "<p>a<script>alert(1)</script>b<style>p{color:red}</style>c<iframe src=x>frame</iframe>d<svg><text>svg</text></svg>e<template><p>t</p></template>f</p>",
    );
    expect(html).toBe('<p dir="auto">abcdef</p>');
  });

  test("an end tag in a script's text does not end the script early, and an unclosed script takes the rest", () => {
    expect(inner("<p>a<script>document.write('<p>x</p>')</script>b</p>")).toBe('<p dir="auto">ab</p>');
    expect(inner("<p>a</p><script>alert(1)")).toBe('<p dir="auto">a</p>');
  });

  test("an image, a form control and other elements outside the allowlist are dropped or unwrapped to their text", () => {
    expect(inner('<p>a<img src=x onerror="alert(1)">b<input value=c>d</p>')).toBe('<p dir="auto">abd</p>');
    expect(inner("<div><h1>Title</h1><table><tr><td>cell</td></tr></table></div>")).toBe('<p dir="auto">Titlecell</p>');
    expect(inner('<p><font color="red">red</font> <marquee>moves</marquee></p>')).toBe('<p dir="auto">red moves</p>');
  });

  test("event handlers, styles, ids and classes outside the server's are not kept", () => {
    const html = bare(
      '<p onclick="alert(1)" style="position:fixed" id="x" class="invisible">a <span onmouseover="alert(1)" style="color:red">b</span> <a href="https://e.example/" onclick="alert(1)" style="x" target="_self" rel="opener">c</a></p>',
    );
    expect(html).toBe(
      '<div lang="en"><p dir="auto">a <span>b</span> <a href="https://e.example/" rel="nofollow noopener noreferrer" target="_blank">c</a></p></div>',
    );
  });

  test("a link of a scheme outside the allowlist, or relative, or with no href, is its text", () => {
    for (const href of [
      "javascript:alert(1)",
      "JaVaScRiPt:alert(1)",
      "&#106;avascript:alert(1)",
      "java\tscript:alert(1)",
      " javascript:alert(1)",
      "data:text/html,<script>alert(1)</script>",
      "vbscript:x",
      "/relative",
      "//example.com/",
      "",
    ]) {
      expect(inner(`<p><a href="${href.replaceAll('"', "&quot;")}">go</a></p>`)).toBe('<p dir="auto">go</p>');
    }
    expect(inner("<p><a>go</a></p>")).toBe('<p dir="auto">go</p>');
    expect(inner('<p><a href="gemini://g.example/">g</a></p>')).toContain('href="gemini://g.example/"');
  });

  test("character references read as text, and markup in them stays text", () => {
    expect(inner("<p>&lt;script&gt;alert(&#39;x&#39;)&lt;/script&gt; &amp; &#x1F600; &#0; &copy;</p>")).toBe(
      '<p dir="auto">&lt;script&gt;alert(&#x27;x&#x27;)&lt;/script&gt; &amp; 😀 � &amp;copy;</p>',
    );
  });

  test("comments and bogus markup are dropped; a stray < is text", () => {
    expect(inner("<p>a<!-- <script>alert(1)</script> -->b<!doctype html>c<?xml x?>d</p>")).toBe(
      '<p dir="auto">abcd</p>',
    );
    expect(inner("<p>1 < 2 and 3 <4</p>")).toBe('<p dir="auto">1 &lt; 2 and 3 &lt;4</p>');
    expect(inner('<p>a<span title="unclosed>b</p>')).toBe('<p dir="auto">a&lt;span title=&quot;unclosed&gt;b</p>');
  });

  test("blocks never nest in a paragraph, links never nest in a link, and list items stay in a list", () => {
    expect(inner("<p>a<p>b<ul><li>c<li>d</ul>")).toBe(
      '<p dir="auto">a</p><p dir="auto">b</p><ul dir="auto"><li dir="auto">c</li><li dir="auto">d</li></ul>',
    );
    expect(inner('<p><a href="https://a.example/">a<a href="https://b.example/">b</a></a></p>')).toBe(
      '<p dir="auto"><a href="https://a.example/" rel="nofollow noopener noreferrer" target="_blank">a</a><a href="https://b.example/" rel="nofollow noopener noreferrer" target="_blank">b</a></p>',
    );
    expect(inner("<li>stray</li><rt>stray</rt>")).toBe('<p dir="auto">straystray</p>');
  });

  test("ol and li keep only integers", () => {
    expect(inner('<ol start="x" reversed="yes"><li value="1e9">a</li></ol>')).toBe(
      '<ol dir="auto" reversed=""><li dir="auto">a</li></ol>',
    );
  });
});

test("the parser keeps a line break after <pre>, which the server writes twice", () => {
  expect(parseContent("<pre>\n\nx</pre>")).toEqual([{ tag: "pre", attrs: {}, children: ["\nx"] }]);
});
