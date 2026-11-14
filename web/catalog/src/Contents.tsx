// SPDX-License-Identifier: AGPL-3.0-only

// The one content renderer of @my/app-components (PostContent): a remote post as Mastodon sends it, a local post as
// the server renders it, Arabic and Hebrew (RTL) among English (LTR), and hostile HTML that it draws as nothing or as
// text, with that HTML shown as it came; above them, two posts in en and ja with a nested quote, lists and code.

import { PostContent } from "@my/app-components";
import { Avatar, IconButton } from "@my/design-system";
import { iconButton, rootPx, text, type Scheme } from "@my/design-tokens";
import type { CSSProperties } from "react";
import { images } from "./Avatars";
import { serverHtml } from "@my/e2e/fake";
import { emojis } from "./timeline/fixtures";
import { ago, caption, NameLine } from "./Times";

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

const samples = [
  { title: "remote: mentions, hashtags, a shortened link, emoji, code, a quote, a list", html: remote, lang: "en" },
  { title: "local: the server's HTML of a plain text, with a line break and a paragraph", html: local, lang: "ja" },
  { title: "RTL among LTR: Arabic, Hebrew and English, each paragraph in its own direction", html: rtl },
  { title: "hostile: what the server would never let through draws as nothing or as text", html: hostile, lang: "en" },
];

const panel: CSSProperties = {
  alignContent: "start",
  background: "var(--color-surface)",
  border: "1px solid #ccc",
  borderRadius: 8,
  color: "var(--color-text)",
  display: "grid",
  flex: `1 1 ${rem(320)}`,
  gap: rem(20),
  gridTemplateColumns: "minmax(0, 1fr)",
  padding: rem(16),
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

function Specimen({ scheme }: { scheme: Scheme }) {
  return (
    <div style={{ ...panel, colorScheme: scheme }}>
      {samples.map((s) => (
        <div key={s.title} style={{ display: "grid", gap: rem(6), gridTemplateColumns: "minmax(0, 1fr)" }}>
          <div style={caption}>
            {scheme} · {s.title}
          </div>
          <PostContent html={s.html} emojis={emojis} lang={s.lang} />
          {s.html === hostile && <pre style={source}>{s.html}</pre>}
        </div>
      ))}
    </div>
  );
}

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

const blockPosts = [
  { lang: "en", name: "Alice Chen", handle: "@alice@example.social", avatar: images.dusk, html: blocks.en },
  { lang: "ja", name: "佐藤 美香", handle: "@mika@mstdn.example.jp", avatar: images.snow, html: blocks.ja },
] as const;

const one = "minmax(0, 1fr)";

// A post as /app/timeline shows it: the author header over the body and the actions at the full width.
function BlockPost({ post }: { post: (typeof blockPosts)[number] }) {
  return (
    <article lang={post.lang} style={{ display: "grid", gap: rem(8), gridTemplateColumns: one }}>
      <div style={{ alignItems: "center", display: "flex", gap: rem(10) }}>
        <Avatar name={post.name} src={post.avatar} size="post" decorative />
        <div style={{ display: "grid", flex: "1 1 0", fontSize: `${text.body.size}rem`, minWidth: 0 }}>
          <NameLine name={post.name} visibility="public" date={ago(12 * 60_000)} />
          <span
            style={{ ...caption, lineHeight: 1.3, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}
          >
            {post.handle}
          </span>
        </div>
      </div>
      <PostContent html={post.html} emojis={emojis} lang={post.lang} />
      <div style={{ display: "flex", gap: rem(4), marginInline: `calc(-1 * ${iconButton.paddingInline}rem)` }}>
        <IconButton icon="reply" label="Reply" count={2} />
        <IconButton icon="boost" label="Boost" count={4} />
        <IconButton icon="favourite" label="Favourite" count={18} />
        <span style={{ display: "inline-flex", marginInlineStart: "auto" }}>
          <IconButton icon="more" label="More" />
        </span>
      </div>
    </article>
  );
}

// The posts in one scheme, on the page's surface, a post's gap apart; the frame is the catalog's.
function BlockSpecimen({ scheme }: { scheme: Scheme }) {
  return (
    <div style={{ ...panel, colorScheme: scheme, gap: rem(28) }}>
      <div style={caption}>{scheme} · quotes, lists and code in a post</div>
      {blockPosts.map((p) => (
        <BlockPost key={p.lang} post={p} />
      ))}
    </div>
  );
}

export function Contents() {
  return (
    <div style={{ display: "grid", gap: rem(24) }}>
      <div id="content-blocks" style={{ display: "flex", flexWrap: "wrap", gap: rem(16) }}>
        <BlockSpecimen scheme="light" />
        <BlockSpecimen scheme="dark" />
      </div>
      <div style={{ display: "flex", flexWrap: "wrap", gap: rem(16) }}>
        <Specimen scheme="light" />
        <Specimen scheme="dark" />
      </div>
    </div>
  );
}
