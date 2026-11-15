// SPDX-License-Identifier: AGPL-3.0-only

// An article as a post draws it (ADR-0160 §8, #3089): its card in a timeline, and in full on its page, with the post's
// author line and actions either way; and a reply under it on its page, without the line of whom it replies to.

import { describe, expect, test } from "bun:test";
import { I18nProvider } from "@my/design-system";
import { renderToStaticMarkup } from "react-dom/server";
import { Post, type PostProps } from "./post";
import { PostContent } from "./post-content";
import type { PostAccount, PostArticle, PostStatus } from "./status";

const sam: PostAccount = {
  id: "1",
  username: "sam",
  acct: "sam",
  domain: "my.example",
  display_name: "Sam Rivera",
  emojis: [],
};

const alice: PostAccount = {
  id: "2",
  username: "alice",
  acct: "alice@example.social",
  domain: "example.social",
  display_name: "Alice Chen",
  emojis: [],
};

const starter: PostArticle = {
  title: "A sourdough starter in seven days",
  summary: "What I fed it, when it rose, and the three mistakes that cost me a week.",
  cover: {
    id: "c",
    type: "image",
    url: "https://my.example/media/loaf.webp",
    preview_url: "https://my.example/media/loaf-small.webp",
    description: "A round loaf on a wooden board",
    meta: { width: 640, height: 400 },
    blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj",
  },
};

/** An article as a list gives it: its title, summary and cover, and an empty text. */
const status = (over: Partial<PostStatus> = {}): PostStatus => ({
  id: "10",
  account: sam,
  content: "",
  spoiler_text: "",
  visibility: "public",
  language: "en",
  created_at: new Date(Date.now() - 3 * 60_000).toISOString(),
  media_attachments: [],
  mentions: [],
  emojis: [],
  replies_count: 2,
  boosts_count: 3,
  favourites_count: 4,
  reactions: [],
  article: starter,
  ...over,
});

const body =
  '<h2>The first three days</h2><p>Nothing, then bubbles.</p><table><tbody><tr><td align="right">50 g</td></tr></tbody></table>';

const reading = <PostContent html={body} article />;

const render = (props: PostProps, locale = "en") =>
  renderToStaticMarkup(
    <I18nProvider locale={locale}>
      <Post {...props} />
    </I18nProvider>,
  );

/** The markup without classes and glyphs, to read the structure. */
const bare = (props: PostProps, locale?: string) =>
  render(props, locale)
    .replace(/ class="[^"]*"/g, "")
    .replace(/<svg .*?<\/svg>/g, "");

/** The links of the markup whose href is `href`, each with its text and nothing else. */
const linksTo = (html: string, href: string) =>
  [...html.matchAll(/<a ([^>]*)>(.*?)<\/a>/g)]
    .filter(([, attrs]) => attrs?.includes(`href="${href}"`))
    .map(([, , inner]) =>
      (inner ?? "")
        .replace(/<[^>]+>/g, " ")
        .replace(/\s+/g, " ")
        .trim(),
    );

describe("an article in a timeline", () => {
  test("its card is one link to its page, saying what it is, with its title and summary", () => {
    const html = bare({ status: status() });
    const card = linksTo(html, "/@sam/status/10").filter((text) => text.includes(starter.title));
    expect(card).toEqual([`Article ${starter.title} ${starter.summary}`]);
  });

  test("its cover shows with its description, after its title", () => {
    const html = bare({ status: status() });
    expect(html).toMatch(/<img [^>]*alt="A round loaf on a wooden board"/);
    expect(html.indexOf("A round loaf on a wooden board")).toBeGreaterThan(html.indexOf(starter.title));
  });

  test("it keeps the post's author line and actions", () => {
    const html = bare({ status: status() });
    expect(html).toContain("Sam Rivera</bdi>");
    expect(html).toMatch(/<a [^>]*href="\/@sam\/status\/10"[^>]*><bdi><time /);
    for (const action of ["Reply", "Favourite", "More"]) expect(html).toMatch(new RegExp(`aria-label="${action}[",]`));
  });

  test("in Japanese, it says it is an article in Japanese", () => {
    const card = linksTo(bare({ status: status() }, "ja"), "/@sam/status/10").filter((t) => t.includes(starter.title));
    expect(card).toEqual([`記事 ${starter.title} ${starter.summary}`]);
  });

  test("without a summary or a cover, the card is its title alone", () => {
    const html = bare({ status: status({ article: { title: starter.title, summary: "" } }) });
    expect(linksTo(html, "/@sam/status/10").filter((t) => t.includes(starter.title))).toEqual([
      `Article ${starter.title}`,
    ]);
    expect(html).not.toContain("<img");
  });

  test("a sensitive article covers its cover, and its title and summary stay shown", () => {
    const html = bare({ status: status({ sensitive: true }) });
    expect(html).not.toContain("loaf.webp");
    expect(html).toContain(starter.title);
    expect(html).toContain(starter.summary);
  });

  test("a boosted article from another server links to its page there, under who boosted it", () => {
    const boosted = status({ id: "20", account: alice });
    const html = bare({ status: status({ id: "30", article: undefined, boost: boosted }) });
    expect(html).toContain("Sam Rivera</bdi> boosted");
    const card = linksTo(html, "/@alice@example.social/status/20").filter((t) => t.includes(starter.title));
    expect(card).toEqual([`Article ${starter.title} ${starter.summary}`]);
  });

  test("the card is not its text: the reader is not shown an empty body nor the full text", () => {
    const html = bare({ status: status() });
    expect(html).not.toContain("<h1");
    expect(html).not.toContain("The first three days");
  });
});

