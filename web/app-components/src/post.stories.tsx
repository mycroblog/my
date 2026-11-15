// SPDX-License-Identifier: AGPL-3.0-only

// The post of a timeline: Default, a sample post driven by the Controls (its visibility, counts, content warning, a
// long text, the reader's boost, favourite and bookmark, and whether it is the reader's own), and a story per state:
// plain, local, boosted, a reply, behind a content warning, with images, quoting, with reactions and with the reader's
// own, translated (and the notes when the source language, the provider or both are unknown), hovered and focused
// from the keyboard (shown at rest), long names and handles in en and ja, for a reader without an account, whose
// actions lead to the sign-in, and with a poll: open, its vote in flight, its results, and ended. Each post answers its actions as the
// timeline's do, in a column of the timeline's width.

import { useLocale } from "@my/design-system";
import { Preview, type PreviewState } from "@my/design-system/preview";
import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState, type ReactNode } from "react";
import { statusPath } from "./paths";
import { posts, type TimelinePost } from "./post.samples";
import { SamplePost } from "./sample-post.samples";
import type { PostTranslation } from "./status";

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

const longNameEn: TimelinePost = {
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
};

const longNameJa: TimelinePost = {
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
};

// A post seen on its own server alone, so that the local visibility's glyph shows in its header.
const local: TimelinePost = {
  ...byId("hana-followers"),
  id: "local",
  visibility: "local",
  text: "このサーバーの人だけに。日曜のお茶会、今週は駅前の喫茶店でやります。",
};

const hour = 60 * minute;
const day = 24 * hour;

// Polls (FR-112): open, in flight, voted, hidden until the end, ended, and the reader's own, in en and ja.
const meetup: TimelinePost = {
  id: "poll-one",
  lang: "en",
  author: { name: "Lena Novak", handle: "@lena@toot.example.eu" },
  ago: 25 * minute,
  visibility: "public",
  text: "Which day works for the next meetup? :wave:",
  counts: { replies: 3, boosts: 1, favourites: 4 },
  poll: {
    options: [
      { title: "Saturday morning", votes: 9 },
      { title: "Saturday evening", votes: 17 },
      { title: "Sunday afternoon :party:", votes: 6 },
    ],
    voters: 32,
    endsIn: 2 * day + 3 * hour,
  },
};

const talks: TimelinePost = {
  id: "poll-several",
  lang: "en",
  author: { name: "Open Source Weekly", handle: "@news@social.example.org" },
  ago: 2 * hour,
  visibility: "public",
  text: "Which talks should we record this year? Pick as many as you like.",
  counts: { replies: 12, boosts: 30, favourites: 51 },
  poll: {
    multiple: true,
    options: [
      { title: "Federation from scratch", votes: 64 },
      { title: "Moderating a small server without burning out, a field report from three years of it", votes: 88 },
      { title: "Custom emoji :mycro: and how they travel", votes: 23 },
      { title: "Polls over ActivityPub", votes: 41 },
    ],
    voters: 120,
    endsIn: 5 * hour,
  },
};

const tea: TimelinePost = {
  id: "poll-ja",
  lang: "ja",
  author: { name: "山田 はな", handle: "@hana@my.example" },
  ago: 3 * hour,
  visibility: "public",
  text: "日曜のお茶会、どのお菓子にしましょう？",
  counts: { replies: 2, boosts: 0, favourites: 6 },
  poll: {
    options: [
      { title: "どら焼き", votes: 5 },
      { title: "駅前の喫茶店の季節のタルト（予約が要るので早めに決めたいです）", votes: 11 },
      { title: "おせんべい", votes: 2 },
    ],
    voters: 18,
    endsIn: 40 * minute,
  },
};

const withPoll = (post: TimelinePost, id: string, poll: Partial<NonNullable<TimelinePost["poll"]>>): TimelinePost => {
  if (post.poll === undefined) throw new Error(`sample post ${post.id} has no poll`);
  return { ...post, id, poll: { ...post.poll, ...poll } };
};

