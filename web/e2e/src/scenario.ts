// SPDX-License-Identifier: AGPL-3.0-only

// The scenario (ADR-0015 §5): the people, posts and relationships of a small server in use, as data only. The seed
// (seed/seed.ts) applies it to a running `my serve` in the order of this file; the E2E tests sign in as its people;
// the fake API (fake/api.ts) answers the screens' stories from it. It holds what the screens of the web client show
// today (the home timeline, the composer, a post and its thread, the notifications, a profile, the sign-in with a
// password, a two-factor code or a passkey, and the consent to an app), and grows with the screens. It names no ID and
// no time: the server decides them, so nothing that reads the scenario depends on them. Every item has a key, which the
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
  /** Pinned to its author's profile. */
  pinned?: boolean;
};

/** What a person does to a post: a favourite, a boost, an emoji reaction (`:shortcode:` for a custom one), a bookmark. */
export type Interaction =
  | { kind: "favourite"; by: string; post: string }
  | { kind: "boost"; by: string; post: string }
  | { kind: "reaction"; by: string; post: string; emoji: string }
  | { kind: "bookmark"; by: string; post: string };

/** A follow; the follow of a locked account waits as a request until the followee accepts it. */
export type Follow = { follower: string; followee: string; accepted?: boolean };

/** A block or a mute of one person by another. */
export type Restriction = { kind: "block" | "mute"; by: string; target: string };

/** A custom emoji of the server, drawn by code. */
export type CustomEmoji = { shortcode: string; drawing: Drawing };

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

/** The posts, oldest first. */
export const posts: readonly Post[] = [
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

/** The person of the scenario with `username`. */
export function personOf(username: string): Person {
  const p = people.find((x) => x.username === username);
  if (p === undefined) throw new Error(`scenario: no person ${username}`);
  return p;
}
