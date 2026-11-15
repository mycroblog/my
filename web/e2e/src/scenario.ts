// SPDX-License-Identifier: AGPL-3.0-only

// The scenario (ADR-0015 §5): the people, posts and relationships of a small server in use, as data only. The seed
// (seed/seed.ts) applies it to a running `my serve` in the order of this file; the E2E tests sign in as its people;
// the fake API (fake/api.ts) answers the screens' stories from it. It holds what the screens of the web client show
// today (the home timeline, the composer with its drafts and scheduled posts, a post, its edit history and its thread,
// an article as its card and on its page,
// the notifications, a profile, a hashtag, the reader's lists and bookmarks, the sign-in with a password, a two-factor
// code or a passkey, the consent to an app, and the shell's announcements and upcoming events), and grows with the
// screens. It names no ID and no time (an event's day is counted from the seed): the
// server decides them, so nothing that reads the scenario depends on them. Every item has a key, which the
// seed sends as `Idempotency-Key: demo:<key>`. The passwords are fixed, which a server takes from the seed only in
// the `development` profile of a test build (ADR-0015 §6). Links use the reserved `.test` and `.invalid` hosts.

import type { Drawing, RGB } from "./drawing";

/** A person of the scenario, who signs in with `email` and `password`. */
export type Person = {
  username: string;
  email: string;
  password: string;
  /** `admin` holds the built-in admin role, `moderator` the built-in moderator role; the others neither. */
  staff?: "admin" | "moderator";
  displayName: string;
  /** The profile text, as its author writes it. */
  note: string;
  /** The profile's fields, name and value. */
  fields: readonly (readonly [string, string])[];
  /** The UI locale and the language of the person's posts. */
  locale: "en" | "ja";
  /** Whether a follow waits for the person's approval. */
  locked?: boolean;
  avatar: Drawing;
  header?: Drawing;
  /** Signs in with a code of an authenticator app after the password (TOTP, RFC 6238). */
  twoFactor?: boolean;
  /** Has a passkey, which signs in without the password. */
  passkey?: boolean;
};

/** An image attached to a post: its file name, alt text, and drawing. */
export type Image = { name: string; description: string; drawing: Drawing };

/**
 * An attachment other than an image: a video (its poster drawn), a GIF (a short silent loop, its first frame drawn),
 * a sound, or a file of a kind no client shows, with its alt text (empty for none) and a video's or a sound's length
 * in seconds. Only the fake API answers them: the server takes only real video and sound files, which the repository
 * does not hold, so the seed leaves out a post that has them.
 */
export type Attachment =
  | { kind: "video" | "gifv"; name: string; description: string; poster: Drawing; seconds?: number }
  | { kind: "audio"; name: string; description: string; seconds: number }
  | { kind: "unknown"; name: string; description: string };

/**
 * The card of a post's first link, as the server would make it from the page. Only the fake API answers it: the
 * server fetches a page itself, and the scenario's pages are on hosts that do not exist.
 */
export type LinkCard = {
  title: string;
  description: string;
  site: string;
  language?: string;
  image?: { description?: string; drawing: Drawing };
  /** The username of the person the page names as its author, as their server confirms them. */
  author?: string;
};

/** Who may see a post (the API's Visibility). */
export type Visibility = "public" | "local" | "unlisted" | "private" | "direct";

/** A post of a person; `replyTo` and `quote` name earlier posts by key, `@username` in the text mentions a person. */
export type Post = {
  key: string;
  author: string;
  text: string;
  visibility: Visibility;
  /** The BCP 47 tag of the text's language. */
  language: string;
  replyTo?: string;
  quote?: string;
  /** The content warning. */
  spoiler?: string;
  images?: readonly Image[];
  /** Its attachments other than images (only the fake's), after its images. */
  attachments?: readonly Attachment[];
  /** Its media are sensitive: covered until the reader shows them. */
  sensitive?: boolean;
  /** The card of its first link (only the fake's). */
  card?: LinkCard;
  /** Pinned to its author's profile. */
  pinned?: boolean;
  /** Its poll, which ends `days` days after the seed posts it. */
  poll?: Poll;
  /**
   * What it said before its author edited it, oldest first: the seed posts the first, then edits it to each of the
   * others, and last to `text`, what it says now; each version stays in its edit history.
   */
  earlier?: readonly string[];
  /** It is an article (ADR-0160): `text` is its body in Markdown, under this title, summary and cover. */
  article?: Article;
};

