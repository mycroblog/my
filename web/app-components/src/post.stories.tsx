// SPDX-License-Identifier: AGPL-3.0-only

// The post of a timeline: Default, a sample post driven by the Controls (its visibility, counts, content warning, a
// long text, the reader's boost, favourite and bookmark, and whether it is the reader's own), and a story per state:
// plain, local, boosted, a reply, behind a content warning, with images, quoting, with reactions and with the reader's
// own, translated (and the notes when the source language, the provider or both are unknown), offered for
// translation, its translation on its way (and the browser's model downloading), refused for each reason, translated
// behind a content warning with a poll and an image, hovered and focused
// from the keyboard (shown at rest), long names and handles in en and ja, for a reader without an account, whose
// actions lead to the sign-in, with a poll: open, its vote in flight, its results, and ended, the more menu open on
// one's own post and another's, the confirmations of delete and of delete and redraft, edited posts, and an edit
// history; and what a post carries besides images: its link card in both sizes, without an image and with its author,
// a video, a GIF, a sound and a file, the kinds mixed, sensitive media covered and shown, and the media viewer opened
// from it. Each post answers its actions as the timeline's do, in a column of the timeline's width.

import { useLocale } from "@my/design-system";
import { Preview, PreviewExpanded, type PreviewState } from "@my/design-system/preview";
import { pickerSections, pickerStrings, searchPicker } from "@my/design-system/samples";
import { breakpoint, rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState, type ReactNode } from "react";
import { statusPath } from "./paths";
import type { PostProps, TranslationFailure } from "./post";
import { mediaPosts, posts, translatedWhole, type TimelinePost } from "./post.samples";
import { SamplePost, type SampleOpen } from "./sample-post.samples";
import type { PostTranslation } from "./status";

const rem = (px: number) => `${px / rootPx}rem`;
const minute = 60_000;

const byId = (id: string) => {
  const post = [...posts, ...mediaPosts].find((p) => p.id === id);
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

// The images of the edited post: the one it was posted with, which the first edit took away, and the one the last
// edit added.
const [snowRidge = { alt: "", src: "" }, duskRidge = { alt: "", src: "" }] = byId("ravi-two").media ?? [];

// One's own post, edited twice: a day changed and its first image taken away, then the meeting place and another image
// added; in en and ja.
const editedOwn: Record<"en" | "ja", TimelinePost> = {
  en: {
    ...byId("own"),
    id: "edited-own",
    ago: 3 * hour,
    edited: 40 * minute,
    text: "Trying the new bakery on Elm Street on Saturday morning. Meet at nine by the fountain, anyone welcome!",
    media: [duskRidge],
    reactions: [],
    history: [
      {
        ago: 3 * hour,
        text: "Trying the new bakery on Elm Street tomorrow morning. Anyone want to come along?",
        media: [snowRidge],
      },
      { ago: 2 * hour, text: "Trying the new bakery on Elm Street on Saturday morning. Anyone want to come along?" },
    ],
  },
  ja: {
    ...byId("own"),
    id: "edited-own-ja",
    lang: "ja",
    ago: 3 * hour,
    edited: 40 * minute,
    text: "土曜の朝、駅前の新しいパン屋に行ってみます。9時に噴水の前で集合、誰でも歓迎です！",
    media: [duskRidge],
    reactions: [],
    history: [
      { ago: 3 * hour, text: "明日の朝、駅前の新しいパン屋に行ってみます。一緒に行く人いますか？", media: [snowRidge] },
      { ago: 2 * hour, text: "土曜の朝、駅前の新しいパン屋に行ってみます。一緒に行く人いますか？" },
    ],
  },
};

// Another's post from another server, edited once to fix a word.
const editedRemote: TimelinePost = {
  ...byId("lena-quiet"),
  id: "edited-remote",
  edited: 5 * hour,
  history: [
    { ago: byId("lena-quiet").ago, text: "Quiet morning. Tea, rain, and a book I have meant to finish since sprung." },
  ],
};

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
  ...shown
}: {
  post: TimelinePost;
  state?: PreviewState;
  translationAs?: Translation;
  votingAt?: readonly number[];
  open?: SampleOpen | undefined;
} & Pick<PostProps, "cardSize" | "defaultViewing" | "defaultMenuOpen">) {
  const lang = new Intl.Locale(useLocale().locale).language === "ja" ? "ja" : "en";
  return (
    <Column>
      <Preview state={state}>
        <SamplePost key={post.id} post={post} lang={lang} translationAs={translationAs} {...shown} />
      </Preview>
    </Column>
  );
}

