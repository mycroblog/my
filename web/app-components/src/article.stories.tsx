// SPDX-License-Identifier: AGPL-3.0-only

// Articles (ADR-0160, #3089) as posts draw them, in a column of the timeline's width. In a timeline: an article as its
// card among posts, with a cover in en and ja, without a cover, sensitive, and boosted from another server. On its
// page: the article in full as a thread's focused post, its head, its text set for long reading, its actions, and its
// replies under it, without the line of whom they reply to; with a cover, sensitive, and without a cover.

import { articleText, images } from "@my/design-system/samples";
import { GroupHeading, ThreadFocus, ThreadReply, useLocale } from "@my/design-system";
import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { Post } from "./post";
import { PostContent } from "./post-content";
import { blurhashes, posts, scenes, statusOf, type TimelinePost } from "./post.samples";
import type { PostArticle, PostStatus } from "./status";

const rem = (px: number) => `${px / rootPx}rem`;
const minute = 60_000;
const hour = 60 * minute;

const useLang = () => (new Intl.Locale(useLocale().locale).language === "ja" ? "ja" : "en");

const Column = ({ children }: { children: ReactNode }) => (
  <div style={{ maxWidth: rem(600), paddingInline: rem(16) }}>
    <div style={{ display: "grid", gap: rem(28) }}>{children}</div>
  </div>
);

const cover = (src: string, description: string): PostArticle["cover"] => ({
  id: src.slice(-8),
  type: "image",
  url: src,
  preview_url: src,
  description,
  meta: { width: 640, height: 400 },
  blurhash: blurhashes.get(src),
});

const alice = { name: "Alice Chen", handle: "@alice@example.social", avatar: images.dusk };
const hana = { name: "山田 はな", handle: "@hana@my.example" };
const ravi = { name: "Ravi", handle: "@ravi@fedi.example.net" };
const news = { name: "Open Source Weekly", handle: "@news@social.example.org", avatar: images.logo };
const night = { name: "夜勤の記録", handle: "@yakin@mstdn.example.jp" };
const mika = { name: "佐藤 美香", handle: "@mika@mstdn.example.jp", avatar: images.snow };

/** A sample article: a post with no text in a list, its title, summary and cover, sensitive or not. */
function articleOf(
  post: Omit<TimelinePost, "text" | "visibility" | "counts"> & Partial<Pick<TimelinePost, "counts">>,
  article: PostArticle,
): PostStatus {
  const status = statusOf(
    { visibility: "public", text: "", counts: { replies: 4, boosts: 12, favourites: 31 }, ...post },
    post.id,
  );
  const shown = status.boost ?? status;
  const withArticle = { ...shown, article };
  return status.boost === undefined ? withArticle : { ...status, boost: withArticle };
}

const year: Record<"en" | "ja", PostArticle> = {
  en: {
    title: "Running a small server for a year",
    summary:
      "We opened the server to twelve friends and a cat. A year later two hundred people call it home. What it took, what it cost, and what we would do again.",
    cover: cover(scenes.sea, "A blue sea under a pale sun"),
  },
  ja: {
    title: "小さなサーバーを一年続けて",
    summary:
      "十二人の友だちと一匹の猫のために開いたサーバーが、一年で二百人の居場所になりました。必要だったこと、かかった費用、もう一度やるならどうするか。",
    cover: cover(scenes.sea, "淡い太陽の下の青い海"),
  },
};

const timeline: readonly PostStatus[] = [
  articleOf({ id: "article-en", lang: "en", author: alice, ago: 12 * minute }, year.en),
  ...posts.filter((p) => p.id === "lena-quiet").map((p) => statusOf(p, p.id)),
  articleOf(
    { id: "article-ja", lang: "ja", author: hana, ago: 2 * hour },
    {
      title: "秋の京都で、古い喫茶店を巡った三日間",
      summary: "朝はモーニング、昼は散歩、夜は喫茶店のはしご。三日で十一軒をまわって、また行きたい五軒を選びました。",
      cover: cover(scenes.autumn, "紅葉した山の稜線と夕方の空"),
    },
  ),
  articleOf(
    { id: "article-plain", lang: "en", author: ravi, ago: 5 * hour, counts: { replies: 0, boosts: 2, favourites: 6 } },
    {
      title: "Notes on federation and caching",
      summary:
        "Other servers keep a copy of what you post. What a delete asks of them, what most of them do, and what to expect from the rest.",
    },
  ),
  articleOf(
    { id: "article-sensitive", lang: "ja", author: night, ago: 9 * hour, sensitive: true },
    {
      title: "夜の病棟で働くということ",
      summary: "十年間の夜勤で見たこと。病気と死について書いています。",
      cover: cover(scenes.night, "暗い丘の上に浮かぶ淡い月"),
    },
  ),
  articleOf(
    { id: "article-boosted", lang: "en", author: news, ago: 20 * hour, boostedBy: mika },
    {
      title:
        "This week in open source: a new release of the federation test suite, two security fixes, and a call for translators",
      summary: "",
      cover: cover(scenes.forest, "A green valley with a stream"),
    },
  ),
];

function Timeline() {
  return (
    <Column>
      {timeline.map((status) => (
        <Post key={status.id} status={status} onAction={() => {}} />
      ))}
    </Column>
  );
}

const pageArticle: Record<"en" | "ja", PostArticle> = year;