/** What makes a post an article: its title, its summary, and its cover. */
export type Article = { title: string; summary: string; cover?: Image };

/** A poll of a post: its options, whether a vote may choose several, and how many days it takes votes. */
export type Poll = { options: readonly string[]; multiple?: boolean; days: number };

/**
 * What a person does to a post: a favourite, a boost, an emoji reaction (`:shortcode:` for a custom one), a bookmark,
 * a vote in its poll for the options of `choices` (their indexes).
 */
export type Interaction =
  | { kind: "favourite"; by: string; post: string }
  | { kind: "boost"; by: string; post: string }
  | { kind: "reaction"; by: string; post: string; emoji: string }
  | { kind: "bookmark"; by: string; post: string }
  | { kind: "vote"; by: string; post: string; choices: readonly number[] };

/** A follow; the follow of a locked account waits as a request until the followee accepts it. */
export type Follow = { follower: string; followee: string; accepted?: boolean };

/** A block or a mute of one person by another. */
export type Restriction = { kind: "block" | "mute"; by: string; target: string };

/** A custom emoji of the server, drawn by code, with the picker's category and the other names it finds it by. */
export type CustomEmoji = { shortcode: string; drawing: Drawing; category?: string; aliases?: readonly string[] };

/** An OAuth app a person registered, which other people consent to at /oauth/authorize. */
export type App = {
  key: string;
  owner: string;
  name: string;
  website: string;
  redirectUris: readonly string[];
  scope: string;
};

const light = (c: RGB): RGB => [
  Math.round(c[0] + (255 - c[0]) * 0.6),
  Math.round(c[1] + (255 - c[1]) * 0.6),
  Math.round(c[2] + (255 - c[2]) * 0.6),
];

/** A person's avatar: a figure in white on a gradient of their color. */
const portrait = (color: RGB): Drawing => ({
  width: 256,
  height: 256,
  top: light(color),
  bottom: color,
  shapes: [
    { kind: "circle", x: 128, y: 104, r: 52, color: [255, 255, 255] },
    { kind: "circle", x: 128, y: 250, r: 96, color: [255, 255, 255] },
  ],
});

/** A landscape: a sky from `top` to `bottom`, a sun, and three ridges darkening toward the front. */
const landscape = (top: RGB, bottom: RGB, sun: RGB, ridges: readonly [RGB, RGB, RGB]): Drawing => ({
  width: 640,
  height: 400,
  top,
  bottom,
  shapes: [
    { kind: "circle", x: 448, y: 150, r: 48, color: sun },
    { kind: "wave", y: 230, amplitude: 34, period: 420, phase: 0.6, color: ridges[0] },
    { kind: "wave", y: 290, amplitude: 22, period: 300, phase: 2.1, color: ridges[1] },
    { kind: "wave", y: 345, amplitude: 14, period: 520, phase: 4, color: ridges[2] },
  ],
});

const dawn = landscape(
  [246, 199, 161],
  [251, 230, 207],
  [255, 244, 224],
  [
    [201, 143, 123],
    [154, 106, 99],
    [93, 68, 72],
  ],
);
const sea = landscape(
  [127, 179, 213],
  [214, 236, 245],
  [255, 251, 232],
  [
    [91, 143, 168],
    [63, 111, 134],
    [42, 77, 95],
  ],
);

/** A round loaf on a board, seen from above. */
const loaf: Drawing = {
  width: 480,
  height: 480,
  top: [138, 90, 60],
  bottom: [120, 76, 50],
  shapes: [
    { kind: "rect", x: 40, y: 40, w: 400, h: 400, color: [196, 150, 104] },
    { kind: "circle", x: 240, y: 240, r: 150, color: [160, 98, 52] },
    { kind: "circle", x: 240, y: 240, r: 128, color: [214, 160, 98] },
    { kind: "rect", x: 150, y: 232, w: 180, h: 16, color: [236, 214, 176] },
  ],
};

