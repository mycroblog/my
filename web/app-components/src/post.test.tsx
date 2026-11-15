// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { I18nProvider } from "@my/design-system";
import { renderToStaticMarkup } from "react-dom/server";
import { Post, type PostProps, type TranslationFailure } from "./post";
import type { PostAccount, PostPoll, PostStatus, PostTranslation } from "./status";

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
  test("the name leads to the author's profile and the time to the post's page, the server's addresses", () => {
    const html = bare({ status: status() });
    expect(html).toMatch(/<a [^>]*href="\/@lena@toot.example.eu"[^>]*><bdi id="[^"]+">Lena Novak<\/bdi><\/a>/);
    expect(html).toMatch(/<a [^>]*href="\/@lena@toot.example.eu\/status\/10"[^>]*><bdi><time /);
  });

  test("the avatar, the name isolated, then the visibility and the time at the end, over the handle", () => {
    const html = bare({ status: status() });
    const name = /<bdi id="([^"]+)">Lena Novak<\/bdi>/.exec(html);
    expect(name).not.toBeNull();
    expect(html).toMatch(new RegExp(`^<article lang="en" dir="ltr" tabindex="0" aria-labelledby="${name?.[1] ?? ""}"`));
    expect(html).toMatch(/Lena Novak<\/bdi><\/a><span><span><span role="img" aria-label="Public"><svg /);
    expect(render({ status: status() })).toContain("tabler-icon-world");
    expect(html).toContain("<bdi><time ");
    expect(html).toContain('<bdi dir="ltr">@lena@toot.example.eu</bdi>');
    expect(html.indexOf("3m ago")).toBeLessThan(html.indexOf('<bdi dir="ltr">@lena@toot.example.eu'));
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

  test("without its stamp, the author's line has no visibility and no time", () => {
    const html = bare({ status: status(), stamp: false });
    expect(html).not.toContain('aria-label="Public"');
    expect(html).not.toContain("<time ");
    expect(html).toContain("Lena Novak</bdi>");
  });
});

