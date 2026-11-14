// SPDX-License-Identifier: AGPL-3.0-only

// The sample data of the rough screens (#3047): a thread around a focused post, a reader's notifications of every
// kind they meet, and profiles in each state the reader meets them in (their own, another local account, a remote
// one, a locked one, one they block, one they mute). The posts are the timeline's fixtures (timeline/fixtures.ts),
// which the real post draws; the people are the timeline's.

import { images } from "../Avatars";
import { scenes, type TimelinePost } from "../timeline/fixtures";

const minute = 60_000;
const hour = 60 * minute;
const day = 24 * hour;

export type Who = { name: string; handle: string; avatar?: string };

export const me: Who = { name: "Kumi Ito", handle: "@kumi@my.example" };
const alice: Who = { name: "Alice Chen", handle: "@alice@example.social", avatar: images.dusk };
const mika: Who = { name: "佐藤 美香", handle: "@mika@mstdn.example.jp", avatar: images.snow };
const news: Who = { name: "Open Source Weekly", handle: "@news@social.example.org", avatar: images.logo };
const takuya: Who = { name: "たくや", handle: "@takuya@example.jp" };
const ravi: Who = { name: "Ravi", handle: "@ravi@fedi.example.net" };
const hana: Who = { name: "山田 はな", handle: "@hana@my.example" };
const lena: Who = { name: "Lena Novak", handle: "@lena@toot.example.eu" };
const ken: Who = { name: "Kenji 🍜 Watanabe", handle: "@kenji@my.example" };

const none = { replies: 0, boosts: 0, favourites: 0 };

/** A post of a sample: public, with no counts, unless it says otherwise. */
const post = (
  p: Partial<TimelinePost> & Pick<TimelinePost, "id" | "lang" | "author" | "ago" | "text">,
): TimelinePost => ({
  visibility: "public",
  counts: none,
  ...p,
});

// The post detail -------------------------------------------------------------------------------------------------

/** A post of the thread, how deep it replies under the focused post (0 for the ancestors), and what follows it. */
export type ThreadPost = { post: TimelinePost; depth: number; more?: number };

