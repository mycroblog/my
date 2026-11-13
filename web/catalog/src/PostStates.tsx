// SPDX-License-Identifier: AGPL-3.0-only

// The post of @my/app-components in each of its states, one under another in a column of the timeline's width, in
// the page's locale and scheme: plain, local, boosted, a reply, behind a content warning, with images, quoting, with
// reactions and with the reader's own, translated (and the notes when the source language, the provider or both are
// unknown), hovered and focused from the keyboard (shown at rest), and long names and handles in en and ja. Each row
// has an anchor, `post-states-<id>`. Each post answers its actions as /timeline's do.

import type { PostTranslation } from "@my/app-components";
import { Preview, type PreviewState } from "@my/design-system/preview";
import { rootPx, text } from "@my/design-tokens";
import { useLocale } from "@my/design-system";
import { SamplePost } from "./timeline/Column";
import { posts, type TimelinePost } from "./timeline/fixtures";

const rem = (px: number) => `${px / rootPx}rem`;
const minute = 60_000;

const byId = (id: string) => {
  const post = posts.find((p) => p.id === id);
  if (!post) throw new Error(`no sample post ${id}`);
  return post;
};

const ownReaction: TimelinePost = {
  ...byId("lena-quiet"),
  id: "own-reaction",
  reactions: [
    { emoji: "☕", count: 4, mine: true },
    { emoji: ":sparkle:", count: 2 },
    { emoji: "📚", count: 1 },
  ],
};

const longNames: TimelinePost[] = [
  {
    id: "long-en",
    lang: "en",
    author: {
      name: "Bartholomew Fitzgerald-Montgomery, keeper of the very small server",
      handle: "@bartholomew.fitzgerald.montgomery@a-rather-long-instance-name.example.social",
    },
    ago: 4 * minute,
    visibility: "unlisted",
    replyTo: "@mika@mstdn.example.jp",
    text: "@mika A long name and a long handle, so that the name truncates first and the handle at its end.",
    counts: { replies: 12, boosts: 3_456, favourites: 78_901 },
  },
  {
    id: "long-ja",
    lang: "ja",
    author: {
      name: "写真と本とお茶が好きな山田はなこ（小さなサーバーの管理人をしています）",
      handle: "@yamada_hanako_photos_books_tea@mstdn.example.jp",
    },
    ago: 40 * minute,
    visibility: "followers",
    boostedBy: { name: "とても長い表示名のアカウントがブーストしたときの例", handle: "@long@my.example" },
    text: "長い名前と長いハンドルの例です。名前が先に切れて、ハンドルは末尾で切れます。",
    counts: { replies: 0, boosts: 0, favourites: 2 },
  },
];

// A post seen on its own server alone, so that the local visibility's glyph shows in its header.
const local: TimelinePost = {
  ...byId("hana-followers"),
  id: "local",
  visibility: "local",
  text: "このサーバーの人だけに。日曜のお茶会、今週は駅前の喫茶店でやります。",
};

/** What the server leaves out of a translation it answers: the language it came from, its provider, or both. */
const withoutSource = (t: PostTranslation): PostTranslation => ({ ...t, source_language: "und" });
const withoutProvider = (t: PostTranslation): PostTranslation => ({ ...t, provider: undefined });

type Row = {
  /** The row's anchor in the section, `post-states-<id>`. */
  id: string;
  title: string;
  post: TimelinePost;
  state?: PreviewState;
  translationAs?: ((t: PostTranslation) => PostTranslation) | undefined;
};

function rows(lang: "en" | "ja"): Row[] {
  // A post in the other language, shown translated into the reader's.
  const foreign = posts.find((p) => p.lang !== lang && p.translation?.[lang] !== undefined && p.media === undefined);
  const translated = (id: string, title: string, translationAs?: Row["translationAs"]): Row[] =>
    foreign ? [{ id, title, post: { ...foreign, id, translated: true }, translationAs }] : [];
  return [
    { id: "plain", title: "plain", post: byId("lena-quiet") },
    { id: "local", title: "local visibility (the glyph before the time)", post: local },
    { id: "boosted", title: "boosted", post: byId("news-boost") },
    { id: "reply", title: "a reply", post: byId("ken-reply") },
    { id: "cw", title: "behind a content warning", post: byId("takuya-cw") },
    { id: "images", title: "with images", post: byId("ravi-two") },
    { id: "quoting", title: "quoting", post: byId("ravi-quote") },
    { id: "reactions", title: "with reactions", post: byId("hana-four") },
    { id: "own-reaction", title: "with the reader's own reaction", post: ownReaction },
    ...translated("translated", "translated: language and provider known"),
    ...translated("translated-no-source", "translated: provider known, source language unknown", withoutSource),
    ...translated("translated-no-provider", "translated: source language known, provider unknown", withoutProvider),
    ...translated("translated-neither", "translated: neither source language nor provider known", (t) =>
      withoutProvider(withoutSource(t)),
    ),
    { id: "hovered", title: "hovered", post: byId("alice-reply"), state: "hover" },
    { id: "focused", title: "focused from the keyboard", post: byId("alice-reply"), state: "focus-visible" },
    ...longNames.map((post) => ({ id: post.id, title: `long names (${post.lang})`, post })),
  ];
}

export function PostStates() {
  const lang = new Intl.Locale(useLocale().locale).language === "ja" ? "ja" : "en";
  return (
    <div
      style={{
        color: "var(--color-text)",
        display: "grid",
        gap: rem(36),
        gridTemplateColumns: "minmax(0, 1fr)",
        maxWidth: rem(600),
        // The timeline's side margin, so that a post's surface and focus ring, which reach past it, stay in sight.
        paddingInline: rem(16),
      }}
    >
      {rows(lang).map((row) => (
        // The caption stands clear of the surface and the ring above the post (its reach, 14, and the ring, 7).
        <div
          key={row.id}
          id={`post-states-${row.id}`}
          style={{ display: "grid", gap: rem(30), gridTemplateColumns: "minmax(0, 1fr)" }}
        >
          <h3
            lang="en"
            style={{
              color: "var(--color-muted-text)",
              fontSize: `${text.caption.size}rem`,
              fontWeight: 400,
              margin: 0,
            }}
          >
            {row.title}
          </h3>
          <Preview state={row.state}>
            <SamplePost post={row.post} lang={lang} translationAs={row.translationAs} />
          </Preview>
        </div>
      ))}
    </div>
  );
}
