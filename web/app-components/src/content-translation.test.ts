// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { translateContent, translatePlain, type TranslateText } from "./content-translation";

/** A translator as the server's fake provider is: the target language before each text, and what it was sent. */
function translator(answer: (text: string) => string = (text) => `[en] ${text}`) {
  const sent: string[] = [];
  const translate: TranslateText = (text) => {
    sent.push(text);
    return Promise.resolve(answer(text));
  };
  return { sent, translate };
}

const mention =
  '<span class="h-card" translate="no"><a href="https://my.example/@lena" class="u-url mention">@<span>lena</span></a></span>';
const hashtag = '<a href="https://my.example/tags/bread" class="mention hashtag" rel="tag">#<span>bread</span></a>';
const link =
  '<a href="https://bakery.example/menu" target="_blank" rel="nofollow noopener" translate="no"><span class="invisible">https://</span><span class="">bakery.example/menu</span></a>';

/** The text of `html` without its tags. */
const textOf = (html: string) => html.replace(/<[^>]*>/g, "");

describe("the text of a post", () => {
  test("each block is a fragment of its own, translated in place", async () => {
    const { sent, translate } = translator();
    const html = await translateContent("<p>おはよう</p><p>パンを焼いた</p>", translate);
    expect(sent).toEqual(["おはよう", "パンを焼いた"]);
    expect(html).toBe("<p>[en] おはよう</p><p>[en] パンを焼いた</p>");
  });

  test("a list's items are fragments of their own, the list kept", async () => {
    const { sent, translate } = translator();
    const html = await translateContent("<ul><li>ライ麦</li><li>サワー種</li></ul>", translate);
    expect(sent).toEqual(["ライ麦", "サワー種"]);
    expect(html).toBe("<ul><li>[en] ライ麦</li><li>[en] サワー種</li></ul>");
  });

  test("a mention, a hashtag and a link are never sent, and come back as they were", async () => {
    const { sent, translate } = translator();
    const html = await translateContent(`<p>${mention} さん、${hashtag} の店は ${link} です</p>`, translate);
    expect(sent).toHaveLength(1);
    expect(sent[0]).not.toContain("lena");
    expect(sent[0]).not.toContain("bread");
    expect(sent[0]).not.toContain("bakery");
    expect(sent[0]).not.toContain("https");
    expect(html).toContain('href="https://my.example/@lena"');
    expect(html).toContain('href="https://my.example/tags/bread"');
    expect(html).toContain('href="https://bakery.example/menu"');
    expect(textOf(html)).toMatch(/^\[en\] @lena さん、#bread の店は https:\/\/bakery\.example\/menu です$/);
  });

  test("code is never sent, and comes back as it was", async () => {
    const { sent, translate } = translator();
    const html = await translateContent("<p>こう書く</p><pre><code>let 名前 = 1;</code></pre>", translate);
    expect(sent).toEqual(["こう書く"]);
    expect(html).toContain("<pre><code>let 名前 = 1;</code></pre>");
  });

  test("decoration around words stays around their translation", async () => {
    const { translate } = translator();
    const html = await translateContent("<p>とても<strong>大事</strong>です</p>", translate);
    expect(html).toMatch(/^<p>\[en\] とても<strong>大事<\/strong>です<\/p>$/);
  });

  test("a block with no words to translate (a mention alone) is not sent", async () => {
    const { sent, translate } = translator();
    const html = await translateContent(`<p>${mention}</p><p>ありがとう</p>`, translate);
    expect(sent).toEqual(["ありがとう"]);
    expect(html).toContain('href="https://my.example/@lena"');
    expect(html).toContain("<p>[en] ありがとう</p>");
  });

  test("markup in the translator's output stays text: it is escaped, never parsed", async () => {
    const { translate } = translator(() => '<img src="x" onerror="alert(1)"><script>alert(2)</script>');
    const html = await translateContent("<p>こんにちは</p>", translate);
    expect(html).not.toContain("<img");
    expect(html).not.toContain("<script");
    expect(html).toMatch(/^<p>&lt;img src=("|&quot;)x("|&quot;) onerror=/);
    expect(html).toMatch(/&lt;script&gt;alert\(2\)&lt;\/script&gt;<\/p>$/);
  });

  test("a translator that loses the kept parts still leaves them in place, the words between them translated", async () => {
    // Keeps letters and spaces only: whatever stands for the mention and the link is lost.
    const { translate } = translator((text) => `[en] ${text.replace(/[^\p{L}\p{Zs}]/gu, "")}`);
    const html = await translateContent(`<p>${mention} さん ${link} を見て</p>`, translate);
    expect(html).toContain('href="https://my.example/@lena"');
    expect(html).toContain('href="https://bakery.example/menu"');
    expect(textOf(html)).toContain("@lena");
    expect(textOf(html)).toContain("https://bakery.example/menu");
    expect(textOf(html)).toContain("[en]");
    expect(textOf(html).indexOf("@lena")).toBeLessThan(textOf(html).indexOf("https://bakery.example/menu"));
  });

  test("it rejects when the translator does", async () => {
    const failing: TranslateText = () => Promise.reject(new Error("no model"));
    const done = await translateContent("<p>こんにちは</p>", failing).then(
      () => "resolved",
      () => "rejected",
    );
    expect(done).toBe("rejected");
  });
});

describe("plain text (a content warning, a poll's option, an alt text)", () => {
  test("is translated whole", async () => {
    const { sent, translate } = translator();
    expect(await translatePlain("食べ物の写真", translate)).toBe("[en] 食べ物の写真");
    expect(sent).toEqual(["食べ物の写真"]);
  });

  test("markup in it or in the output is only text", async () => {
    const { translate } = translator();
    expect(await translatePlain("<b>太字</b>", translate)).toBe("[en] <b>太字</b>");
  });

  test("an address, a handle and a custom emoji's shortcode are never sent, and come back as they were", async () => {
    const { sent, translate } = translator();
    const out = await translatePlain("見て https://bakery.example/menu @lena :blobcat:", translate);
    expect(sent.join(" ")).not.toContain("bakery");
    expect(sent.join(" ")).not.toContain("lena");
    expect(sent.join(" ")).not.toContain("blobcat");
    expect(out).toContain("https://bakery.example/menu");
    expect(out).toContain("@lena");
    expect(out).toContain(":blobcat:");
    expect(out).toContain("[en]");
  });

  test("text with no words is not sent", async () => {
    const { sent, translate } = translator();
    expect(await translatePlain("🍞 🥐", translate)).toBe("🍞 🥐");
    expect(sent).toEqual([]);
  });
});
