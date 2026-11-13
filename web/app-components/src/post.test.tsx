// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { I18nProvider } from "@my/design-system";
import { renderToStaticMarkup } from "react-dom/server";
import { Post, type PostProps } from "./post";
import type { PostAccount, PostStatus, PostTranslation } from "./status";

const lena: PostAccount = {
  id: "1",
  username: "lena",
  acct: "lena@toot.example.eu",
  domain: "toot.example.eu",
  display_name: "Lena Novak",
  emojis: [],
};

const ken: PostAccount = {
  id: "2",
  username: "kenji",
  acct: "kenji",
  domain: "my.example",
  display_name: "Kenji :ramen:",
  emojis: [{ shortcode: "ramen", url: "https://my.example/ramen.gif", static_url: "https://my.example/ramen.png" }],
  avatar: { small: { url: "https://my.example/kenji.webp" } },
};

const status = (over: Partial<PostStatus> = {}): PostStatus => ({
  id: "10",
  account: lena,
  content: "<p>Quiet morning.</p>",
  spoiler_text: "",
  visibility: "public",
  language: "en",
  created_at: new Date(Date.now() - 3 * 60_000).toISOString(),
  media_attachments: [],
  mentions: [],
  emojis: [],
  replies_count: 2,
  boosts_count: 1_234,
  favourites_count: 12_840,
  reactions: [],
  ...over,
});

const render = (props: PostProps, locale = "en") =>
  renderToStaticMarkup(
    <I18nProvider locale={locale}>
      <Post {...props} />
    </I18nProvider>,
  );

/** The markup without classes, to read the structure. */
const bare = (props: PostProps, locale?: string) => render(props, locale).replace(/ class="[^"]*"/g, "");

describe("the author header", () => {
  test("the avatar, the name isolated, then the visibility and the time at the end, over the handle", () => {
    const html = bare({ status: status() });
    const name = /<bdi id="([^"]+)">Lena Novak<\/bdi>/.exec(html);
    expect(name).not.toBeNull();
    expect(html).toMatch(new RegExp(`^<article lang="en" dir="ltr" tabindex="0" aria-labelledby="${name?.[1] ?? ""}"`));
    expect(html).toMatch(/Lena Novak<\/bdi><span><span><span role="img" aria-label="Public"><svg /);
    expect(render({ status: status() })).toContain("tabler-icon-world");
    expect(html).toContain("<bdi><time ");
    expect(html).toContain('<bdi dir="ltr">@lena@toot.example.eu</bdi>');
    expect(html.indexOf("3m ago")).toBeLessThan(html.indexOf("@lena@toot.example.eu"));
  });

  test("a local account's handle names its domain too, and a name keeps its custom emoji", () => {
    const html = bare({ status: status({ account: ken }) });
    expect(html).toContain('<bdi dir="ltr">@kenji@my.example</bdi>');
    expect(html).toMatch(/>Kenji <picture>.*alt=":ramen:"/);
    expect(html).toContain('src="https://my.example/kenji.webp"');
  });

  test("each visibility has its glyph and its name", () => {
    const glyphs = { public: "world", local: "building-community", unlisted: "moon", private: "lock", direct: "at" };
    for (const [visibility, glyph] of Object.entries(glyphs) as [PostStatus["visibility"], string][]) {
      expect(render({ status: status({ visibility }) })).toContain(`tabler-icon-${glyph} `);
    }
    expect(bare({ status: status({ visibility: "private" }) })).toContain('aria-label="Followers only"');
  });
});