const meetupVoted = withPoll(meetup, "poll-voted", { voted: [1] });
const talksVoted = withPoll(talks, "poll-several-voted", { voted: [0, 3] });
const meetupHidden = withPoll(meetup, "poll-hidden", { hideTotals: true, voted: [2] });
const meetupEnded = withPoll(meetup, "poll-ended", { endsIn: -3 * hour });
const talksEnded = withPoll(talks, "poll-several-ended", { endsIn: -day, voted: [1, 2] });
const ownPoll: TimelinePost = { ...withPoll(tea, "poll-own", {}), own: true };
const ownHidden: TimelinePost = { ...withPoll(tea, "poll-own-hidden", { hideTotals: true }), own: true };

const longText =
  "\n\nA second paragraph, long enough to wrap many times in a column of the timeline's width: it keeps going past " +
  "the width of any phone, with a link https://example.org/a/very/long/path/that/never/ends and more words after it.";

/** What the server leaves out of a translation it answers: the language it came from, its provider, or both. */
const withoutSource = (t: PostTranslation): PostTranslation => ({ ...t, source_language: "und" });
const withoutProvider = (t: PostTranslation): PostTranslation => ({ ...t, provider: undefined });

type Translation = ((t: PostTranslation) => PostTranslation) | undefined;

/** A column of the timeline's width, with its side margin, which a post's surface and focus ring reach into. */
const Column = ({ children }: { children: ReactNode }) => (
  <div style={{ maxWidth: rem(600), paddingInline: rem(16) }}>{children}</div>
);

/** A sample post for a reader without an account, and the post the sign-in was last asked for, under it. */
function VisitorPost() {
  const lang = new Intl.Locale(useLocale().locale).language === "ja" ? "ja" : "en";
  const [asked, setAsked] = useState("");
  return (
    <Column>
      <SamplePost
        post={byId("lena-quiet")}
        lang={lang}
        signIn={(status) => {
          setAsked(statusPath(status));
        }}
      />
      <output>{asked === "" ? "" : `→ /login?return_to=${asked}`}</output>
    </Column>
  );
}

/** One sample post in the column, in the reader's language (the toolbar's locale), `state` shown at rest. */
function Shown({
  post,
  state,
  translationAs,
  votingAt,
}: {
  post: TimelinePost;
  state?: PreviewState;
  translationAs?: Translation;
  votingAt?: readonly number[];
}) {
  const lang = new Intl.Locale(useLocale().locale).language === "ja" ? "ja" : "en";
  return (
    <Column>
      <Preview state={state}>
        <SamplePost key={post.id} post={post} lang={lang} translationAs={translationAs} votingAt={votingAt} />
      </Preview>
    </Column>
  );
}

/** Several sample posts one after another, as a timeline spaces them. */
function Several({ posts: shown }: { posts: TimelinePost[] }) {
  const lang = new Intl.Locale(useLocale().locale).language === "ja" ? "ja" : "en";
  return (
    <Column>
      <div style={{ display: "grid", gap: rem(28) }}>
        {shown.map((post) => (
          <SamplePost key={post.id} post={post} lang={lang} />
        ))}
      </div>
    </Column>
  );
}

/** A post in the other language than the reader's, shown translated into theirs. */
function TranslatedPost({ translationAs }: { translationAs?: Translation }) {
  const lang = new Intl.Locale(useLocale().locale).language === "ja" ? "ja" : "en";
  const foreign = posts.find((p) => p.lang !== lang && p.translation?.[lang] !== undefined && p.media === undefined);
  if (foreign === undefined) throw new Error(`no sample post translates into ${lang}`);
  return <Shown post={{ ...foreign, translated: true }} translationAs={translationAs} />;
}

type Args = {
  visibility: TimelinePost["visibility"];
  replies: number;
  boosts: number;
  favourites: number;
  contentWarning: string;
  longText: boolean;
  boosted: boolean;
  favourited: boolean;
  bookmarked: boolean;
  own: boolean;
};

/** The sample post of Default, as the Controls set it. */
const postOf = (a: Args): TimelinePost => {
  const base = byId("lena-quiet");
  return {
    ...base,
    id: "default",
    visibility: a.visibility,
    counts: { replies: a.replies, boosts: a.boosts, favourites: a.favourites },
    ...(a.contentWarning === "" ? {} : { cw: a.contentWarning }),
    text: a.longText ? base.text + longText : base.text,
    boosted: a.boosted,
    favourited: a.favourited,
    bookmarked: a.bookmarked,
    own: a.own,
  };
};