const password = (username: string) => `demo-password-${username}`;
const person = (p: Omit<Person, "email" | "password">): Person => ({
  ...p,
  email: `${p.username}@demo.test`,
  password: password(p.username),
});

export const people: readonly Person[] = [
  person({
    username: "hana",
    staff: "admin",
    displayName: "Hana 花",
    note: "このデモサーバーの管理者です。困ったことがあれば気軽にどうぞ。",
    fields: [["役割", "管理者"]],
    locale: "ja",
    avatar: portrait([214, 92, 120]),
    header: dawn,
  }),
  person({
    username: "marco",
    staff: "moderator",
    displayName: "Marco",
    note: "Moderator. I read every report, usually within a day.",
    fields: [],
    locale: "en",
    avatar: portrait([64, 120, 200]),
  }),
  person({
    username: "yuki",
    displayName: "Yuki 雪",
    note: "写真と散歩。週末は山にいます。",
    fields: [["写真", "https://yuki.photos.test"]],
    locale: "ja",
    avatar: portrait([90, 170, 210]),
    header: sea,
  }),
  person({
    username: "sam",
    displayName: "Sam Rivera",
    note: "Writes about bread, bikes, and the occasional bug.",
    fields: [["Blog", "https://sam.blog.test"]],
    locale: "en",
    avatar: portrait([230, 150, 60]),
  }),
  person({
    username: "layla",
    displayName: "ليلى Layla",
    note: "Translator. أكتب بالعربية والإنجليزية.",
    fields: [],
    locale: "en",
    avatar: portrait([120, 90, 190]),
  }),
  person({
    username: "ren",
    displayName: "Ren 蓮",
    note: "鍵アカウントです。フォローは承認制です。",
    fields: [],
    locale: "ja",
    locked: true,
    avatar: portrait([70, 160, 110]),
  }),
  person({
    username: "kai",
    displayName: "Kai",
    note: "Signs in with a code from an authenticator app.",
    fields: [],
    locale: "en",
    avatar: portrait([150, 150, 150]),
    twoFactor: true,
  }),
  person({
    username: "mio",
    displayName: "Mio 澪",
    note: "パスキーでログインしています。",
    fields: [],
    locale: "ja",
    avatar: portrait([240, 130, 90]),
    passkey: true,
  }),
  person({
    username: "quickdeals",
    displayName: "QuickDeals",
    note: "Best deals every day!!!",
    fields: [],
    locale: "en",
    avatar: portrait([200, 200, 70]),
  }),
];

/** The person the seed acts as for the server's settings. */
export const admin = "hana";

/** The instance texts and settings, set with `my instance set` (docs/operations/instance-settings.md). */
export const instance: Readonly<Record<string, string>> = {
  name: "mycroblog demo",
  short_description: "A small community to try mycroblog with. 小さなコミュニティで mycroblog を試せます。",
  federation_mode: "off",
  default_locale: "en",
  contact_account: admin,
  contact_email: "contact@demo.test",
  // The fake provider of the test build (MY_TRANSLATOR_FAKE): "[ja] Hello" for "Hello", a provider that is down for a
  // text with "[translation-fails]". It receives the default visibilities, not private or direct posts. One person
  // may send 1000 characters a month, which two posts of the default maximum length (500) use up.
  translation_providers: "fake",
  translation_user_monthly_characters: "1000",
  // A post may be scheduled a minute ahead, so that the schedule the seed makes to fail fails soon after it.
  scheduled_post_min_lead_minutes: "1",
};