describe("the context line", () => {
  test("a boost names who boosted it, and shows the boosted post", () => {
    const boost = status({ id: "20", account: ken, boost: status() });
    const html = bare({ status: boost });
    expect(html).toMatch(/^<article [^>]*><p lang="en"><span><svg [^>]*>.*?<\/svg><\/span><span><bdi>Kenji <picture>/);
    expect(render({ status: boost })).toMatch(/^<article [^>]*><p [^>]*><span [^>]*><svg [^>]*tabler-icon-repeat/);
    expect(html).toContain("</picture></bdi> boosted</span></p>");
    expect(html).toContain("Lena Novak</bdi>");
    expect(html).toContain("Quiet morning.");
  });

  test("the boost line stays one line: the booster's name truncates, the verb (with ja's particle) stays", () => {
    const boost = status({ id: "20", account: ken, boost: status() });
    const html = render({ status: boost });
    expect(html).toMatch(/<span class="[^"]*flex whitespace-pre[^"]*"><bdi class="min-w-0 truncate">Kenji /);
    expect(render({ status: boost }, "ja")).toMatch(/<\/bdi>さんがブースト<\/span><\/p>/);
  });

  test("a reply names whom it replies to, its handle left-to-right", () => {
    const reply = status({
      in_reply_to_account_id: "3",
      mentions: [{ id: "3", username: "mika", acct: "mika@mstdn.example.jp" }],
    });
    expect(bare({ status: reply })).toContain('<span>Replying to <bdi dir="ltr">@mika</bdi></span>');
    expect(bare({ status: reply }, "ja")).toContain('<bdi dir="ltr">@mika</bdi>さんへの返信');
  });

  test("a post that is neither has none", () => {
    expect(bare({ status: status() })).toMatch(/^<article [^>]*><div>/);
    expect(bare({ status: status({ in_reply_to_account_id: "9" }) })).not.toContain("Replying");
  });
});

describe("the body", () => {
  test("a content warning folds the text and the images, with the eye-off glyph", () => {
    const warned = status({
      spoiler_text: "Food, close up",
      media_attachments: [{ id: "m", type: "image", description: "A bowl of soup", preview_url: "https://m/s.webp" }],
    });
    const html = bare({ status: warned });
    expect(render({ status: warned })).toContain("tabler-icon-eye-off");
    expect(html).toMatch(/>Food, close up<\/div><button [^>]*aria-expanded="false"/);
    expect(html).toMatch(/ hidden="">.*Quiet morning\..*alt="A bowl of soup"/);
  });

  test("the images draw at the full width with their descriptions", () => {
    const html = bare({
      status: status({
        media_attachments: [
          {
            id: "a",
            type: "image",
            description: "A ridge",
            preview_url: "https://m/a-small.webp",
            url: "https://m/a.webp",
          },
          { id: "b", type: "image", url: "https://m/b.webp" },
          { id: "c", type: "audio" },
        ],
      }),
    });
    expect(html).toContain('src="https://m/a-small.webp" alt="A ridge"');
    expect(html).toContain('src="https://m/b.webp" alt=""');
    expect(html.match(/<img /g)).toHaveLength(2);
  });

  test("a quoted post is embedded one level deep, its images as small previews", () => {
    const quoted = status({
      id: "30",
      account: ken,
      language: "ja",
      content: "<p>小さなサーバー</p>",
      media_attachments: [{ id: "q", type: "image", description: "森", preview_url: "https://m/q.webp" }],
    });
    const html = render({ status: status({ quote: { state: "accepted", quoted_status: quoted } }) });
    expect(html).toMatch(/<div class="[^"]*bg-surface-raised[^"]*" lang="ja">/);
    expect(html).toContain("小さなサーバー");
    expect(html).toContain("h-media-preview-height");
    expect(html).toContain("size-avatar-inline");
    expect(render({ status: status({ quote: { state: "deleted" } }) })).not.toContain("size-avatar-inline");
  });
});