/** Several sample posts one after another, a timeline's gap apart, in the column. */
function Several({
  posts: several,
  ...shown
}: { posts: readonly TimelinePost[] } & Pick<PostProps, "cardSize" | "defaultShown">) {
  const lang = new Intl.Locale(useLocale().locale).language === "ja" ? "ja" : "en";
  return (
    <Column>
      <div style={{ display: "grid", gap: rem(28) }}>
        {several.map((post) => (
          <SamplePost key={post.id} post={post} lang={lang} {...shown} />
        ))}
      </div>
    </Column>
  );
}

/** The sample posts of these ids, one after another. */
function Posts({ ids, ...shown }: { ids: readonly string[] } & Pick<PostProps, "cardSize" | "defaultShown">) {
  return <Several posts={ids.map(byId)} {...shown} />;
}

/**
 * The sensitive posts: one picture, four, behind a content warning too, an image with a sound, and a link card's
 * image.
 */
const sensitiveIds = ["sensitive", "sensitive-four", "sensitive-cw", "sensitive-mixed", "sensitive-card"] as const;

/** The reader's language, en or ja, from the toolbar's locale. */
const useLang = () => (new Intl.Locale(useLocale().locale).language === "ja" ? "ja" : "en");

/** The reader's edited post, in their language, with `open` over it at first. */
function EditedOwn({ open }: { open?: SampleOpen }) {
  return <Shown post={editedOwn[useLang()]} open={open} />;
}

/** Edited posts, one's own and another's, beside one never edited. */
function EditedPosts() {
  const lang = useLang();
  return (
    <Column>
      <div style={{ display: "grid", gap: rem(32) }}>
        {[editedOwn[lang], editedRemote, byId("alice-reply")].map((p) => (
          <SamplePost key={p.id} post={p} lang={lang} />
        ))}
      </div>
    </Column>
  );
}

const pickerTitle = { en: { title: "React", close: "Close" }, ja: { title: "リアクション", close: "閉じる" } } as const;

/**
 * A post with the emoji picker of its react action open, as the app opens it: a sheet under the tablet width, which
 * the app decides by the viewport, else a popover under the button. It reacts as the server would answer.
 */
function Reacting({ post }: { post: TimelinePost }) {
  const lang = new Intl.Locale(useLocale().locale).language === "ja" ? "ja" : "en";
  const sheet = !window.matchMedia(`(min-width: ${breakpoint.tablet}rem)`).matches;
  return (
    <Column>
      <SamplePost
        post={post}
        lang={lang}
        picker={{
          sections: pickerSections(lang),
          search: searchPicker,
          labels: pickerStrings[lang],
          sheet,
          title: pickerTitle[lang].title,
          closeLabel: pickerTitle[lang].close,
          defaultOpen: true,
        }}
      />
    </Column>
  );
}

/** A post in the other language than the reader's that translates into theirs, without images. */
function foreignIn(lang: "en" | "ja"): TimelinePost {
  const foreign = posts.find((p) => p.lang !== lang && p.translation?.[lang] !== undefined && p.media === undefined);
  if (foreign === undefined) throw new Error(`no sample post translates into ${lang}`);
  return foreign;
}

/** A post in the other language than the reader's, shown translated into theirs. */
function TranslatedPost({ translationAs }: { translationAs?: Translation }) {
  return <Shown post={{ ...foreignIn(useLang()), translated: true }} translationAs={translationAs} />;
}

/**
 * A post in the other language than the reader's, offered for translation: as it is, its translation on its way
 * (`translatingAt`: true, or the share of the browser's model downloaded).
 */
