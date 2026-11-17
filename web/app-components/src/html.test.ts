// SPDX-License-Identifier: AGPL-3.0-only

// The allowlist of a post's text, as the parser reads the HTML the server rendered: what it keeps, with which
// attributes, and what it never lets through, which reads as nothing or as text. What an article's allowlist adds is in
// article-content.test.ts, and what is drawn from the parts in post-content.dom.test.tsx.

import { describe, expect, test } from "bun:test";
import { parseContent, type ContentElement, type ContentNode } from "./html";

/** An element of a text, written short. */
const el = (
  tag: ContentElement["tag"],
  attrs: ContentElement["attrs"],
  ...children: ContentNode[]
): ContentElement => ({
  tag,
  attrs,
  children,
});

/** Every element of `nodes`, at any depth, in the text's order. */
const elements = (nodes: ContentNode[]): ContentElement[] =>
  nodes.flatMap((node) => (typeof node === "string" ? [] : [node, ...elements(node.children)]));

/** The text of `nodes`, without its elements. */
const textOf = (nodes: ContentNode[]): string =>
  nodes.map((node) => (typeof node === "string" ? node : textOf(node.children))).join("");

describe("what the allowlist keeps", () => {
  test("a local post as the server renders it: paragraphs, a line break, a mention, a hashtag and a shortened URL", () => {
    // internal/app/content/text.go: ParsedText.HTML.
    const local =
      '<p>Hello <span class="h-card"><a href="https://my.example/@mika" class="u-url mention" rel="nofollow noopener noreferrer" target="_blank">@<span>mika</span></a></span>!<br>Line two</p>' +
      '<p><a href="https://my.example/tags/fediverse" class="mention hashtag" rel="nofollow noopener noreferrer" target="_blank">#<span>fediverse</span></a> ' +
      '<a href="https://example.jp/blog/2026/11/small-server-from-scratch" rel="nofollow noopener noreferrer" target="_blank"><span class="invisible">https://</span><span class="ellipsis">example.jp/blog/2026/11/small-</span><span class="invisible">server-from-scratch</span></a></p>';

    expect(parseContent(local)).toEqual([
      el(
        "p",
        {},
        "Hello ",
        el(
          "span",
          { class: "h-card" },
          el("a", { class: "u-url mention", href: "https://my.example/@mika" }, "@", el("span", {}, "mika")),
        ),
        "!",
        el("br", {}),
        "Line two",
      ),
      el(
        "p",
        {},
        el(
          "a",
          { class: "mention hashtag", href: "https://my.example/tags/fediverse" },
          "#",
          el("span", {}, "fediverse"),
        ),
        " ",
        el(
          "a",
          { href: "https://example.jp/blog/2026/11/small-server-from-scratch" },
          el("span", { class: "invisible" }, "https://"),
          el("span", { class: "ellipsis" }, "example.jp/blog/2026/11/small-"),
          el("span", { class: "invisible" }, "server-from-scratch"),
        ),
      ),
    ]);
  });

  test("a remote post as Mastodon writes it keeps that a mention is not to be translated", () => {
    const remote =
      '<p><span class="h-card" translate="no"><a href="https://mastodon.example/@alice" class="u-url mention">@<span>alice</span></a></span> look</p>';

    expect(parseContent(remote)).toEqual([
      el(
        "p",
        {},
        el(
          "span",
          { class: "h-card", translate: "no" },
          el("a", { class: "u-url mention", href: "https://mastodon.example/@alice" }, "@", el("span", {}, "alice")),
        ),
        " look",
      ),
    ]);
  });

  test("the inline elements, a list's numbering and a language are kept", () => {
    expect(parseContent('<ol start="3" reversed><li value="7">x</li></ol>')).toEqual([
      el("ol", { start: "3", reversed: "" }, el("li", { value: "7" }, "x")),
    ]);
    const [marked] = parseContent(
      '<p lang="ja"><del>a</del><s>b</s><code>c</code><b>d</b><strong>e</strong><u>f</u><i>g</i><em>h</em></p>',
    );
    expect(typeof marked !== "string" && marked?.attrs).toEqual({ lang: "ja" });
    expect(elements([marked ?? ""]).map((element) => element.tag)).toEqual([
      "p",
      "del",
      "s",
      "code",
      "b",
      "strong",
      "u",
      "i",
      "em",
    ]);
    expect(parseContent("<p><ruby>漢<rp>(</rp><rt>かん</rt><rp>)</rp></ruby></p>")).toEqual([
      el("p", {}, el("ruby", {}, "漢", el("rp", {}, "("), el("rt", {}, "かん"), el("rp", {}, ")"))),
    ]);
  });

  test("ol and li keep only integers", () => {
    expect(parseContent('<ol start="x" reversed="yes"><li value="1e9">a</li></ol>')).toEqual([
      el("ol", { reversed: "" }, el("li", {}, "a")),
    ]);
  });

  test("the parser keeps a line break after <pre>, which the server writes twice", () => {
    expect(parseContent("<pre>\n\nx</pre>")).toEqual([el("pre", {}, "\nx")]);
  });
});