/** The server's custom emoji, added with `my admin emoji create`. */
export const emojis: readonly CustomEmoji[] = [
  {
    shortcode: "wave",
    drawing: {
      width: 64,
      height: 64,
      top: [255, 214, 102],
      bottom: [255, 190, 64],
      shapes: [
        { kind: "circle", x: 22, y: 26, r: 5, color: [107, 74, 31] },
        { kind: "circle", x: 42, y: 26, r: 5, color: [107, 74, 31] },
        { kind: "rect", x: 20, y: 40, w: 24, h: 5, color: [107, 74, 31] },
      ],
    },
  },
  {
    shortcode: "sparkle",
    drawing: {
      width: 64,
      height: 64,
      top: [122, 92, 255],
      bottom: [76, 195, 138],
      shapes: [
        { kind: "circle", x: 32, y: 32, r: 14, color: [255, 211, 77] },
        { kind: "circle", x: 50, y: 14, r: 5, color: [255, 236, 170] },
        { kind: "circle", x: 14, y: 50, r: 4, color: [255, 236, 170] },
      ],
    },
  },
  {
    shortcode: "teacup",
    category: "Sunday tea",
    aliases: ["ocha", "tea"],
    drawing: {
      width: 64,
      height: 64,
      top: [214, 240, 222],
      bottom: [150, 200, 170],
      shapes: [
        { kind: "rect", x: 16, y: 26, w: 30, h: 22, color: [255, 255, 255] },
        { kind: "circle", x: 48, y: 36, r: 6, color: [255, 255, 255] },
        { kind: "rect", x: 20, y: 30, w: 22, h: 6, color: [120, 170, 90] },
      ],
    },
  },
];

/** The follows, in order; a follow of a locked account without `accepted` stays a request. */
export const follows: readonly Follow[] = [
  ...["marco", "yuki", "sam", "layla", "ren", "kai", "mio"].map((follower) => ({ follower, followee: "hana" })),
  ...["marco", "yuki", "sam", "layla"].map((followee) => ({ follower: "hana", followee })),
  { follower: "yuki", followee: "sam" },
  { follower: "sam", followee: "yuki" },
  { follower: "yuki", followee: "layla" },
  { follower: "layla", followee: "yuki" },
  { follower: "layla", followee: "sam" },
  { follower: "marco", followee: "sam" },
  { follower: "kai", followee: "yuki" },
  { follower: "mio", followee: "yuki" },
  { follower: "hana", followee: "ren", accepted: true },
  { follower: "yuki", followee: "ren", accepted: true },
  { follower: "sam", followee: "ren" },
];

/**
 * A list a person made of people they follow (ADR-0013 §10, §11), which only they see: its name and its members. The
 * seed finds it again by its name.
 */
export type PersonList = { key: string; owner: string; name: string; members: readonly string[] };

/** The lists, oldest first. */
export const lists: readonly PersonList[] = [
  { key: "yuki-bread", owner: "yuki", name: "Bread and coffee", members: ["sam", "hana"] },
  { key: "yuki-translators", owner: "yuki", name: "翻訳の仲間", members: ["layla"] },
  {
    key: "yuki-long",
    owner: "yuki",
    name: "A list with a name long enough to wrap on a phone",
    members: ["sam", "layla", "ren", "hana"],
  },
];