describe("the screen's own lines", () => {
  test("a context line comes first and details come before the actions, inside the post", () => {
    const html = bare({
      status: status(),
      context: <p>Pinned</p>,
      details: <p>Nov 14, 2026</p>,
    });
    expect(html).toMatch(/^<article [^>]*><p>Pinned<\/p>/);
    expect(html.indexOf("Nov 14, 2026")).toBeGreaterThan(html.indexOf("Quiet morning."));
    expect(html.indexOf("Nov 14, 2026")).toBeLessThan(html.indexOf('aria-label="Reply'));
    expect(html.indexOf("Nov 14, 2026")).toBeLessThan(html.lastIndexOf("</article>"));
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
      /Post translated from <bdi>Japanese<\/bdi> by <bdi>LibreTranslate<\/bdi> · <span><button [^>]*>Show original<\/button><\/span>/,
    );
  });

  test("the note never breaks before or inside the provider's name", () => {
    const named = { ...translation, provider: { name: "Google Cloud Translation" } };
    expect(bare({ status: ja, translation: named })).toContain("by <bdi>Google Cloud Translation</bdi>");
    expect(
      bare({ status: status(), translation: { ...translation, language: "ja", source_language: "en" } }, "ja"),
    ).toContain("<bdi>LibreTranslate</bdi> が");
  });

  test("behind a content warning, the translate line sits in the warning's band, folded or open", () => {
    const warned = status({ spoiler_text: "ネタバレ" });
    const html = bare({ status: warned, translatable: true });
    expect(html).toMatch(
      /<span>ネタバレ<\/span><div aria-live="polite"><p[^>]*>.*<button [^>]*>Translate<\/button>.*<\/div><\/div><button [^>]*aria-expanded="false"/,
    );
  });

  test("the note says what it knows, in the reader's locale", () => {
    const unknown = { ...translation, source_language: "und" };
    expect(bare({ status: ja, translation: unknown })).toContain("Post translated by <bdi>LibreTranslate</bdi> ");
    const { provider: _, ...nameless } = translation;
    expect(bare({ status: ja, translation: nameless })).toContain("Post translated from <bdi>Japanese</bdi> ");
    expect(
      bare({ status: status(), translation: { ...translation, language: "ja", source_language: "en" } }, "ja"),
    ).toContain("<bdi>LibreTranslate</bdi> が投稿を<bdi>英語</bdi>から翻⁠訳");
  });

  /** The words of the translate line (the polite live region), the joiners that hold its last word dropped. */
  const lineOf = (props: PostProps, locale?: string) => {
    const html = bare(props, locale).replaceAll("⁠", "");
    const start = html.indexOf('<div aria-live="polite">');
    return start < 0 ? "" : html.slice(start, html.indexOf("</div>", start));
  };
  const words = (line: string) =>
    line
      .replace(/<[^>]*>/g, "")
      .replaceAll("&#x27;", "'")
      .replaceAll(" ", " ");

  test("on its way, the line says it is translating, and offers nothing to press", () => {
    const line = lineOf({ status: ja, translatable: true, translating: true });
    expect(words(line)).toBe("Translating…");
    expect(line).not.toContain("<button");
    expect(bare({ status: ja, translatable: true, translating: true })).toContain("小さなサーバー");
  });

  test("while the browser's model downloads, the line shows its share, and announces it by quarters", () => {
    const line = lineOf({ status: ja, translatable: true, translating: 0.4 });
    expect(line).toContain('<span aria-hidden="true">Downloading this browser&#x27;s translation model… 40%</span>');
    expect(line).toMatch(/<span>Downloading this browser&#x27;s translation model… 25%<\/span>/);
    const at = (share: number) =>
      /<span>Downloading[^<]*… (\d+)%<\/span>/.exec(lineOf({ status: ja, translating: share }))?.[1];
    expect([0.1, 0.26, 0.5, 0.74, 0.99, 1].map(at)).toEqual(["0", "25", "50", "50", "75", "100"]);
  });

  const failure = (failed: TranslationFailure, locale?: string) =>
    lineOf({ status: ja, translatable: true, translationFailure: failed }, locale);

  test("over the limit with nothing else to try, the line says when translation comes back, with nothing to press", () => {
    const line = failure({ reason: "limit", retry: false });
    expect(words(line)).toBe(
      "This server's monthly translation limit is reached. Translation is available again next month.",
    );
    expect(line).not.toContain("<button");
  });

  test("over the limit where the browser may translate, the next press goes to the browser", () => {
    const line = failure({ reason: "limit", retry: "browser" });
    expect(words(line)).toBe("This server's monthly translation limit is reached. · Translate in this browser");
    expect(line).toMatch(/<button [^>]*>Translate in this browser<\/button>/);
    expect(line).not.toContain("next month");
  });

  test("a visibility the provider may not receive hands the next press to the browser", () => {
    expect(words(failure({ reason: "visibility", retry: "browser" }))).toBe(
      "This post cannot be sent to this server's translation service. · Translate in this browser",
    );
  });

  test("the provider down says so, and Try again asks it again", () => {
    const line = failure({ reason: "failed", retry: "server" });
    expect(words(line)).toBe("This server's translation service is not answering. · Try again");
    expect(line).toMatch(/<button [^>]*>Try again<\/button>/);
  });

  test("a language the provider does not translate, and a browser unable to, offer nothing to press", () => {
    const language = failure({ reason: "language", retry: false });
    expect(words(language)).toBe("This server's translation service does not translate between these languages.");
    expect(language).not.toContain("<button");
    const browser = failure({ reason: "browser", retry: false });
    expect(words(browser)).toBe("This browser cannot translate this post.");
    expect(browser).not.toContain("<button");
  });

  test("a refusal no longer offers Translate itself", () => {
    for (const failed of [
      { reason: "limit", retry: false },
      { reason: "failed", retry: "server" },
      { reason: "visibility", retry: "browser" },
    ] as const) {
      expect(failure(failed)).not.toMatch(/>Translate<\/button>/);
    }
  });

  test("the refusals read in the reader's locale", () => {
    expect(words(failure({ reason: "limit", retry: false }, "ja"))).toBe(
      "このサーバーの今月の翻訳の上限に達しました。 来月からまた翻訳できます。",
    );
    expect(words(failure({ reason: "failed", retry: "server" }, "ja"))).toContain("もう一度試す");
    expect(words(failure({ reason: "visibility", retry: "browser" }, "ja"))).toContain("このブラウザで翻訳する");
    expect(words(lineOf({ status: ja, translatable: true, translating: true }, "ja"))).toBe("翻訳しています…");
  });

  test("behind a content warning, a refusal sits in the warning's band, before Show", () => {
    const warned = status({ language: "ja", spoiler_text: "ネタバレ" });
    const html = bare({
      status: warned,
      translatable: true,
      translationFailure: { reason: "visibility", retry: "browser" },
    });
    expect(html).toMatch(
      /<span>ネタバレ<\/span><div aria-live="polite"><p[^>]*>.*This post cannot be sent.*<\/div><\/div><\/div><button [^>]*aria-expanded="false"/,
    );
  });

  test("translated behind a content warning: the warning, the text, the poll's options and the alt text", () => {
    const whole = status({
      language: "ja",
      spoiler_text: "ネタバレ",
      content: "<p>小さなサーバー</p>",
      media_attachments: [{ id: "m", type: "image", description: "スープ", preview_url: "https://m/s.webp" }],
      poll: {
        id: "p",
        expires_at: new Date(Date.now() + 2 * 86_400_000).toISOString(),
        expired: false,
        multiple: false,
        votes_count: 0,
        voters_count: 0,
        options: [
          { title: "ライ麦", votes_count: 0 },
          { title: "サワー種", votes_count: 0 },
        ],
        voted: false,
        own_votes: [],
      },
    });
    const html = bare({
      status: whole,
      translatable: true,
      translation: {
        ...translation,
        content: "<p>[en] 小さなサーバー</p>",
        spoiler_text: "[en] ネタバレ",
        media_attachments: [{ id: "m", description: "[en] スープ" }],
        poll_options: ["[en] ライ麦", "[en] サワー種"],
      },
    });
    expect(html).toMatch(/<span>\[en\] ネタバレ<\/span><div aria-live="polite">.*Show original/);
    expect(html).toContain("[en] 小さなサーバー");
    expect(html).toMatch(/>\[en\] ライ麦<\/span><\/label>.*>\[en\] サワー種<\/span><\/label>/);
    expect(html).toContain('alt="[en] スープ"');
    for (const original of ["<span>ネタバレ</span>", '<p dir="auto">小さなサーバー', ">ライ麦<", 'alt="スープ"']) {
      expect(html).not.toContain(original);
    }
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
      /aria-label="Reply, 2".*aria-label="Boost, 1.2K".*aria-label="Favourite, 12.8K".*aria-label="React".*<span><button [^>]*aria-label="More"/,
    );
    expect(html).toMatch(/aria-haspopup="true"[^>]*aria-label="Boost, 1.2K" aria-pressed="true"/);
    expect(html).toContain('aria-label="Favourite, 12.8K" aria-pressed="false"');
  });

  test("they are labelled in the reader's locale, whatever the post's", () => {
    const html = bare({ status: status() }, "ja");
    expect(html).toContain('<div lang="ja"><button ');
    expect(html).toContain('aria-label="返信, 2"');
    expect(html).toContain('aria-label="お気に入り, 1.3万"');
  });
});