/** The thread's first post, then the ancestors the screen folds, then those it shows before the focused post. */
export const thread = {
  root: post({
    id: "t-root",
    lang: "en",
    author: alice,
    ago: 6 * hour,
    text: "Thinking of moving our book club off the big server. Anyone run a small one for a group of about twenty? What do you wish you had known? #selfhosting",
    counts: { replies: 9, boosts: 4, favourites: 21 },
  }),
  folded: 3,
  parents: [
    post({
      id: "t-a5",
      lang: "ja",
      author: mika,
      ago: 4 * hour,
      replyTo: "@ravi@fedi.example.net",
      text: "@ravi うちも同じでした。最初は 3 行だけ書いて、あとは実際に困ったときに足しています。",
      counts: { replies: 1, boosts: 0, favourites: 6 },
      translation: {
        en: "@ravi Same here. I wrote just three lines at first, and added more when something actually came up.",
      },
    }),
    post({
      id: "t-a6",
      lang: "en",
      author: alice,
      ago: 3 * hour,
      replyTo: "@mika@mstdn.example.jp",
      text: "@mika Three lines is about my attention span too. What were your three?",
      counts: { replies: 1, boosts: 0, favourites: 3 },
    }),
  ],
  focused: post({
    id: "t-focus",
    lang: "ja",
    author: mika,
    ago: 2 * hour + 40 * minute,
    replyTo: "@alice@example.social",
    text: "@alice これです :mycro:\n\n1. 相手を人として扱う\n2. 宣伝は自分のものだけ、ほどほどに\n3. 困ったら管理人にひとこと\n\n3 つ目がいちばん効きました。管理人も一人で抱えないこと。 #小さなサーバー",
    media: [{ alt: "サーバーのルールを書いた手書きのメモ。三つの項目が並んでいる", src: scenes.snow }],
    counts: { replies: 14, boosts: 12, favourites: 48 },
    favourited: true,
    reactions: [
      { emoji: "🙏", count: 9, mine: true },
      { emoji: ":blobcat:", count: 4 },
    ],
    translation: {
      en: "@alice Here they are :mycro:\n\n1. Treat others as people\n2. Promote only your own things, and sparingly\n3. If in trouble, tell the admin\n\nThe third worked best. The admin should not carry it alone either. #小さなサーバー",
    },
  }),
  /** The replies, depth first in posting order, as the API gives them. */
  replies: [
    {
      depth: 1,
      post: post({
        id: "t-r1",
        lang: "en",
        author: alice,
        ago: 2 * hour + 20 * minute,
        replyTo: "@mika@mstdn.example.jp",
        text: "@mika These are perfect. Stealing them for the book club.",
        counts: { replies: 1, boosts: 0, favourites: 7 },
      }),
    },
    {
      depth: 2,
      post: post({
        id: "t-r2",
        lang: "ja",
        author: mika,
        ago: 2 * hour + 10 * minute,
        replyTo: "@alice@example.social",
        text: "@alice どうぞどうぞ！",
        counts: { replies: 1, boosts: 0, favourites: 2 },
      }),
    },
    {
      depth: 3,
      post: post({
        id: "t-r3",
        lang: "en",
        author: alice,
        ago: 2 * hour,
        replyTo: "@mika@mstdn.example.jp",
        text: "@mika Thank you! I will report back after the first month.",
        counts: { replies: 1, boosts: 0, favourites: 3 },
      }),
    },
    {
      depth: 4,
      post: post({
        id: "t-r4",
        lang: "en",
        author: ravi,
        ago: 90 * minute,
        replyTo: "@alice@example.social",
        text: "@alice Please do, I want to hear how three lines of rules hold up against twenty readers.",
        counts: { replies: 1, boosts: 0, favourites: 2 },
      }),
    },
    {
      depth: 5,
      post: post({
        id: "t-r5",
        lang: "ja",
        author: mika,
        ago: 80 * minute,
        replyTo: "@ravi@fedi.example.net",
        text: "@ravi 私も楽しみにしてます",
        counts: { replies: 0, boosts: 0, favourites: 1 },
      }),
    },
    {
      depth: 1,
      post: post({
        id: "t-r6",
        lang: "ja",
        author: mika,
        ago: 70 * minute,
        replyTo: "@mika@mstdn.example.jp",
        text: "追記: 3 つ目は「困ったら一人で抱えない」に書き換えました。管理人に限らずみんなに。",
        counts: { replies: 0, boosts: 2, favourites: 11 },
      }),
    },
    {
      depth: 1,
      more: 8,
      post: post({
        id: "t-r7",
        lang: "ja",
        author: ken,
        ago: 50 * minute,
        replyTo: "@mika@mstdn.example.jp",
        text: "@mika ルールの 2 つ目、うちのサーバーでも使わせてください :blobcat:",
        counts: { replies: 8, boosts: 0, favourites: 4 },
      }),
    },
    {
      depth: 1,
      post: post({
        id: "t-r8",
        lang: "en",
        author: lena,
        ago: 20 * minute,
        replyTo: "@mika@mstdn.example.jp",
        text: "@mika Saving this thread. Deciding who moderates before day one is so underrated.",
        counts: { replies: 0, boosts: 0, favourites: 2 },
      }),
    },
  ] satisfies ThreadPost[],
  /** The replies the server holds beyond those it gave (it gives 60 at most). */
  later: 12,
  boosts: 12,
  quotes: 3,
  favourites: 48,
};

/** A post with no thread around it and no replies yet. */
export const lone = post({
  id: "t-lone",
  lang: "en",
  author: lena,
  ago: 5 * minute,
  text: "Finished the book I had meant to finish since spring. The last chapter was worth the wait.",
});

// The notifications -----------------------------------------------------------------------------------------------

export type NotifiedKind =
  | "mention"
  | "favourite"
  | "boost"
  | "reaction"
  | "quote"
  | "follow"
  | "follow_request"
  | "follow_accepted"
  | "status"
  | "quote_request"
  | "account_moved";

/** A notification: about a post shown whole, an act of an account, or a word from the server. */
export type Notice = { id: string; ago: number; unread: boolean } & (
  | { kind: "post"; type: "mention" | "quote" | "status"; who: Who; post: TimelinePost }
  | {
      kind: "act";
      type: Exclude<NotifiedKind, "mention" | "status">;
      who: Who;
      /** The reader's post it is about, as its first words. */
      about?: string;
      /** The emoji of a reaction. */
      emoji?: string;
      /** The account it moved to. */
      to?: Who;
    }
  | {
      kind: "server";
      type: "poll_closed" | "moderation_notice" | "report_resolved" | "announcement";
      title?: string;
      body?: string;
    }
);

const yours = "Trying out the new home timeline on my.example. Hello, fediverse!";