/** The posts, oldest first. */
export const posts: readonly Post[] = [
  // An article and its reply come first, older than every other post, so that they push none of those off the first
  // page of a timeline.
  {
    key: "sam-article",
    author: "sam",
    language: "en",
    visibility: "public",
    article: {
      title: "A sourdough starter in seven days",
      summary: "What I fed it, when it rose, and the three mistakes that cost me a week. A diary of a jar of flour.",
      cover: { name: "starter.png", description: "A round loaf on a wooden board", drawing: loaf },
    },
    text: [
      "Seven days ago I mixed flour and water in a jar. This is what happened next, day by day.",
      "# The first three days",
      "Nothing, then bubbles, then a smell like **old cheese**. That smell is normal: keep feeding it.",
      "- Day 1: 50 g flour, 50 g water\n- Day 2: the same, at the same hour\n- Day 3: half of it out, then the same",
      "> A starter is ready when it doubles within six hours of a feed.",
      "# What I fed it",
      "| Day | Flour | Water |\n| --- | ---: | ---: |\n| 1 | 50 g | 50 g |\n| 4 | 100 g | 100 g |\n| 7 | 100 g | 80 g |",
      "## The mistakes",
      "Too warm a kitchen, too cold a fridge, and a lid screwed tight. The map of the shop I bought the flour from: ![the shop on a map](https://maps.test/flour.png)",
      "```\nfeed: 1 starter : 1 flour : 1 water\n```",
      "---",
      "Next week: the first loaf.",
    ].join("\n\n"),
  },
  {
    key: "yuki-article-reply",
    author: "yuki",
    language: "ja",
    visibility: "public",
    replyTo: "sam-article",
    text: "@sam 三日目のにおい、うちでも同じでした。続きを楽しみにしています",
  },
  {
    key: "hana-welcome",
    author: "hana",
    language: "ja",
    visibility: "public",
    text: "mycroblog のデモサーバーへようこそ :wave:\n\n困ったことがあれば @marco か私に気軽にどうぞ。 #はじめまして",
    pinned: true,
  },
  {
    key: "marco-hello",
    author: "marco",
    language: "en",
    visibility: "public",
    text: "Hi all, I'm one of the moderators here. Reports reach me within a day. The rules: https://demo.test/about :sparkle:",
  },
  {
    key: "yuki-ridge",
    author: "yuki",
    language: "ja",
    visibility: "public",
    text: "今朝の尾根。三時に起きた甲斐がありました #山 #写真",
    images: [{ name: "ridge.png", description: "朝焼けの山並み。手前に三つの尾根が重なっている", drawing: dawn }],
    pinned: true,
  },
  {
    key: "sam-ridge-reply",
    author: "sam",
    language: "en",
    visibility: "public",
    replyTo: "yuki-ridge",
    text: "@yuki That light! Which trail is this?",
  },
  {
    key: "yuki-ridge-reply",
    author: "yuki",
    language: "ja",
    visibility: "public",
    replyTo: "sam-ridge-reply",
    text: "@sam 北側の尾根道です。駐車場から二時間くらい。",
    earlier: ["@sam 南側の尾根道です。", "@sam 北側の尾根道です。"],
  },
  {
    key: "sam-ridge-reply-2",
    author: "sam",
    language: "en",
    visibility: "public",
    replyTo: "yuki-ridge-reply",
    text: "@yuki Adding it to my list for the spring.",
  },
  {
    key: "hana-ridge-reply",
    author: "hana",
    language: "ja",
    visibility: "public",
    replyTo: "yuki-ridge",
    text: "@yuki きれい！次の集まりのポスターに使わせてほしいです",
  },
  {
    key: "sam-bread",
    author: "sam",
    language: "en",
    visibility: "public",
    text: "Day 3 of the sourdough experiment. The crumb is finally open enough to call it bread. #baking",
    images: [
      { name: "loaf.png", description: "A round loaf of sourdough on a wooden board, seen from above", drawing: loaf },
    ],
  },
  {
    key: "sam-poll",
    author: "sam",
    language: "en",
    visibility: "public",
    text: "Bread for Saturday's meetup: which loaf should I bake? #baking",
    poll: { options: ["Sourdough", "Rye", "Focaccia"], days: 7 },
  },
  {
    key: "layla-rtl",
    author: "layla",
    language: "ar",
    visibility: "public",
    text: "صباح الخير! أترجم اليوم فصلاً جديداً من رواية قصيرة.",
  },
  {
    key: "layla-jokes",
    author: "layla",
    language: "en",
    visibility: "public",
    text: "Translating a short novel this month. The hardest part is always the jokes. https://layla.notes.test/translating-jokes",
    card: {
      title: "Why jokes are the hardest thing to translate",
      description: "A pun lives in one language. Notes from a month of carrying a short novel's humour into English.",
      site: "Layla's notes",
      language: "en",
      image: { description: "Two speech bubbles over a calm sea", drawing: sea },
      author: "layla",
    },
  },
  {
    key: "yuki-tide",
    author: "yuki",
    language: "ja",
    visibility: "public",
    text: "夕方の潮が満ちてくるところ。九十秒だけ",
    attachments: [
      {
        kind: "video",
        name: "tide.mp4",
        description: "夕暮れの浜に波が寄せる。空が橙から紫に変わっていく",
        poster: dawn,
        seconds: 92,
      },
    ],
  },
  {
    key: "sam-gif",
    author: "sam",
    language: "en",
    visibility: "public",
    text: "Me when the loaf finally rises",
    attachments: [{ kind: "gifv", name: "rise.mp4", description: "", poster: loaf }],
  },
  {
    key: "marco-rain",
    author: "marco",
    language: "en",
    visibility: "public",
    text: "Rain on the tin roof of the cabin last night. Headphones recommended.",
    attachments: [
      { kind: "audio", name: "rain.mp3", description: "Field recording: steady rain on a tin roof", seconds: 194 },
    ],
  },
  {
    key: "hana-slides",
    author: "hana",
    language: "ja",
    visibility: "public",
    text: "先週の勉強会のスライドです",
    attachments: [{ kind: "unknown", name: "slides.key", description: "" }],
  },
  {
    key: "yuki-walk",
    author: "yuki",
    language: "ja",
    visibility: "public",
    text: "週末の散歩、三枚。朝の山、昼の海、帰りに買ったパン",
    images: [
      { name: "walk-ridge.png", description: "朝の山並み。尾根が三つ重なっている", drawing: dawn },
      { name: "walk-sea.png", description: "昼の海。空は淡い青", drawing: sea },
      { name: "walk-loaf.png", description: "丸いパンを上から見たところ", drawing: loaf },
    ],
  },
  {
    key: "yuki-sensitive",
    author: "yuki",
    language: "ja",
    visibility: "public",
    text: "転んで擦りむいた膝。苦手な人は開かないでください",
    sensitive: true,
    images: [{ name: "knee.png", description: "擦りむいた膝の傷", drawing: dawn }],
  },
  {
    key: "sam-spoiler",
    author: "sam",
    language: "en",
    visibility: "public",
    spoiler: "The ending of the film we watched",
    text: "The phone call in the first scene was the whole plot all along.",
  },
  {
    key: "ren-followers",
    author: "ren",
    language: "ja",
    visibility: "private",
    text: "フォロワー限定で。来週の集まり、駅前の喫茶店の二階に決まりました。",
  },
  {
    key: "hana-unlisted",
    author: "hana",
    language: "ja",
    visibility: "unlisted",
    text: "メンテナンスのお知らせ: 土曜の朝 3 時から 10 分ほど止まります。",
  },
  {
    key: "marco-local",
    author: "marco",
    language: "en",
    visibility: "local",
    text: "For this server only: the meetup on Saturday starts at ten.",
  },
  {
    key: "sam-direct",
    author: "sam",
    language: "en",
    visibility: "direct",
    text: "@yuki Are you coming to the meetup on Saturday? I can bring the bread.",
  },
  {
    key: "yuki-quote",
    author: "yuki",
    language: "ja",
    visibility: "public",
    quote: "hana-welcome",
    text: "参加しました！よろしくお願いします :sparkle:",
  },
  {
    key: "kai-hello",
    author: "kai",
    language: "en",
    visibility: "public",
    text: "Just joined. Signing in takes a code from my phone now, and that's fine.",
  },
  {
    key: "mio-hello",
    author: "mio",
    language: "ja",
    visibility: "public",
    text: "はじめまして。パスワードなしでログインできるのは楽ですね",
  },
  {
    key: "quickdeals-spam",
    author: "quickdeals",
    language: "en",
    visibility: "public",
    text: "Huge discounts today only!!! https://deals.invalid/now",
  },
];