describe("an article on its page", () => {
  test("its title is the heading over its summary, then its cover, then its text", () => {
    const html = bare({ status: status(), reading });
    expect(html).toMatch(/<h1 dir="auto">A sourdough starter in seven days<\/h1>/);
    const at = (s: string) => html.indexOf(s);
    expect(at(starter.title)).toBeLessThan(at(starter.summary));
    expect(at(starter.summary)).toBeLessThan(at("A round loaf on a wooden board"));
    expect(at("A round loaf on a wooden board")).toBeLessThan(at("The first three days"));
  });

  test("its text is the full text, its headings and its table, and no card leads away from it", () => {
    const html = bare({ status: status(), reading });
    expect(html).toContain('<h2 dir="auto">The first three days</h2>');
    expect(html).toContain('<td align="right">50 g</td>');
    expect(linksTo(html, "/@sam/status/10").filter((t) => t.includes(starter.title))).toEqual([]);
  });

  test("it keeps the post's author line and actions", () => {
    const html = bare({ status: status(), reading });
    expect(html).toContain("Sam Rivera</bdi>");
    for (const action of ["Reply", "Favourite", "More"]) expect(html).toMatch(new RegExp(`aria-label="${action}[",]`));
  });

  test("its text reads in its language", () => {
    const html = bare({ status: status({ language: "ja" }), reading });
    expect(html).toMatch(/<div lang="ja" dir="auto"><h2 dir="auto">The first three days/);
  });

  test("a sensitive article folds its cover and text under a line with Show; its title and summary stay", () => {
    const html = bare({ status: status({ sensitive: true }), reading });
    expect(html).toContain(starter.title);
    expect(html).toContain(starter.summary);
    expect(html).toContain("Sensitive article");
    expect(html).toMatch(/<button [^>]*aria-expanded="false"/);
    // Folded, the text is not drawn at all: none of its images may be asked for before Show.
    expect(html).not.toContain("The first three days");
    expect(bare({ status: status({ sensitive: true }), reading }, "ja")).toContain("センシティブな記事");
  });

  test("a sensitive article's cover is not asked for before Show, as no sensitive media's file is", () => {
    // As in its card, and as a sensitive post's media (post.test.tsx): the blurhash in its place, its file not asked for.
    const html = bare({ status: status({ sensitive: true }), reading });
    expect(html).not.toContain("loaf.webp");
  });

  test("without a summary, the heading stands alone", () => {
    const html = bare({ status: status({ article: { title: starter.title, summary: "" } }), reading });
    expect(html).toMatch(/<hgroup[^>]*><h1 dir="auto">A sourdough starter in seven days<\/h1><\/hgroup>/);
  });
});

describe("a sensitive article's page before Show", () => {
  // A text with an image of its own, which must not be asked for while the article is folded.
  const withImage =
    '<h2>The first three days</h2><p>Nothing, then bubbles.</p><p><img src="https://my.example/media/large/bubbles.webp" alt="Bubbles in a jar" width="800" height="600" loading="eager"></p><table><tbody><tr><td>50 g</td></tr></tbody></table>';
  const page = (sensitive: boolean) =>
    render({ status: status({ sensitive }), reading: <PostContent html={withImage} article /> });

  test("its head, its line and its Show are there", () => {
    const html = page(true);
    expect(html).toMatch(/<h1 [^>]*>A sourdough starter in seven days<\/h1>/);
    expect(html).toContain(starter.summary);
    expect(html).toContain("Sensitive article");
    expect(html).toMatch(/<button [^>]*aria-expanded="false"[^>]*>(?:(?!<\/button>).)*Show/);
  });

  test("its cover is only its blurhash: no image and no preload of it", () => {
    const html = page(true);
    expect(html).toContain("<canvas");
    expect(html).not.toContain("<img");
    expect(html).not.toContain('rel="preload"');
    expect(html).not.toContain("loaf");
  });

  test("none of its text is in the markup, not even hidden: no heading, paragraph, table or image of it", () => {
    const html = page(true);
    for (const part of ["The first three days", "Nothing, then bubbles.", "bubbles.webp", "Bubbles in a jar", "50 g"])
      expect(html).not.toContain(part);
    expect(html).not.toMatch(/<h[2-6][ >]/);
    expect(html).not.toContain("<table");
    // The only paragraphs are the summary's and the post's own lines (none of the text's).
    expect(html.match(/<p [^>]*dir="auto"[^>]*>/g) ?? []).toHaveLength(1);
  });

  test("a non-sensitive article draws its cover and its whole text, its image included", () => {
    const html = page(false);
    expect(html).toMatch(/<img [^>]*src="https:\/\/my.example\/media\/loaf.webp"/);
    expect(html).toMatch(/<h2 [^>]*>The first three days<\/h2>/);
    expect(html).toContain("Nothing, then bubbles.");
    expect(html).toMatch(/<img [^>]*src="https:\/\/my.example\/media\/large\/bubbles.webp"[^>]*alt="Bubbles in a jar"/);
    expect(html).toContain("<table");
    expect(html).not.toContain("Sensitive article");
  });
});

describe("a reply under the article", () => {
  const reply = status({
    id: "11",
    account: alice,
    content: "<p>@sam Same smell here on day three.</p>",
    article: undefined,
    in_reply_to_account_id: "1",
    mentions: [{ id: "1", username: "sam", acct: "sam" }],
  });

  test("names whom it replies to, as any reply does", () => {
    expect(bare({ status: reply })).toContain('Replying to <bdi dir="ltr">@sam</bdi>');
  });

  test("drops that line where the screen shows it already", () => {
    const html = bare({ status: reply, replyingTo: false });
    expect(html).not.toContain("Replying to");
    expect(html).toContain("Same smell here on day three.");
  });
});