describe("the poll", () => {
  const inTwoDays = () => new Date(Date.now() + 2 * 86_400_000 + 3_600_000).toISOString();
  const poll = (over: Partial<PostPoll> = {}): PostPoll => ({
    id: "p",
    expires_at: inTwoDays(),
    expired: false,
    multiple: false,
    votes_count: 4,
    voters_count: 4,
    options: [
      { title: "Sourdough", votes_count: 3 },
      { title: "Rye", votes_count: 1 },
    ],
    voted: false,
    own_votes: [],
    ...over,
  });
  const withPoll = (over: Partial<PostPoll> = {}) => status({ poll: poll(over) });
  /** The markup of the poll alone, from its group to the post's actions. */
  const pollOf = (props: PostProps, locale?: string) => {
    const html = bare(props, locale);
    const start = html.indexOf('<div role="group" aria-label="Poll"');
    const ja = html.indexOf('<div role="group" aria-label="投票"');
    const from = start >= 0 ? start : ja;
    return from < 0 ? "" : html.slice(from, html.indexOf('<div lang="', from));
  };
  const vote = /<button [^>]*>(<span>)?Vote</;

  test("a post without a poll draws none", () => {
    expect(bare({ status: status() })).not.toContain('aria-label="Poll"');
  });

  test("open to the reader: one choice is a radio per option, Vote waits for a choice, then who voted and the end", () => {
    const html = pollOf({ status: withPoll() });
    expect(html).toContain('<div role="radiogroup" aria-label="Options">');
    expect(html.match(/<input type="radio"/g)).toHaveLength(2);
    expect(html).toMatch(/<label>.*Sourdough<\/span><\/label><label>.*Rye<\/span><\/label>/);
    expect(html).toMatch(/<button [^>]*disabled=""[^>]*><span>Vote<\/span><\/button>/);
    expect(html).toMatch(/4 people voted.* · .*Ends in 2d/);
    expect(html).not.toContain("%");
  });

  test("several choices are a checkbox per option, and the line says several may be chosen", () => {
    const html = pollOf({ status: withPoll({ multiple: true }) });
    expect(html).toContain('<div role="group" aria-label="Options">');
    expect(html.match(/<input type="checkbox"/g)).toHaveLength(2);
    expect(html).not.toContain('type="radio"');
    expect(html).toMatch(/Several choices allowed.* · .*4 people voted.* · .*Ends in 2d/);
  });

  test("in flight, the reader's choices stay held and Vote is busy, with nothing to change", () => {
    const html = pollOf({ status: withPoll({ multiple: true }), voting: [1] });
    expect(html.match(/<input type="checkbox" disabled=""/g)).toHaveLength(2);
    expect(html).toMatch(/<input type="checkbox" disabled=""[^>]*checked=""[^>]*\/>.*Rye/);
    expect(html).not.toMatch(/<input [^>]*checked=""[^>]*\/>.*Sourdough.*Rye/);
    expect(html).toMatch(/<button [^>]*aria-disabled="true"[^>]*aria-busy="true"[^>]*><span>Vote<\/span>/);
    expect(html).not.toContain("%");
  });

  test("after the reader's vote, each option's share, the reader's choice marked, and no Vote", () => {
    const html = pollOf({ status: withPoll({ voted: true, own_votes: [0] }) });
    expect(html).toMatch(
      /<li>.*Sourdough<span> \(your vote\)<\/span><\/span><span>75%<\/span>.*<\/li><li>.*Rye<\/span><span>25%</,
    );
    expect(html).not.toMatch(/Rye<span> \(your vote\)/);
    expect(html).not.toContain("<input");
    expect(html).not.toMatch(vote);
    expect(html).toMatch(/4 people voted.* · .*Ends in 2d/);
  });

  test("several choices voted mark each of them", () => {
    const html = pollOf({ status: withPoll({ multiple: true, voted: true, own_votes: [0, 1] }) });
    expect(html.match(/ \(your vote\)/g)).toHaveLength(2);
  });

  test("ended, it shows the results without a vote, and says it ended", () => {
    const html = pollOf({
      status: withPoll({ expired: true, expires_at: new Date(Date.now() - 3_600_000).toISOString() }),
    });
    expect(html).toMatch(/Sourdough<\/span><span>75%<\/span>.*Rye<\/span><span>25%</);
    expect(html).not.toContain("<input");
    expect(html).not.toMatch(vote);
    expect(html).not.toContain("your vote");
    expect(html).toMatch(/4 people voted.* · .*Ended</);
    expect(html).not.toContain("Ends in");
  });

  test("the reader's own poll shows its results, never a vote", () => {
    const html = pollOf({ status: withPoll(), own: true });
    expect(html).toContain("75%");
    expect(html).not.toContain("<input");
    expect(html).not.toMatch(vote);
  });

  test("counts hidden until it ends: no share of any option, and the line says when they show", () => {
    const hidden = { options: [{ title: "Sourdough" }, { title: "Rye" }] };
    const voted = pollOf({ status: withPoll({ ...hidden, voted: true, own_votes: [1] }) });
    expect(voted).not.toContain("%");
    expect(voted).toMatch(/Rye<span> \(your vote\)<\/span>/);
    expect(voted).toContain("Counts show when it ends");
    expect(pollOf({ status: withPoll(hidden), own: true })).toContain("Counts show when it ends");
  });

  test("a poll whose voters are unknown counts its votes instead", () => {
    const html = pollOf({ status: withPoll({ voters_count: undefined }) });
    expect(html).toContain("4 votes");
    expect(html).not.toContain("people voted");
  });

  test("it reads in the reader's locale: the group, the options, the line and Vote", () => {
    const open = pollOf({ status: withPoll({ multiple: true, voters_count: undefined }) }, "ja");
    expect(open).toContain('<div role="group" aria-label="投票">');
    expect(open).toContain('aria-label="選択肢"');
    expect(open).toMatch(/複数選べます.* · .*4票.* · .*2日後に締切/);
    expect(open).toMatch(/<span>投票する<\/span><\/button>/);
    const ended = pollOf({ status: withPoll({ expired: true, voted: true, own_votes: [0] }) }, "ja");
    expect(ended).toMatch(/4人が投票.* · .*締切済み/);
    expect(ended).toMatch(/Sourdough<span>[^<]*自分の投票[^<]*<\/span><\/span><span>75%/);
    const hidden = pollOf({ status: withPoll({ options: [{ title: "A" }, { title: "B" }] }), own: true }, "ja");
    expect(hidden).toContain("票数は締切後に表示");
  });
});