// A story of one state shows a fixed sample, which the Controls do not change.
const fixed = { parameters: { controls: { disable: true } } };

// Tagged `themes`: the story tests run it in dark too.
const meta = {
  title: "Post",
  tags: ["themes"],
  args: {
    visibility: "public",
    replies: 1,
    boosts: 0,
    favourites: 2,
    contentWarning: "",
    longText: false,
    boosted: false,
    favourited: false,
    bookmarked: false,
    own: false,
  },
  argTypes: {
    visibility: { control: "select", options: ["public", "local", "unlisted", "followers", "direct"] },
    replies: { control: "number" },
    boosts: { control: "number" },
    favourites: { control: "number" },
    contentWarning: { control: "text", description: "A content warning, which folds the text; none when empty" },
    longText: { control: "boolean", description: "A second paragraph that wraps many times" },
    boosted: { control: "boolean", description: "The reader boosted it" },
    favourited: { control: "boolean", description: "The reader favourited it" },
    bookmarked: { control: "boolean", description: "The reader bookmarked it" },
    own: { control: "boolean", description: "The reader wrote it: its more menu edits and deletes" },
  },
  render: (args) => <Shown key={JSON.stringify(args)} post={postOf(args)} />,
} satisfies Meta<Args>;
export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Plain: Story = { ...fixed, render: () => <Shown post={byId("lena-quiet")} /> };
export const Local: Story = { ...fixed, render: () => <Shown post={local} /> };
export const Boosted: Story = { ...fixed, render: () => <Shown post={byId("news-boost")} /> };
export const Reply: Story = { ...fixed, render: () => <Shown post={byId("ken-reply")} /> };
export const ContentWarning: Story = { ...fixed, render: () => <Shown post={byId("takuya-cw")} /> };
export const Images: Story = { ...fixed, render: () => <Shown post={byId("ravi-two")} /> };
export const Quoting: Story = { ...fixed, render: () => <Shown post={byId("ravi-quote")} /> };
export const Reactions: Story = { ...fixed, render: () => <Shown post={byId("hana-four")} /> };
export const OwnReaction: Story = { ...fixed, render: () => <Shown post={ownReaction} /> };
/** In the reader's language, with the language it came from and the provider that translated it. */
export const Translated: Story = { ...fixed, render: () => <TranslatedPost /> };
export const TranslatedNoSource: Story = { ...fixed, render: () => <TranslatedPost translationAs={withoutSource} /> };
export const TranslatedNoProvider: Story = {
  ...fixed,
  render: () => <TranslatedPost translationAs={withoutProvider} />,
};
export const TranslatedNeither: Story = {
  ...fixed,
  render: () => <TranslatedPost translationAs={(t) => withoutProvider(withoutSource(t))} />,
};
/**
 * A reader without an account: the post's actions show as for anyone, and one that needs an account leads to the
 * sign-in with the post, which the line under it stands in for once one is pressed.
 */
export const Visitor: Story = { ...fixed, render: () => <VisitorPost /> };
export const Hovered: Story = { ...fixed, render: () => <Shown post={byId("alice-reply")} state="hover" /> };
export const FocusVisible: Story = {
  ...fixed,
  render: () => <Shown post={byId("alice-reply")} state="focus-visible" />,
};
export const LongNames: Story = { ...fixed, render: () => <Shown post={longNameEn} /> };
export const LongNamesJapanese: Story = { ...fixed, render: () => <Shown post={longNameJa} /> };
/** A poll open to the reader: one choice, then several; pressing Vote sends it, and the results follow. */
export const Poll: Story = { ...fixed, render: () => <Several posts={[meetup, talks, tea]} /> };
/** The reader's vote in flight: its choices held, Vote spinning. */
export const PollVoting: Story = { ...fixed, render: () => <Shown post={meetup} votingAt={[1]} /> };
/** Results: after the reader's vote, hidden until the end, ended, and the reader's own poll. */
export const PollResults: Story = {
  ...fixed,
  render: () => <Several posts={[meetupVoted, talksVoted, meetupHidden, talksEnded, ownPoll]} />,
};
/** Ended without the reader's vote, and the reader's own poll whose results are hidden until it ends. */
export const PollEnded: Story = { ...fixed, render: () => <Several posts={[meetupEnded, ownHidden]} /> };
