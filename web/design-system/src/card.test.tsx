// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import type { ReactNode } from "react";
import { I18nProvider } from "react-aria-components";
import { renderToStaticMarkup } from "react-dom/server";
import { LinkCard, type LinkCardProps } from "./card";

/** The markup without classes and glyphs, to read the structure. */
const bare = (node: ReactNode, locale = "en") =>
  renderToStaticMarkup(<I18nProvider locale={locale}>{node}</I18nProvider>)
    .replace(/ class="[^"]*"/g, "")
    .replace(/<svg .*?<\/svg>/g, "");

const page: LinkCardProps = {
  href: "https://blog.example/posts/rye",
  title: "Baking rye at home",
  description: "What three weeks of starters taught me.",
  site: "Field Notes",
  image: {
    src: "https://my.example/proxy/rye.webp",
    width: 1200,
    height: 600,
    alt: "A dark loaf on a board",
    blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj",
  },
  author: { shown: <b>Ann Lee</b>, name: "Ann Lee" },
};

/** The text a screen reader reads for the link: its text, without what is hidden from it. */
const linkText = (html: string) =>
  /<a [^>]*>(.*)<\/a>/
    .exec(html)?.[1]
    ?.replace(/<span aria-hidden="true">.*?<\/b><\/span>/g, "")
    .replace(/<[^>]+>/g, " ")
    .replace(/\s+/g, " ")
    .trim();

describe("the link card", () => {
  test("one link to the page, in a new tab, that never passes on rank or the reader's page", () => {
    const html = bare(<LinkCard {...page} />);
    expect(html.match(/<a /g)).toHaveLength(1);
    expect(html).toMatch(
      /<a [^>]*href="https:\/\/blog.example\/posts\/rye" target="_blank" rel="noopener noreferrer nofollow"/,
    );
  });

  test("named by its visible text: the site, the title, the description, by the author, and that it opens a new tab", () => {
    expect(linkText(bare(<LinkCard {...page} />))).toBe(
      "Field Notes Baking rye at home What three weeks of starters taught me. by Ann Lee (opens in a new tab)",
    );
  });

  test("the author shows as given and reads as by and the name", () => {
    const html = bare(<LinkCard {...page} />);
    expect(html).toContain('<span aria-hidden="true"><b>Ann Lee</b></span><span>by Ann Lee</span>');
    expect(bare(<LinkCard {...page} />, "ja")).toContain("<span>著者: Ann Lee</span>");
    expect(bare(<LinkCard {...page} author={undefined} />)).not.toContain("by ");
  });

  test("the image comes through the media proxy with its own description", () => {
    const html = bare(<LinkCard {...page} />);
    expect(html).toContain('<img src="https://my.example/proxy/rye.webp" alt="A dark loaf on a board"/>');
    const { alt: _, ...undescribed } = page.image ?? { src: "", width: 0, height: 0 };
    expect(bare(<LinkCard {...page} image={undescribed} />)).toContain(
      '<img src="https://my.example/proxy/rye.webp" alt=""/>',
    );
  });

  test("small or large, and without an image or a description, it is the same link", () => {
    for (const card of [
      <LinkCard key="s" {...page} size="small" />,
      <LinkCard key="t" {...page} image={undefined} description="" />,
    ]) {
      const html = bare(card);
      expect(html).toMatch(/<a [^>]*href="https:\/\/blog.example\/posts\/rye"/);
      expect(html).toContain("Baking rye at home");
    }
    expect(bare(<LinkCard {...page} image={undefined} />)).not.toContain("<img");
    expect(linkText(bare(<LinkCard {...page} description="" author={undefined} />))).toBe(
      "Field Notes Baking rye at home (opens in a new tab)",
    );
  });

  test("the language of the page marks its text", () => {
    expect(bare(<LinkCard {...page} lang="de" />)).toMatch(/lang="de"/);
  });
});

describe("a sensitive card", () => {
  test("its image is covered: the blurhash in its place, and no image asked for", () => {
    const html = bare(<LinkCard {...page} sensitive />);
    expect(html).toContain('<canvas width="32" height="32" aria-hidden="true"></canvas>');
    expect(html).not.toContain("<img");
    expect(html).not.toContain("proxy/rye.webp");
  });

  test("the line that shows it is outside the link, before it", () => {
    const html = bare(<LinkCard {...page} sensitive />);
    expect(html).toMatch(
      /^<div[^>]*><div[^>]*>.*?Sensitive image<\/span><button [^>]*aria-expanded="false"[^>]*>.*?<span lang="en">Show<\/span><\/span><\/button><\/div><a /,
    );
    expect(bare(<LinkCard {...page} sensitive />, "ja")).toMatch(
      /センシティブな画像<\/span><button [^>]*>.*?<span lang="ja">表示<\/span>/,
    );
  });

  test("a sensitive card without an image has nothing to cover", () => {
    const html = bare(<LinkCard {...page} image={undefined} sensitive />);
    expect(html).not.toContain("Sensitive");
    expect(html).toMatch(/^<a /);
  });
});