test("under an RTL locale it lays out right-to-left, and the handle still reads left-to-right", () => {
  const html = bare({ status: status() }, "ar-XB");
  expect(html).toMatch(/^<article lang="en" dir="rtl"/);
  expect(html).toContain('<bdi dir="ltr">@lena@toot.example.eu</bdi>');
});

describe("the link card", () => {
  const ann: PostAccount = {
    id: "4",
    username: "ann",
    acct: "ann@blog.example",
    domain: "blog.example",
    display_name: "Ann Lee",
    emojis: [],
  };
  const card: NonNullable<PostStatus["card"]> = {
    url: "https://blog.example/posts/rye",
    title: "Baking rye at home",
    description: "What three weeks of starters taught me.",
    site_name: "Field Notes",
    image: {
      url: "https://my.example/proxy/rye.webp",
      width: 1200,
      height: 600,
      description: "A dark loaf on a board",
      blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj",
    },
    author: ann,
  };
  const linked = (over: Partial<PostStatus> = {}) =>
    status({ content: '<p>Read <a href="https://blog.example/posts/rye">this</a></p>', card, ...over });
  const cardOf = (html: string) =>
    /<a [^>]*href="https:\/\/blog.example\/posts\/rye" target="_blank"[^>]*>.*?<\/a>/.exec(html)?.[0];

  test("a post with a card shows it after its text: the image through the proxy, the site, the title, and by its author", () => {
    const html = bare({ status: linked() });
    const shown = cardOf(html) ?? "";
    expect(shown).toContain('<img src="https://my.example/proxy/rye.webp" alt="A dark loaf on a board"/>');
    expect(shown).toMatch(/Field Notes.*Baking rye at home.*What three weeks of starters taught me\./);
    expect(shown).toContain("<span>by Ann Lee</span>");
    expect(shown).toContain("(opens in a new tab)");
    expect(html.indexOf(shown)).toBeGreaterThan(html.indexOf("Read "));
    expect(html.indexOf(shown)).toBeLessThan(html.indexOf('aria-label="Reply'));
  });

  test("small or large as the reader prefers, the same card", () => {
    expect(cardOf(bare({ status: linked(), cardSize: "small" }))).toContain("Baking rye at home");
    expect(cardOf(bare({ status: linked(), cardSize: "large" }))).toContain("Baking rye at home");
  });

  test("none when the post has media", () => {
    const html = bare({
      status: linked({
        media_attachments: [{ id: "m", type: "image", preview_url: "https://m/s.webp", description: "Soup" }],
      }),
    });
    expect(cardOf(html)).toBeUndefined();
    expect(html).not.toContain("Baking rye at home");
    expect(html).toContain('alt="Soup"');
  });

  test("none when the post quotes another", () => {
    const quoted = status({ id: "30", account: ken, content: "<p>小さなサーバー</p>" });
    const html = bare({ status: linked({ quote: { state: "accepted", quoted_status: quoted } }) });
    expect(cardOf(html)).toBeUndefined();
    expect(html).not.toContain("Baking rye at home");
    expect(html).toContain("小さなサーバー");
  });

  test("a sensitive post covers the card's image, with its line outside the link", () => {
    const html = bare({ status: linked({ sensitive: true }) });
    expect(html).not.toContain("proxy/rye.webp");
    expect(html).toMatch(
      /Sensitive image<\/span><button [^>]*aria-expanded="false"[^>]*>.*?<span lang="en">Show<\/span>.*?<\/button><\/div><a [^>]*href="https:\/\/blog.example\/posts\/rye"/,
    );
  });

  test("behind a content warning, the card folds with the text", () => {
    const html = bare({ status: linked({ spoiler_text: "Food" }) });
    expect(html).toMatch(/ hidden="">.*Baking rye at home/);
  });
});