describe("what the server would never let through reads as nothing or as text", () => {
  test("a script, a style, an iframe and the like go with their contents", () => {
    expect(
      parseContent(
        "<p>a<script>alert(1)</script>b<style>p{color:red}</style>c<iframe src=x>frame</iframe>d<svg><text>svg</text></svg>e<template><p>t</p></template>f</p>",
      ),
    ).toEqual([el("p", {}, "abcdef")]);
  });

  test("an end tag in a script's text does not end the script early, and an unclosed script takes the rest", () => {
    expect(parseContent("<p>a<script>document.write('<p>x</p>')</script>b</p>")).toEqual([el("p", {}, "ab")]);
    expect(parseContent("<p>a</p><script>alert(1)")).toEqual([el("p", {}, "a")]);
  });

  test("an image, a form control and other elements outside the allowlist are dropped or unwrapped to their text", () => {
    expect(parseContent('<p>a<img src=x onerror="alert(1)">b<input value=c>d</p>')).toEqual([el("p", {}, "abd")]);
    expect(parseContent("<div><h1>Title</h1><table><tr><td>cell</td></tr></table></div>")).toEqual(["Titlecell"]);
    expect(parseContent('<p><font color="red">red</font> <marquee>moves</marquee></p>')).toEqual([
      el("p", {}, "red moves"),
    ]);
  });

  test("event handlers, styles, ids, classes outside the server's, and a link's own target and rel are not kept", () => {
    expect(
      parseContent(
        '<p onclick="alert(1)" style="position:fixed" id="x" class="invisible">a <span onmouseover="alert(1)" style="color:red">b</span> <a href="https://e.example/" onclick="alert(1)" style="x" target="_self" rel="opener">c</a></p>',
      ),
    ).toEqual([el("p", {}, "a ", el("span", {}, "b"), " ", el("a", { href: "https://e.example/" }, "c"))]);
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
    ])
      expect([href, parseContent(`<p><a href="${href.replaceAll('"', "&quot;")}">go</a></p>`)]).toEqual([
        href,
        [el("p", {}, "go")],
      ]);
    expect(parseContent("<p><a>go</a></p>")).toEqual([el("p", {}, "go")]);
    expect(parseContent('<p><a href="gemini://g.example/">g</a></p>')).toEqual([
      el("p", {}, el("a", { href: "gemini://g.example/" }, "g")),
    ]);
  });

  test("character references read as text, and markup in them stays text", () => {
    expect(parseContent("<p>&lt;script&gt;alert(&#39;x&#39;)&lt;/script&gt; &amp; &#x1F600; &#0; &copy;</p>")).toEqual([
      el("p", {}, "<script>alert('x')</script> & 😀 � &copy;"),
    ]);
  });

  test("comments and bogus markup are dropped; a stray < is text", () => {
    expect(parseContent("<p>a<!-- <script>alert(1)</script> -->b<!doctype html>c<?xml x?>d</p>")).toEqual([
      el("p", {}, "abcd"),
    ]);
    expect(parseContent("<p>1 < 2 and 3 <4</p>")).toEqual([el("p", {}, "1 < 2 and 3 <4")]);
    expect(parseContent('<p>a<span title="unclosed>b</p>')).toEqual([el("p", {}, 'a<span title="unclosed>b')]);
  });

  test("blocks never nest in a paragraph, links never nest in a link, and list items stay in a list", () => {
    expect(parseContent("<p>a<p>b<ul><li>c<li>d</ul>")).toEqual([
      el("p", {}, "a"),
      el("p", {}, "b"),
      el("ul", {}, el("li", {}, "c"), el("li", {}, "d")),
    ]);
    expect(parseContent('<p><a href="https://a.example/">a<a href="https://b.example/">b</a></a></p>')).toEqual([
      el("p", {}, el("a", { href: "https://a.example/" }, "a"), el("a", { href: "https://b.example/" }, "b")),
    ]);
    expect(parseContent("<li>stray</li><rt>stray</rt>")).toEqual(["straystray"]);
  });

  test("a post's text never keeps what only an article may hold: headings and cells are their text, a rule and an image nothing", () => {
    const post = parseContent('<h2>Title</h2><hr><p>a<img src="https://my.example/a.png" alt="pic">b</p>');
    expect(elements(post).map((element) => element.tag)).toEqual(["p"]);
    expect(textOf(post)).toBe("Titleab");
    expect(
      parseContent('<table><thead><tr><th align="right">h</th></tr></thead><tbody><tr><td>c</td></tr></tbody></table>'),
    ).toEqual(["hc"]);
    expect(parseContent("<h3>x</h3><hr>")).toEqual(["x"]);
  });
});