export const notices: Notice[] = [
  {
    id: "n-reply",
    ago: 4 * minute,
    unread: true,
    kind: "post",
    type: "mention",
    who: ken,
    post: post({
      id: "n-reply-post",
      lang: "ja",
      author: ken,
      ago: 4 * minute,
      replyTo: "@kumi@my.example",
      text: "@kumi ようこそ！困ったことがあったら気軽に聞いてください :wave:",
      counts: { replies: 0, boosts: 0, favourites: 1 },
    }),
  },
  { id: "n-fav", ago: 6 * minute, unread: true, kind: "act", type: "favourite", who: alice, about: yours },
  {
    id: "n-react",
    ago: 9 * minute,
    unread: true,
    kind: "act",
    type: "reaction",
    who: mika,
    emoji: "🎉",
    about: yours,
  },
  { id: "n-follow", ago: 15 * minute, unread: true, kind: "act", type: "follow", who: lena },
  { id: "n-request", ago: 22 * minute, unread: true, kind: "act", type: "follow_request", who: takuya },
  {
    id: "n-quote",
    ago: 40 * minute,
    unread: true,
    kind: "post",
    type: "quote",
    who: ravi,
    post: post({
      id: "n-quote-post",
      lang: "en",
      author: ravi,
      ago: 40 * minute,
      text: "Another small server joins the fediverse. Welcome!",
      quote: { lang: "en", author: me, ago: 2 * hour, visibility: "public", text: yours },
      counts: { replies: 0, boosts: 1, favourites: 5 },
    }),
  },
  { id: "n-boost", ago: 2 * hour, unread: false, kind: "act", type: "boost", who: news, about: yours },
  {
    id: "n-poll",
    ago: 3 * hour,
    unread: false,
    kind: "server",
    type: "poll_closed",
    body: "Tea or coffee for the meetup? — Tea 62% · Coffee 38% · 21 votes",
  },
  { id: "n-quote-request", ago: 5 * hour, unread: false, kind: "act", type: "quote_request", who: alice, about: yours },
  {
    id: "n-status",
    ago: 6 * hour,
    unread: false,
    kind: "post",
    type: "status",
    who: hana,
    post: post({
      id: "n-status-post",
      lang: "ja",
      author: hana,
      ago: 6 * hour,
      text: "来週の集まりの場所が決まりました。駅前の喫茶店の二階です :sparkle:",
      counts: { replies: 2, boosts: 1, favourites: 9 },
    }),
  },
  {
    id: "n-moderation",
    ago: 9 * hour,
    unread: false,
    kind: "server",
    type: "moderation_notice",
    body: "A post of yours was marked sensitive: its image shows without a content warning.",
  },
  { id: "n-accepted", ago: 20 * hour, unread: false, kind: "act", type: "follow_accepted", who: hana },
  {
    id: "n-moved",
    ago: day + 2 * hour,
    unread: false,
    kind: "act",
    type: "account_moved",
    who: ravi,
    to: { name: "Ravi", handle: "@ravi@new.example.net" },
  },
  {
    id: "n-announcement",
    ago: 2 * day,
    unread: false,
    kind: "server",
    type: "announcement",
    title: "Maintenance on Saturday, 03:00 UTC",
  },
  { id: "n-report", ago: 3 * day, unread: false, kind: "server", type: "report_resolved" },
];

/** What the reader's notification policy holds back, by sender (ADR-0158 §7). */
export const held = { people: 3, notifications: 5 };

// The profile -----------------------------------------------------------------------------------------------------

export type ProfileState = "own" | "local" | "remote" | "locked" | "blocked" | "muted";

export type ProfileSample = {
  who: Who;
  banner?: string;
  bio: string;
  /** The fields, name and value, the value as its author wrote it (the server renders it). */
  fields: [string, string][];
  joined: Date;
  following: number;
  followers: number;
  locked?: boolean;
  bot?: boolean;
  /** The role badges (local accounts only). */
  badges?: string[];
  featured: [string, number][];
  pinned: TimelinePost[];
  posts: TimelinePost[];
  /** The reader's relationship to the account. */
  rel: { following?: boolean; requested?: boolean; followedBy?: boolean; blocking?: boolean; muting?: boolean };
};

const authored = (who: Who, lang: "en" | "ja", items: [number, string, Partial<TimelinePost>?][]) =>
  items.map(([ago, text, rest], i) => post({ id: `${who.handle}-${i}`, lang, author: who, ago, text, ...rest }));