function ForeignPost({ translatingAt }: { translatingAt?: boolean | number }) {
  const lang = useLang();
  const post = { ...foreignIn(lang), translated: false };
  return (
    <Column>
      <SamplePost key={post.id} post={post} lang={lang} translatingAt={translatingAt} />
    </Column>
  );
}

// Each refusal, on a post of the other language than the reader's (the last one followers-only or direct): over the
// monthly limit with nothing else to try, and with the browser's translator to try; the provider down, where pressing
// again asks it again; a language it does not translate; the browser unable; and a visibility the provider may not
// receive, where the next press goes to the browser's translator.
const refusals: readonly TranslationFailure[] = [
  { reason: "limit", retry: false },
  { reason: "limit", retry: "browser" },
  { reason: "failed", retry: "server" },
  { reason: "language", retry: false },
  { reason: "browser", retry: false },
  { reason: "visibility", retry: "browser" },
];

/** The posts of the other language than the reader's that a translation was refused for, one per refusal. */
function RefusedPosts() {
  const lang = useLang();
  const plain = (p: TimelinePost) =>
    p.lang !== lang && p.own !== true && p.media === undefined && p.cw === undefined && p.quote === undefined;
  const open = posts.filter((p) => plain(p) && (p.visibility === "public" || p.visibility === "unlisted"));
  const closed = posts.find((p) => plain(p) && (p.visibility === "followers" || p.visibility === "direct"));
  if (closed === undefined || open.length === 0) throw new Error(`too few sample posts for ${lang}`);
  // The open posts in turn, as often as the refusals need them.
  const shown = [...refusals.slice(1).map((_, i) => open[i % open.length] ?? closed), closed];
  return (
    <Column>
      <div style={{ display: "grid", gap: rem(28) }}>
        {shown.map((post, i) => (
          <SamplePost
            key={`${post.id}-${String(i)}`}
            post={{ ...post, translated: false }}
            lang={lang}
            translationFails={refusals[i]}
          />
        ))}
      </div>
    </Column>
  );
}

/**
 * A post behind a content warning with a poll and an image, in the other language than the reader's: folded and
 * offered, folded and translated, and open and translated, its translate line in the warning's band.
 */