const reply = (id: string, lang: "en" | "ja", text: string, author: TimelinePost["author"], ago: number) =>
  statusOf(
    {
      id,
      lang,
      author,
      ago,
      visibility: "public",
      replyTo: "@alice@example.social",
      text,
      counts: { replies: 0, boosts: 0, favourites: 2 },
    },
    id,
  );

const replies = {
  en: [
    reply(
      "reply-1",
      "en",
      "@alice The table of costs is the part I was looking for. Thank you for writing it down.",
      ravi,
      40 * minute,
    ),
    reply("reply-2", "en", "@alice A weekly post about what changed: stealing this for our server.", mika, 25 * minute),
  ],
  ja: [
    reply(
      "reply-1",
      "ja",
      "@alice 費用の表がまさに知りたかったところでした。書いてくれてありがとう。",
      hana,
      40 * minute,
    ),
    reply("reply-2", "ja", "@alice 毎週の「何が変わったか」の投稿、うちのサーバーでもまねします。", mika, 25 * minute),
  ],
};

function Page({ sensitive = false, withCover = true }: { sensitive?: boolean; withCover?: boolean }) {
  const lang = useLang();
  const { cover: c, ...rest } = pageArticle[lang];
  const status = articleOf(
    { id: `page-${lang}`, lang, author: alice, ago: 3 * hour, sensitive },
    withCover ? { ...rest, cover: c } : rest,
  );
  return (
    <Column>
      <ThreadFocus>
        <Post status={status} reading={articleText[lang]} onAction={() => {}} />
      </ThreadFocus>
      {replies[lang].map((s, i) => (
        <ThreadReply key={s.id} depth={1}>
          <Post
            status={s}
            onAction={() => {}}
            replyingTo={false}
            context={i === 0 ? <GroupHeading>{lang === "ja" ? "返信" : "Replies"}</GroupHeading> : undefined}
          />
        </ThreadReply>
      ))}
    </Column>
  );
}

// Tagged `themes`: the story tests run it in dark too.
const meta = { title: "Post/Article", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const InTimeline: Story = { render: () => <Timeline /> };
export const ArticlePage: Story = { render: () => <Page /> };
export const ArticlePageSensitive: Story = { render: () => <Page sensitive /> };
export const ArticlePageNoCover: Story = { render: () => <Page withCover={false} /> };

// A sensitive article whose cover and text image are files of this origin, each at an address of its own run, so that
// the play reads from the page's resource timing whether the browser asked for them: never before Show, both after.
const run = crypto.randomUUID();
const folded = {
  cover: `${location.origin}/sensitive-article-cover.png?run=${run}`,
  image: `${location.origin}/sensitive-article-image.png?run=${run}`,
};

function FoldedPage() {
  const status = articleOf(
    { id: "page-folded", lang: "en", author: alice, ago: 3 * hour, sensitive: true },
    {
      title: year.en.title,
      summary: year.en.summary,
      cover: {
        id: "folded-cover",
        type: "image",
        url: folded.cover,
        preview_url: folded.cover,
        description: "A blue sea under a pale sun",
        meta: { width: 640, height: 400 },
        blurhash: blurhashes.get(scenes.sea),
      },
    },
  );
  return (
    <Column>
      <ThreadFocus>
        <Post
          status={status}
          reading={
            <PostContent
              html={`<h2>What it took</h2><p>Twelve friends and a cat.</p><p><img src="${folded.image}" alt="The server rack" width="640" height="400" loading="eager"></p>`}
              article
            />
          }
          onAction={() => {}}
        />
      </ThreadFocus>
    </Column>
  );
}

const asked = (url: string) => performance.getEntriesByType("resource").some((entry) => entry.name === url);

/** A sensitive article's page asks for neither its cover nor an image of its text before Show, and draws both after. */
export const ArticlePageSensitiveShown: Story = {
  globals: { locale: "en" },
  render: () => <FoldedPage />,
  beforeEach: () => {
    performance.setResourceTimingBufferSize(100_000);
  },
  play: async ({ canvasElement }) => {
    const page = within(canvasElement);
    const show = await page.findByRole("button", { name: "Show", expanded: false });
    await expect(page.getByRole("heading", { level: 1, name: year.en.title })).toBeVisible();
    await expect(page.getByText("Sensitive article")).toBeVisible();
    await expect(page.queryByRole("heading", { level: 2, name: "What it took" })).toBeNull();
    await expect(page.queryByText("Twelve friends and a cat.")).toBeNull();
    // No image of the cover or the text in the page, even hidden (the author's avatar is the only image).
    await expect(page.queryByRole("img", { name: "A blue sea under a pale sun", hidden: true })).toBeNull();
    await expect(page.queryByRole("img", { name: "The server rack", hidden: true })).toBeNull();
    // The page at rest: the fonts and the blurhash drawn, a frame to start any request it would make.
    await document.fonts.ready;
    await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    await expect(asked(folded.cover)).toBe(false);
    await expect(asked(folded.image)).toBe(false);
    await userEvent.click(show);
    await expect(await page.findByRole("heading", { level: 2, name: "What it took" })).toBeVisible();
    await expect(page.getByText("Twelve friends and a cat.")).toBeVisible();
    await expect(page.getByRole("img", { name: "The server rack" })).toBeInTheDocument();
    await waitFor(async () => {
      await expect(asked(folded.cover)).toBe(true);
      await expect(asked(folded.image)).toBe(true);
    });
  },
};