export const profiles: Record<ProfileState, ProfileSample> = {
  own: {
    who: me,
    banner: scenes.dawn,
    bio: "Reading, walking, and learning how a small server works. Here since its first week.",
    fields: [
      ["Pronouns", "she/her"],
      ["Blog", "https://kumi.example/notes"],
    ],
    joined: new Date("2026-10-02"),
    following: 42,
    followers: 18,
    featured: [
      ["books", 12],
      ["walks", 5],
    ],
    pinned: authored(me, "en", [
      [
        12 * day,
        "Hello! I'm Kumi. I read slow books and write short notes about them. #books",
        { counts: { replies: 6, boosts: 2, favourites: 30 } },
      ],
    ]).map((p) => ({ ...p, own: true })),
    posts: authored(me, "en", [
      [
        2 * minute,
        "Trying out the new home timeline on my.example. Hello, fediverse! :wave:",
        { counts: { replies: 1, boosts: 0, favourites: 2 } },
      ],
      [
        day,
        "A walk by the river, and the first cold morning of the year.",
        {
          media: [{ alt: "A river under a pale morning sky", src: scenes.sea }],
          counts: { replies: 0, boosts: 1, favourites: 8 },
        },
      ],
    ]).map((p) => ({ ...p, own: true })),
    rel: {},
  },
  local: {
    who: hana,
    banner: scenes.autumn,
    bio: "旅と写真と、ときどきサーバーの手伝い。週末は山にいます :sparkle:",
    fields: [
      ["写真", "https://hana.example.jp/photos"],
      ["住んでいるところ", "長野"],
      ["好きなもの", "朝焼け、古い地図、熱いお茶"],
    ],
    joined: new Date("2026-08-14"),
    following: 210,
    followers: 1_284,
    badges: ["Moderator"],
    featured: [
      ["写真", 86],
      ["山", 41],
      ["小さなサーバー", 9],
    ],
    pinned: authored(hana, "ja", [
      [
        30 * day,
        "このサーバーのモデレーターをしています。困ったことがあれば、いつでもメンションか DM でどうぞ。",
        { counts: { replies: 3, boosts: 14, favourites: 52 } },
      ],
    ]),
    posts: authored(hana, "ja", [
      [
        5 * hour,
        "三連休の旅行の写真をまとめて。どれも空がきれいだった :sparkle:",
        {
          media: [
            { alt: "朝焼けの山並み", src: scenes.dawn },
            { alt: "青い海と低い丘", src: scenes.sea },
          ],
          counts: { replies: 6, boosts: 9, favourites: 120 },
        },
      ],
      [
        day,
        "来週の集まりの場所が決まりました。駅前の喫茶店の二階です。",
        { counts: { replies: 2, boosts: 1, favourites: 9 } },
      ],
    ]),
    rel: { followedBy: true },
  },
  remote: {
    who: alice,
    banner: scenes.night,
    bio: "Runs a small server for a book club. Writes about self-hosting, soup, and slow mornings.",
    fields: [
      ["Website", "https://alice.example.social"],
      ["Book club", "Second Tuesdays"],
    ],
    joined: new Date("2023-04-20"),
    following: 380,
    followers: 2_910,
    featured: [
      ["selfhosting", 140],
      ["books", 64],
    ],
    pinned: [],
    posts: authored(alice, "en", [
      [
        3 * hour,
        "@mika Three lines is about my attention span too. What were your three?",
        { replyTo: "@mika@mstdn.example.jp", counts: { replies: 1, boosts: 0, favourites: 3 } },
      ],
      [
        20 * hour,
        "A few things I learned running a small server for a year: most of the work is talking to people, not to the machine. #selfhosting",
        { counts: { replies: 27, boosts: 310, favourites: 1_480 } },
      ],
    ]),
    rel: { following: true, followedBy: true },
  },
  locked: {
    who: takuya,
    bio: "映画と本の感想。ネタバレには CW を付けます。",
    fields: [["好きな監督", "たくさん"]],
    joined: new Date("2025-01-09"),
    following: 64,
    followers: 57,
    locked: true,
    featured: [],
    pinned: [],
    posts: authored(takuya, "ja", [
      [
        2 * hour,
        "最後の十分で全部ひっくり返るの、二回目に観るとちゃんと伏線があってすごい。",
        { cw: "映画の結末に触れています", counts: { replies: 0, boosts: 0, favourites: 3 } },
      ],
    ]),
    rel: {},
  },
  blocked: {
    who: ravi,
    bio: "Photos of ridges and the weather over them.",
    fields: [],
    joined: new Date("2024-06-30"),
    following: 120,
    followers: 450,
    featured: [],
    pinned: [],
    posts: [],
    rel: { blocking: true },
  },
  muted: {
    who: lena,
    banner: scenes.forest,
    bio: "Tea, rain, and books I have meant to finish since spring.",
    fields: [["Reading", "The Left Hand of Darkness"]],
    joined: new Date("2024-11-02"),
    following: 98,
    followers: 77,
    featured: [],
    pinned: [],
    posts: authored(lena, "en", [
      [7 * hour, "Quiet morning. Tea, rain, and a book I have meant to finish since spring."],
    ]),
    rel: { muting: true, following: true },
  },
};