/** The favourites, boosts, reactions and bookmarks, in order. */
export const interactions: readonly Interaction[] = [
  ...["sam", "hana", "layla", "marco"].map((by) => ({ kind: "favourite" as const, by, post: "yuki-ridge" })),
  { kind: "boost", by: "marco", post: "yuki-ridge" },
  { kind: "reaction", by: "sam", post: "yuki-ridge", emoji: "🎉" },
  { kind: "reaction", by: "layla", post: "yuki-ridge", emoji: ":sparkle:" },
  { kind: "favourite", by: "yuki", post: "sam-bread" },
  { kind: "favourite", by: "hana", post: "sam-bread" },
  { kind: "boost", by: "hana", post: "sam-bread" },
  { kind: "reaction", by: "hana", post: "sam-bread", emoji: "😋" },
  { kind: "bookmark", by: "yuki", post: "sam-bread" },
  { kind: "favourite", by: "yuki", post: "hana-welcome" },
  { kind: "favourite", by: "sam", post: "hana-welcome" },
  { kind: "boost", by: "sam", post: "hana-welcome" },
  { kind: "favourite", by: "marco", post: "layla-rtl" },
  { kind: "reaction", by: "yuki", post: "layla-rtl", emoji: "🙏" },
  { kind: "favourite", by: "yuki", post: "layla-jokes" },
  { kind: "boost", by: "yuki", post: "layla-jokes" },
  { kind: "vote", by: "hana", post: "sam-poll", choices: [0] },
  { kind: "vote", by: "marco", post: "sam-poll", choices: [2] },
  { kind: "vote", by: "layla", post: "sam-poll", choices: [0] },
];