describe("every kind of media", () => {
  const media: PostStatus["media_attachments"] = [
    {
      id: "v",
      type: "video",
      url: "https://m/v.webm",
      preview_url: "https://m/v.webp",
      description: "Waves at dusk",
      meta: { duration: 92 },
      blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj",
    },
    {
      id: "g",
      type: "gifv",
      url: "https://m/g.mp4",
      preview_url: "https://m/g.webp",
      blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj",
    },
    { id: "s", type: "audio", url: "https://m/s.mp3", description: "Rain", meta: { duration: 192 } },
    { id: "f", type: "unknown", url: "https://files.example.org/slides.key" },
  ];

  test("a video, a GIF, a sound and a file each draw as their kind, with their descriptions", () => {
    const html = bare({ status: status({ media_attachments: media }) });
    expect(html).toContain('aria-label="Play video: Waves at dusk"');
    expect(html).toContain("<span>1:32</span>");
    expect(html).toMatch(/<video src="https:\/\/m\/g.mp4"[^>]*muted=""/);
    expect(html).toContain('aria-label="Open GIF"');
    expect(html).toContain('aria-label="Play audio"');
    expect(html).toContain("<span>0:00 of 3:12</span>");
    expect(html).toMatch(/<a [^>]*href="https:\/\/files.example.org\/slides.key" target="_blank"/);
  });

  test("sensitive, the line names them all and each picture's blurhash is drawn, its file not asked for", () => {
    const html = bare({ status: status({ sensitive: true, media_attachments: media }) });
    expect(html).toMatch(/4 sensitive attachments<\/span><button [^>]*>.*?<span lang="en">Show</);
    expect(html.match(/<canvas /g)).toHaveLength(2);
    expect(html).not.toMatch(/(src|poster)="https:\/\/m\//);
  });

  test("the sensitive line reads in the reader's locale", () => {
    const html = bare(
      {
        status: status({
          sensitive: true,
          media_attachments: [
            { id: "a", type: "image", preview_url: "https://m/a.webp" },
            { id: "b", type: "image", preview_url: "https://m/b.webp" },
          ],
        }),
      },
      "ja",
    );
    expect(html).toMatch(/センシティブな画像 2 枚<\/span><button [^>]*>.*?<span lang="ja">表示<\/span>/);
  });

  test("a content warning and the sensitive flag are separate: the warning folds the line and the covered media", () => {
    const html = bare({
      status: status({
        spoiler_text: "Injury",
        sensitive: true,
        media_attachments: [
          { id: "a", type: "image", preview_url: "https://m/a.webp", blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj" },
        ],
      }),
    });
    expect(html).toMatch(/>Injury<\/div><button [^>]*aria-expanded="false"/);
    expect(html).toMatch(/ hidden="">.*Sensitive image<\/span><button /);
    expect(html).not.toContain("https://m/a.webp");
  });
});
