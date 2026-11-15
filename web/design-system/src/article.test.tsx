// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import type { ReactNode } from "react";
import { I18nProvider } from "react-aria-components";
import { renderToStaticMarkup } from "react-dom/server";
import { ArticleCard, type ArticleCardProps, ArticleTable, ArticleText, ArticleTitle } from "./article";

/** The markup without classes and glyphs, to read the structure. */
const bare = (node: ReactNode, locale = "en") =>
  renderToStaticMarkup(<I18nProvider locale={locale}>{node}</I18nProvider>)
    .replace(/ class="[^"]*"/g, "")
    .replace(/<svg .*?<\/svg>/g, "")
    .replace(/<link [^>]*\/>/g, "");

/** The text a screen reader reads for each link: its text, without the tags. */
const links = (html: string) =>
  [...html.matchAll(/<a [^>]*>(.*?)<\/a>/g)].map(([, inner]) =>
    (inner ?? "")
      .replace(/<[^>]+>/g, " ")
      .replace(/\s+/g, " ")
      .trim(),
  );

const card: ArticleCardProps = {
  href: "/@sam/status/10",
  title: "A sourdough starter in seven days",
  summary: "A diary of a jar of flour.",
  cover: {
    src: "https://my.example/media/loaf.webp",
    width: 640,
    height: 400,
    alt: "A round loaf on a wooden board",
    blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj",
  },
  lang: "en",
};

describe("the article's card", () => {
  test("one link to the article's page in the app, which says what it is, its title and its summary", () => {
    const html = bare(<ArticleCard {...card} />);
    expect(html.match(/<a /g)).toHaveLength(1);
    expect(html).toMatch(/<a [^>]*href="\/@sam\/status\/10"/);
    expect(html).not.toContain('target="_blank"');
    expect(links(html)).toEqual(["Article A sourdough starter in seven days A diary of a jar of flour."]);
    expect(links(bare(<ArticleCard {...card} />, "ja"))).toEqual([
      "記事 A sourdough starter in seven days A diary of a jar of flour.",
    ]);
  });

  test("its cover under them, with its description", () => {
    const html = bare(<ArticleCard {...card} />);
    expect(html).toContain('src="https://my.example/media/loaf.webp"');
    expect(html).toContain('alt="A round loaf on a wooden board"');
    expect(html.indexOf("loaf.webp")).toBeGreaterThan(html.indexOf("A diary of a jar of flour."));
  });

  test("without a summary or a cover, its title alone", () => {
    const html = bare(<ArticleCard {...card} summary="" cover={undefined} />);
    expect(links(html)).toEqual(["Article A sourdough starter in seven days"]);
    expect(html).not.toContain("<img");
  });

  test("sensitive, its cover is covered and its file not asked for; its title and summary stay", () => {
    const html = bare(<ArticleCard {...card} sensitive />);
    expect(html).not.toContain("loaf.webp");
    expect(html).toContain("<canvas");
    expect(links(html)).toEqual(["Article A sourdough starter in seven days A diary of a jar of flour."]);
  });

  test("the article's language marks its title and summary", () => {
    expect(bare(<ArticleCard {...card} lang="ja" />)).toMatch(/lang="ja"/);
  });
});

describe("the article's page", () => {
  test("its title is the page's h1, over its summary, each in its own direction", () => {
    expect(bare(<ArticleTitle title="Title" summary="Summary" lang="en" />)).toBe(
      '<hgroup lang="en"><h1 dir="auto">Title</h1><p dir="auto">Summary</p></hgroup>',
    );
  });

  test("an empty summary is no paragraph", () => {
    expect(bare(<ArticleTitle title="Title" summary="" />)).toBe('<hgroup><h1 dir="auto">Title</h1></hgroup>');
  });

  test("its text holds its language and reads in its own direction", () => {
    expect(bare(<ArticleText lang="ja">x</ArticleText>)).toBe('<div lang="ja" dir="auto">x</div>');
  });

  test("a table in a box a keyboard can scroll, its columns in its own direction, in its language", () => {
    expect(
      bare(
        <ArticleTable lang="ar">
          <tbody>
            <tr>
              <td>x</td>
            </tr>
          </tbody>
        </ArticleTable>,
      ),
    ).toBe('<div tabindex="0"><table lang="ar" dir="auto"><tbody><tr><td>x</td></tr></tbody></table></div>');
  });
});