/** The blocks and mutes. */
export const restrictions: readonly Restriction[] = [
  { kind: "block", by: "yuki", target: "quickdeals" },
  { kind: "mute", by: "yuki", target: "kai" },
];

/** The OAuth apps the people registered. */
export const apps: readonly App[] = [
  {
    key: "tidepool",
    owner: "sam",
    name: "Tidepool",
    website: "https://tidepool.test",
    redirectUris: ["https://tidepool.test/callback"],
    scope: "read write:statuses",
  },
];

/**
 * An announcement of the server to every local user, which the administrator publishes in this order (the last is
 * the newest); `readBy` names who marked it read.
 */
export type ServerAnnouncement = { key: string; title: string; text: string; readBy?: readonly string[] };

/** The server's announcements, which the column beside the timeline shows and a line above the home timeline counts. */
export const announcements: readonly ServerAnnouncement[] = [
  {
    key: "welcome",
    title: "Welcome to the demo server: read the rules before you post",
    text: "This server is a place to try mycroblog. Be kind, and mark what others may not want to see.",
    readBy: ["yuki"],
  },
  {
    key: "emoji-requests",
    title: "Custom emoji requests are open",
    text: "Anyone can now ask for a new custom emoji from the settings. The admins review each request.",
  },
  {
    key: "maintenance",
    title: "Scheduled maintenance on Sunday, 03:00–04:00 UTC",
    text: "The server will be down for about an hour while we move the database to a larger disk.",
  },
];

/**
 * An event a person organizes, `inDays` days after the seed runs, at `hour` o'clock in its time zone, for
 * `hours` hours.
 */
export type ServerEventPlan = {
  key: string;
  organizer: string;
  title: string;
  description: string;
  timeZone: string;
  inDays: number;
  hour: number;
  hours: number;
};

/** The upcoming events, which the column beside the timeline shows, the nearest first. */
export const events: readonly ServerEventPlan[] = [
  {
    key: "meetup",
    organizer: "hana",
    title: "Fediverse meetup in Tokyo",
    description: "An evening of talks and snacks for everyone on the fediverse.",
    timeZone: "Asia/Tokyo",
    inDays: 6,
    hour: 19,
    hours: 3,
  },
  {
    key: "office-hours",
    organizer: "marco",
    title: "Moderators' office hours",
    description: "Ask the moderators anything about the rules and how reports are handled.",
    timeZone: "Europe/Lisbon",
    inDays: 9,
    hour: 12,
    hours: 1,
  },
];

/**
 * A draft a person keeps on the server (ADR-0156), newest last. `replyTo` names the post it answers by key. `schedule`
 * schedules it `inDays` days after the seed runs, at `hour` o'clock UTC. `fails` makes its schedule fail: the seed
 * writes it as a reply to a post of its author's of that text, schedules it at the nearest time the server takes
 * (`scheduled_post_min_lead_minutes`), and deletes that post, so that it is not published for its reply
 * (`reply_unavailable`) about a minute after the seed, and its author is notified.
 */