function TranslatedWhole() {
  const lang = useLang();
  const post = translatedWhole[lang === "ja" ? "en" : "ja"];
  return (
    <Column>
      <div style={{ display: "grid", gap: rem(28) }}>
        <SamplePost key="offered" post={post} lang={lang} />
        <SamplePost key="folded" post={{ ...post, translated: true }} lang={lang} />
        <PreviewExpanded>
          <SamplePost key="open" post={{ ...post, translated: true }} lang={lang} />
        </PreviewExpanded>
      </div>
    </Column>
  );
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
/**
 * The emoji picker of the react action, open: in a popover under it from the tablet width, in a sheet on a phone, the
 * reader's own reaction (🎉) knocked out in it. Picking an emoji moves the reaction there, or adds its chip.
 */
export const ReactPicker: Story = {
  ...fixed,
  render: () => (
    <Reacting
      post={{
        ...ownReaction,
        id: "own-party",
        reactions: [
          { emoji: "🎉", count: 4, mine: true },
          { emoji: ":sparkle:", count: 2 },
        ],
      }}
    />
  ),
};
/** The same on a post with no reactions yet: the first pick adds the first chip. */
export const ReactPickerFirst: Story = {
  ...fixed,
  render: () => <Reacting post={byId("lena-quiet")} />,
};
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
/** Offered: Translate under the text; pressing it shows the translation on its way, then the translation. */
export const Translatable: Story = { ...fixed, render: () => <ForeignPost /> };
/** The translation on its way: a spinner in the glyph's place. */
export const Translating: Story = { ...fixed, render: () => <ForeignPost translatingAt /> };
/** The browser's translator downloading its model before it translates, and how much has come. */
export const TranslationDownloading: Story = { ...fixed, render: () => <ForeignPost translatingAt={0.4} /> };
/**
 * Refused: over the monthly limit (with nothing else to try, and with the browser's translator), the provider down, a
 * language it does not translate, the browser unable, and a visibility the provider may not receive; Try again where
 * the provider may answer next time, Translate in this browser where the browser takes over.
 */
export const TranslationRefused: Story = { ...fixed, render: () => <RefusedPosts /> };
/**
 * Behind a content warning, with a poll and an image: the warning, the options and the alt text translated too, the
 * translate line in the warning's band, folded or open.
 */
export const TranslatedWarning: Story = { ...fixed, render: () => <TranslatedWhole /> };
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
/** The more menu of the reader's own post: its link and sharing, the bookmark, edit, then delete and redraft apart. */
export const OwnMenu: Story = { ...fixed, render: () => <Shown post={byId("own")} defaultMenuOpen /> };
/** The more menu of another's post from another server: its link, its original page, sharing, the bookmark. */
export const OthersMenu: Story = { ...fixed, render: () => <Shown post={byId("lena-quiet")} defaultMenuOpen /> };
/** The confirmation of Delete…, over the post. */
export const DeleteConfirmation: Story = { ...fixed, render: () => <EditedOwn open="delete" /> };
/** The confirmation of Delete and redraft…, over the post. */
export const RedraftConfirmation: Story = { ...fixed, render: () => <EditedOwn open="redraft" /> };
/** The confirmation of Delete and redraft… of the reader's post with a poll, whose votes it loses. */
export const RedraftPollConfirmation: Story = { ...fixed, render: () => <Shown post={ownPoll} open="redraft" /> };
/** The more menu of the reader's post with a poll: no edit, which the server refuses for a poll. */
export const OwnPollMenu: Story = { ...fixed, render: () => <Shown post={ownPoll} defaultMenuOpen /> };
/** Edited posts, one's own and another's, "edited" after their time, beside one never edited. */
export const Edited: Story = { ...fixed, render: () => <EditedPosts /> };
/**
 * The edit history of the reader's post, newest first: each version's words and images marked against the version
 * before it, an image taken away by the first edit and another added by the last.
 */
export const EditHistory: Story = { ...fixed, render: () => <EditedOwn open="history" /> };
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

/** The card of the post's first link, its image above its text (the large card, a new reader's preference). */
export const LinkCard: Story = { ...fixed, render: () => <Posts ids={["card-large", "card-ja"]} /> };
/** The small card: the image a square beside the text. */
export const LinkCardSmall: Story = {
  ...fixed,
  render: () => <Posts ids={["card-large", "card-ja"]} cardSize="small" />,
};
/** A card without an image, and one whose page names its author on the fediverse, in both sizes. */
export const LinkCardTextAndAuthor: Story = {
  ...fixed,
  render: () => (
    // The two sizes a feed's gap apart, as one post follows another.
    <div style={{ display: "grid", gap: rem(28) }}>
      <Posts ids={["card-text", "card-author"]} />
      <Posts ids={["card-text", "card-author"]} cardSize="small" />
    </div>
  ),
};
/** A video, its poster with its length; pressed, it plays in place. */
export const Video: Story = { ...fixed, render: () => <Shown post={byId("video")} /> };
/** A GIF: it loops, muted, and opens whole; still with reduced motion. */
export const Gif: Story = { ...fixed, render: () => <Shown post={byId("gif")} /> };
/** A sound, its description over its progress and length, and a file of a kind the client cannot show. */
export const AudioAndFile: Story = { ...fixed, render: () => <Posts ids={["audio", "unknown"]} /> };
/** An image, a GIF, a video and an image together. */
export const MixedMedia: Story = { ...fixed, render: () => <Shown post={byId("mixed")} /> };
/**
 * Sensitive media, covered until the reader shows it, under a line naming it: one picture, four, under a content
 * warning, an image with a sound (folded), and a card's image.
 */
export const Sensitive: Story = { ...fixed, render: () => <Posts ids={sensitiveIds} /> };
/** The same, shown: the content warning open and the media shown, the line ending in Hide. */
export const SensitiveShown: Story = { ...fixed, render: () => <Posts ids={sensitiveIds} defaultShown /> };
/** The viewer open from a post at its first picture: the post's media one at a time, whole. */
export const ViewerFromPost: Story = { ...fixed, render: () => <Shown post={byId("mixed")} defaultViewing={0} /> };
