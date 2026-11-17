// SPDX-License-Identifier: AGPL-3.0-only

// The one content renderer (PostContent), a story per source of HTML: a remote post as Mastodon sends it, a local post
// as the server renders it, Arabic and Hebrew (RTL) among English (LTR), hostile HTML that it draws as nothing or as
// text, with that HTML shown as it came, and the blocks of a post (a nested quote, lists and code) in en and ja.

import { caption } from "@my/design-system/samples";
import { rootPx } from "@my/design-tokens";
import { serverHtml } from "@my/e2e/fake";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { CSSProperties } from "react";
import { PostContent } from "./post-content";
import { emojis } from "./post.samples";

const rem = (px: number) => `${px / rootPx}rem`;

const remote =
  '<p><span class="h-card" translate="no"><a href="https://mastodon.example/@alice" class="u-url mention" rel="nofollow noopener noreferrer" target="_blank">@<span>alice</span></a></span> the bakery reopened :wave: ' +
  '<a href="https://mastodon.example/tags/SmallWeb" class="mention hashtag" rel="nofollow noopener noreferrer" target="_blank">#<span>SmallWeb</span></a> ' +
  '<a href="https://mastodon.example/tags/%E3%83%91%E3%83%B3" class="mention hashtag" rel="nofollow noopener noreferrer" target="_blank">#<span>パン</span></a></p>' +
  '<p>Photos: <a href="https://photos.example.org/albums/2026/11/the-corner-bakery-on-opening-day" rel="nofollow noopener noreferrer" target="_blank"><span class="invisible">https://</span><span class="ellipsis">photos.example.org/albums/2026</span><span class="invisible">/11/the-corner-bakery-on-opening-day</span></a><br>' +
  "Line two, after a <code>&lt;br&gt;</code> :blobcat:</p>" +
  "<blockquote><p>Bread is better shared.</p></blockquote><ul><li>sourdough</li><li>melon pan</li></ul>";

const local = serverHtml(
  "週末に小さなサーバーを立ててみました :mycro: @alice@example.social さんありがとう！\n設定の手順はブログに https://example.jp/blog/2026/11/small-server-from-scratch\n\n#小さなサーバー #fediverse :sparkle:",
);

const rtl =
  '<p lang="ar">مرحبا بالجميع! هذا أول منشور لي على <a href="https://my.example/tags/fediverse" class="mention hashtag" rel="nofollow noopener noreferrer" target="_blank">#<span>fediverse</span></a> :wave:</p>' +
  '<p lang="he">שלום לכולם, <span class="h-card"><a href="https://my.example/@ravi" class="u-url mention" rel="nofollow noopener noreferrer" target="_blank">@<span>ravi</span></a></span>!</p>' +
  "<p>And an English line after them, ending with its own punctuation!</p>";

const hostile =
  '<p onclick="alert(1)" style="position:fixed;inset:0">Click <a href="javascript:alert(1)">here</a> or <a href="data:text/html,x">there</a>' +
  '<script>alert("script")</script><img src=x onerror="alert(1)"><iframe src="https://evil.example/"></iframe>' +
  '<style>body{display:none}</style><form action="https://evil.example/"><input name="password"></form></p>' +
  "<div><h1>A heading</h1><marquee>moving</marquee></div><!-- a comment --><svg><text>svg</text></svg>" +
  "<p>&lt;b&gt;escaped&lt;/b&gt; stays text</p>";

// A column of the timeline's width, with its side margin.
const column: CSSProperties = {
  display: "grid",
  gap: rem(12),
  gridTemplateColumns: "minmax(0, 1fr)",
  maxWidth: rem(600),
  paddingInline: rem(16),
};

const source: CSSProperties = {
  ...caption,
  background: "var(--color-surface-raised)",
  borderRadius: "var(--radius-control)",
  margin: 0,
  overflowWrap: "anywhere",
  padding: rem(8),
  whiteSpace: "pre-wrap",
};

/** The content of `html`, as a post shows it, and with `shown` the HTML as it came under it. */
const Content = ({ html, lang, shown = false }: { html: string; lang?: string; shown?: boolean }) => (
  <div style={column}>
    <PostContent html={html} emojis={emojis} lang={lang} />
    {shown && <pre style={source}>{html}</pre>}
  </div>
);

// The blocks of a post as Mastodon sends them: a quote with a nested quote, an ordered and an unordered list, inline
// code and a code block with a line too long for a phone, in en and in ja.
const blocks = {
  en:
    "<p>Notes from the meetup, for those who missed it:</p>" +
    "<blockquote><p>Federation is a conversation, not a broadcast.</p>" +
    "<blockquote><p>And every server is somebody's home.</p></blockquote>" +
    "<p>From the keynote, quoting the first talk of the day.</p></blockquote>" +
    "<ol><li>Start a server with <code>my init</code></li><li>Invite three friends</li>" +
    "<li>Let it grow at the pace of its people, one conversation at a time</li></ol>" +
    "<ul><li>sourdough</li><li>melon pan, which sold out before nine</li></ul>" +
    "<p>Then run <code>my serve --config /etc/my/config.toml</code> and look it up:</p>" +
    "<pre><code>curl -sS 'https://my.example/.well-known/webfinger?resource=acct:alice@my.example' | jq '.links[] | select(.rel == \"self\")'</code></pre>",
  ja:
    "<p>勉強会のメモです。来られなかった人へ:</p>" +
    "<blockquote><p>フェデレーションは放送ではなく、会話です。</p>" +
    "<blockquote><p>どのサーバーも、だれかの家です。</p></blockquote>" +
    "<p>基調講演より。朝いちばんの発表を引用していました。</p></blockquote>" +
    "<ol><li><code>my init</code> でサーバーを立てる</li><li>友だちを三人誘う</li>" +
    "<li>そこにいる人たちの速さで、会話ひとつずつ育てる</li></ol>" +
    "<ul><li>サワードウ</li><li>メロンパン(九時前に売り切れ)</li></ul>" +
    "<p>あとは <code>my serve --config /etc/my/config.toml</code> で起動して、確かめます:</p>" +
    "<pre><code>curl -sS 'https://my.example/.well-known/webfinger?resource=acct:mika@my.example' | jq '.links[] | select(.rel == \"self\")'</code></pre>",
};

// The HTML is each story's sample; the content renderer takes nothing a reader would turn. Tagged `themes`, the story
// tests run it in dark too.
const meta = { title: "Post content", tags: ["themes"] } satisfies Meta;
export default meta;

/** A remote post as Mastodon sends it: mentions, hashtags, a shortened link, emoji, code, a quote, a list. */
export const Remote: StoryObj = { render: () => <Content html={remote} lang="en" /> };
/** A local post as the server renders its plain text, with a line break and a paragraph. */
export const Local: StoryObj = { render: () => <Content html={local} lang="ja" /> };
/** Arabic, Hebrew and English, each paragraph in its own direction. */
export const RightToLeftAmongLeftToRight: StoryObj = { render: () => <Content html={rtl} /> };
/** What the server would never let through, drawn as nothing or as text, with the HTML as it came under it. */
export const Hostile: StoryObj = { render: () => <Content html={hostile} lang="en" shown /> };
/** A quote with a nested quote, an ordered and an unordered list, inline code and a code block too long for a phone. */
export const Blocks: StoryObj = { render: () => <Content html={blocks.en} lang="en" /> };
export const BlocksJapanese: StoryObj = { render: () => <Content html={blocks.ja} lang="ja" /> };