describe("the reactions", () => {
  const reacted = status({
    reactions: [
      { name: "🎉", count: 5 },
      {
        name: "blobcat",
        shortcode: "blobcat",
        emoji: { shortcode: "blobcat", url: "https://m/b.gif", static_url: "https://m/b.png" },
        count: 8,
        me: true,
      },
    ],
  });

  test("a chip per emoji with its count, the reader's own on, in a named group", () => {
    const html = bare({ status: reacted });
    expect(html).toContain('<div role="group" aria-label="Reactions" lang="en">');
    expect(html).toContain('aria-label="🎉, 5" aria-pressed="false"');
    expect(html).toContain('aria-label=":blobcat:, 8, including yours" aria-pressed="true"');
    expect(html).toContain('<bdi aria-hidden="true" dir="ltr" lang="en">8</bdi>');
  });

  test("they read in the reader's locale", () => {
    expect(bare({ status: reacted }, "ja")).toContain('aria-label="🎉、5件"');
  });

  test("none draws no group", () => {
    expect(bare({ status: status() })).not.toContain('role="group"');
  });
});

describe("the translate line", () => {
  const translation: PostTranslation = {
    language: "en",
    source_language: "ja",
    content: "<p>A small server.</p>",
    spoiler_text: "",
    media_attachments: [],
    provider: { name: "LibreTranslate" },
  };
  const ja = status({ language: "ja", content: "<p>小さなサーバー</p>" });

  test("a post that may be translated offers to", () => {
    const html = bare({ status: ja, translatable: true });
    expect(render({ status: ja, translatable: true })).toContain("tabler-icon-language");
    expect(html).toMatch(/<button [^>]*>Translate<\/button>/);
    expect(render({ status: ja })).not.toContain("tabler-icon-language");
  });

  test("translated, it shows the translation, names the language and the provider, and offers the original", () => {
    const html = bare({ status: ja, translatable: true, translation });
    expect(html).toContain("A small server.");
    expect(html).not.toContain("小さなサーバー");
    expect(html).toContain('<div lang="en"><p dir="auto">A small server.</p></div>');
    expect(html).toMatch(
      /Translated from <bdi>Japanese<\/bdi> by <bdi>LibreTranslate<\/bdi> <span>· <button [^>]*>Show original<\/button><\/span>/,
    );
  });

  test("the note says what it knows, in the reader's locale", () => {
    const unknown = { ...translation, source_language: "und" };
    expect(bare({ status: ja, translation: unknown })).toContain("Translated by <bdi>LibreTranslate</bdi> ");
    const { provider: _, ...nameless } = translation;
    expect(bare({ status: ja, translation: nameless })).toContain("Translated from <bdi>Japanese</bdi> ");
    expect(
      bare({ status: status(), translation: { ...translation, language: "ja", source_language: "en" } }, "ja"),
    ).toContain("<bdi>LibreTranslate</bdi> が<bdi>英語</bdi>から翻訳");
  });

  test("a pseudo-locale transforms the note's words, never the provider's name", () => {
    const html = bare({ status: ja, translation }, "ar-XB");
    expect(html).toContain("<bdi>LibreTranslate</bdi>");
    expect(html).not.toContain("etalsnarTerbiL");
  });
});

describe("the actions", () => {
  test("reply, boost with its menu, favourite and react at the start, more at the end, with their counts", () => {
    const html = bare({ status: status({ boosted: true }) });
    expect(html).toMatch(
      /aria-label="Reply, 2".*aria-label="Boost, 1,234".*aria-label="Favourite, 12,840".*aria-label="React".*<span><button [^>]*aria-label="More"/,
    );
    expect(html).toMatch(/aria-haspopup="true"[^>]*aria-label="Boost, 1,234" aria-pressed="true"/);
    expect(html).toContain('aria-label="Favourite, 12,840" aria-pressed="false"');
  });

  test("they are labelled in the reader's locale, whatever the post's", () => {
    const html = bare({ status: status() }, "ja");
    expect(html).toContain('<div lang="ja"><button ');
    expect(html).toContain('aria-label="返信, 2"');
    expect(html).toContain('aria-label="お気に入り, 12,840"');
  });
});

test("under an RTL locale it lays out right-to-left, and the handle still reads left-to-right", () => {
  const html = bare({ status: status() }, "ar-XB");
  expect(html).toMatch(/^<article lang="en" dir="rtl"/);
  expect(html).toContain('<bdi dir="ltr">@lena@toot.example.eu</bdi>');
});