export type DraftPlan = {
  key: string;
  author: string;
  text: string;
  language: string;
  spoiler?: string;
  replyTo?: string;
  images?: readonly Image[];
  poll?: Poll;
  schedule?: { inDays: number; hour: number };
  fails?: { post: string };
  /** It is the draft of an article (ADR-0160 §2): `text` is its body in Markdown, under this title and summary. */
  article?: { title: string; summary: string; tags: readonly string[] };
};

/** The drafts and scheduled posts, which the composer's list shows. */
export const drafts: readonly DraftPlan[] = [
  {
    key: "yuki-draft-meetup",
    author: "yuki",
    text: "Which day works for the next photo walk?",
    language: "en",
    poll: { options: ["Saturday morning", "Sunday afternoon"], days: 1 },
  },
  {
    key: "yuki-draft-finale",
    author: "yuki",
    text: "I did not expect the lighthouse keeper to be the narrator all along.",
    language: "en",
    spoiler: "Spoilers for the season finale",
  },
  {
    key: "yuki-draft-ferry",
    author: "yuki",
    text: "Booked two seats. See you at the pier!",
    language: "en",
    fails: { post: "Who wants to take the early ferry to the island on Saturday?" },
  },
  {
    key: "yuki-scheduled-weekly",
    author: "yuki",
    text: "Weekly photo thread is open. Post one picture from your week, no captions needed.",
    language: "en",
    schedule: { inDays: 4, hour: 3 },
  },
  {
    key: "yuki-scheduled-bread",
    author: "yuki",
    text: "@sam Tried your overnight proof: the loaf finally rose. Thank you!",
    language: "en",
    replyTo: "sam-bread",
    schedule: { inDays: 2, hour: 0 },
  },
  {
    key: "yuki-draft-river",
    author: "yuki",
    text: "Walked along the river this morning. The first frost turned every leaf on the path silver.",
    language: "en",
  },
  {
    key: "yuki-article-trail",
    author: "yuki",
    text:
      "## Before you go\n\nThe trail starts behind the old station. Bring **water** and a warm layer: the ridge is " +
      "windy even in summer.\n\n## The route\n\n- Up the stone steps to the shrine\n- Along the ridge to the lookout\n" +
      "- Down through the cedar forest\n\n> The lookout is the best place for the morning light.",
    language: "en",
    article: {
      title: "A photo walk along the north ridge",
      summary: "",
      tags: ["photowalk", "hiking"],
    },
  },
  {
    key: "yuki-article-scheduled",
    author: "yuki",
    text:
      "## What we did\n\nTwelve of us met at the pier and walked the river path to the old mill.\n\n" +
      "## Next time\n\nWe start an hour earlier, for the light.",
    language: "en",
    schedule: { inDays: 3, hour: 1 },
    article: {
      title: "Notes from the November photo walk",
      summary: "What we saw, what we learned, and when we meet next.",
      tags: ["photowalk"],
    },
  },
];

/** When the scheduled draft `plan` is to be published, counted from `now` (ms since the Unix epoch). */
export function draftSchedule(plan: { inDays: number; hour: number }, now: number): Date {
  const day = new Date(now + plan.inDays * 86_400_000);
  day.setUTCHours(plan.hour, 0, 0, 0);
  return day;
}

/** How far `zone`'s clock is ahead of UTC at the instant `ms`, in ms. */
function zoneOffset(ms: number, zone: string): number {
  const local = new Date(ms).toLocaleString("sv-SE", { timeZone: zone }).replace(" ", "T");
  return Date.parse(`${local}Z`) - ms;
}

/** When the event `plan` starts, counted from `now` (ms since the Unix epoch). */
export function eventStart(plan: ServerEventPlan, now: number): Date {
  const day = new Date(now + plan.inDays * 86_400_000).toLocaleDateString("sv-SE", { timeZone: plan.timeZone });
  const asUtc = Date.parse(`${day}T${String(plan.hour).padStart(2, "0")}:00:00Z`);
  return new Date(asUtc - zoneOffset(asUtc, plan.timeZone));
}

/** The person of the scenario with `username`. */
export function personOf(username: string): Person {
  const p = people.find((x) => x.username === username);
  if (p === undefined) throw new Error(`scenario: no person ${username}`);
  return p;
}
