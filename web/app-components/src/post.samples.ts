// SPDX-License-Identifier: AGPL-3.0-only

// The sample posts of the stories: plain fixtures in en and ja, of every kind a timeline holds (replies, boosts,
// content warnings, one, two and four images, long text with links, mentions, hashtags and custom emoji, a post with
// no counts, one's own post, quotes, emoji reactions, translations). The images are generated scenes, not photos.
// `statusOf` turns a fixture into the status the API would send for it, its text rendered as the server renders it
// (the fake server of @my/e2e), and `translationOf` into the translation it would answer, which the post draws.

import { images, type Visibility } from "@my/design-system/samples";
import { serverHtml } from "@my/e2e/fake";
import type { PostEdit } from "./edit-history";
import type { ContentEmoji } from "./post-content";
import type { PostAccount, PostPoll, PostStatus, PostTranslation, PostVisibility } from "./status";

const minute = 60_000;
const hour = 60 * minute;
const day = 24 * hour;

type Account = { name: string; handle: string; avatar?: string };

/** An attachment: its picture (a video's or a sound's poster), its kind (an image by default), its file and length. */
type Media = {
  alt: string;
  src: string;
  kind?: "image" | "gifv" | "video" | "audio" | "unknown";
  href?: string;
  duration?: number;
};

/** The card of the post's first link (ADR-0062 §8). */
type Card = {
  url: string;
  title: string;
  description: string;
  site: string;
  lang?: string;
  image?: { src: string; width: number; height: number; alt?: string };
  author?: Account;
};

export type TimelinePost = {
  id: string;
  lang: "en" | "ja";
  author: Account;
  /** How long ago it was posted, in ms. */
  ago: number;
  /** Who may see it; `local`, seen on its own server alone, has no glyph on the design system's older specimens. */
  visibility: Visibility | "local";
  /** Who boosted it into the timeline. */
  boostedBy?: Account;
  /** The handle of the account it replies to. */
  replyTo?: string;
  cw?: string;
  /** The text as its author wrote it, which the server renders as HTML (serverHtml.ts), with :shortcode: emoji. */
  text: string;
  media?: Media[];
  /** Its media are sensitive: covered until shown (FR-104). */
  sensitive?: boolean;
  /** The card of its first link, which shows when it has neither media nor a quote. */
  card?: Card;
  counts: { replies: number; boosts: number; favourites: number };
  boosted?: boolean;
  favourited?: boolean;
  bookmarked?: boolean;
  own?: boolean;
  /** The quoted post, embedded one level deep (ADR-0069 §5). */
  quote?: QuotedPost;
  /** The emoji reactions by emoji, a Unicode emoji or a :shortcode:; `mine` is the reader's own (FR-116). */
  reactions?: Reaction[];
  /** The text in the reader's language, by that language, with the provider that translated it (ADR-0148). */
  translation?: Partial<Record<"en" | "ja", string>>;
  /** The rest of it in the reader's language, by that language: its content warning, poll options and alt texts. */
  translatedParts?: Partial<Record<"en" | "ja", { cw?: string; poll?: string[]; alt?: string[] }>>;
  /** Shown translated at first, to show that state. */
  translated?: boolean;
  /** Its poll (FR-112). */
  poll?: SamplePoll;
  /**
   * The versions before the current one, oldest first, each with how long ago it was made (the first: `ago`); the
   * post was last edited `edited` ms ago.
   */
  history?: { ago: number; text: string; cw?: string; media?: Media[] }[];
  edited?: number;
};

/**
 * A poll: its options with their votes, how many voted, when it ends (in ms from now; ended when not ahead), the
 * options the reader voted for, and whether its author hides the votes until it ends.
 */
type SamplePoll = {
  options: { title: string; votes: number }[];
  multiple?: boolean;
  voters: number;
  endsIn: number;
  voted?: number[];
  hideTotals?: boolean;
};

export type QuotedPost = Pick<TimelinePost, "lang" | "author" | "ago" | "visibility" | "text" | "media" | "sensitive">;

type Reaction = { emoji: string; count: number; mine?: boolean };

/** The provider a translation names. */
const translationProvider = "LibreTranslate";

const svg = (w: number, h: number, body: string) =>
  `data:image/svg+xml,${encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${w} ${h}" width="${w * 4}" height="${h * 4}">${body}</svg>`)}`;

/** A landscape: a sky from `top` to `low`, a sun, and three ridges darkening toward the front. */
const scene = (top: string, low: string, sun: string, ridges: [string, string, string]) =>
  svg(
    160,
    100,
    `<defs><linearGradient id="s" x2="0" y2="1"><stop offset="0" stop-color="${top}"/><stop offset="1" stop-color="${low}"/></linearGradient></defs><rect width="160" height="100" fill="url(#s)"/><circle cx="112" cy="44" r="14" fill="${sun}"/><path d="M0 62 L30 48 L58 58 L90 40 L124 56 L160 46 V100 H0Z" fill="${ridges[0]}"/><path d="M0 76 Q40 62 80 72 T160 68 V100 H0Z" fill="${ridges[1]}"/><path d="M0 88 Q60 80 160 86 V100 H0Z" fill="${ridges[2]}"/>`,
  );

/** A plate seen from above: a table of `table`, a plate, and food of `food`. */
const plate = (table: string, food: string) =>
  svg(
    100,
    100,
    `<rect width="100" height="100" fill="${table}"/><circle cx="50" cy="50" r="36" fill="#fbf7f2"/><circle cx="50" cy="50" r="27" fill="#efe7dd"/><circle cx="44" cy="46" r="12" fill="${food}"/><circle cx="58" cy="56" r="9" fill="${food}" opacity=".8"/><circle cx="56" cy="40" r="5" fill="#6f9a4c"/>`,
  );

/** The generated scenes the sample posts show. */
export const scenes = {
  dawn: scene("#f6c7a1", "#fbe6cf", "#fff4e0", ["#c98f7b", "#9a6a63", "#5d4448"]),
  sea: scene("#7fb3d5", "#d6ecf5", "#fffbe8", ["#5b8fa8", "#3f6f86", "#2a4d5f"]),
  forest: scene("#a9d0b0", "#e8f2df", "#fff9d9", ["#6f9a6a", "#4c7650", "#2f4f36"]),
  night: scene("#1d2440", "#4b4f7a", "#f3efd6", ["#3a3c5e", "#2a2b45", "#181a2c"]),
  autumn: scene("#f2b56b", "#f9dfb0", "#fff1d0", ["#c2683f", "#94462f", "#5e2d22"]),
  snow: scene("#c9d6e3", "#f3f6f9", "#ffffff", ["#e3e9ef", "#cfd8e1", "#9fb0c0"]),
};

/** Each scene's blurhash, as the server would give it: its sky, its light and its ridges, top to bottom. */
export const blurhashes = new Map<string, string>([
  [scenes.dawn, "IuPrad~U8_"],
  [scenes.sea, "IxHWWH%h01"],
  [scenes.forest, "I#K2tg.80K"],
  [scenes.night, "IE68O8WBDz"],
  [scenes.autumn, "IuPE-V~98y"],
  [scenes.snow, "IVP7OmR.0L"],
]);

const me: Account = { name: "Kumi Ito", handle: "@kumi@my.example" };

const alice: Account = { name: "Alice Chen", handle: "@alice@example.social", avatar: images.dusk };
const mika: Account = { name: "佐藤 美香", handle: "@mika@mstdn.example.jp", avatar: images.snow };
const news: Account = { name: "Open Source Weekly", handle: "@news@social.example.org", avatar: images.logo };
const takuya: Account = { name: "たくや", handle: "@takuya@example.jp" };
const ravi: Account = { name: "Ravi", handle: "@ravi@fedi.example.net" };
const hana: Account = { name: "山田 はな", handle: "@hana@my.example" };
const lena: Account = { name: "Lena Novak", handle: "@lena@toot.example.eu" };
const ken: Account = { name: "Kenji 🍜 Watanabe", handle: "@kenji@my.example" };

export const posts: TimelinePost[] = [
  {
    id: "own",
    lang: "en",
    author: me,
    ago: 2 * minute,
    visibility: "public",
    text: "Trying out the new home timeline on my.example. Hello, fediverse! :wave:",
    counts: { replies: 1, boosts: 0, favourites: 2 },
    own: true,
    reactions: [{ emoji: "👋", count: 3 }],
  },
  {
    id: "alice-reply",
    lang: "en",
    author: alice,
    ago: 3 * minute,
    visibility: "public",
    replyTo: "@mika@mstdn.example.jp",
    text: "@mika Morning! The bakery on the corner finally reopened. See you there at nine?",
    counts: { replies: 2, boosts: 0, favourites: 5 },
    favourited: true,
    translation: { ja: "@mika おはよう！角のパン屋がやっと再開したよ。9時にそこで会わない？" },
  },
  {
    id: "mika-long",
    lang: "ja",
    author: mika,
    ago: 12 * minute,
    visibility: "public",
    text: "週末に小さなサーバーを立ててみました :mycro: 最初はひとりで使うつもりでしたが、@alice@example.social さんに声をかけたら友だちが三人来てくれて、いまはタイムラインがちょうどいい速さで流れています。\n\n大きなサーバーにいたころは追いきれなかった会話が、ここでは全部読めるのがうれしい :blobcat: 設定の手順はブログにまとめました https://example.jp/blog/2026/11/small-server-from-scratch #小さなサーバー #fediverse",
    counts: { replies: 4, boosts: 12, favourites: 38 },
    reactions: [
      { emoji: ":blobcat:", count: 8, mine: true },
      { emoji: "🎉", count: 5 },
      { emoji: "👍", count: 3 },
      { emoji: ":mycro:", count: 2 },
    ],
    translation: {
      en: "I set up a small server over the weekend :mycro: I meant to use it alone at first, but when I asked @alice@example.social, three friends came along, and now the timeline flows at just the right speed.\n\nI'm glad I can read every conversation here, which I couldn't keep up with on a big server :blobcat: I wrote up the setup steps on my blog https://example.jp/blog/2026/11/small-server-from-scratch #小さなサーバー #fediverse",
    },
    translated: true,
  },
  {
    id: "ravi-quote",
    lang: "en",
    author: ravi,
    ago: 40 * minute,
    visibility: "public",
    text: "This is the guide I wish I'd had last year. The part on moving followers is worth reading twice.",
    quote: {
      lang: "en",
      author: news,
      ago: 3 * hour,
      visibility: "public",
      text: "New guide: how to move your account to another server without losing your followers, step by step. https://social.example.org/guides/move",
      media: [{ alt: "Low hills under a green sky", src: scenes.forest }],
    },
    counts: { replies: 2, boosts: 4, favourites: 19 },
  },
  {
    id: "news-boost",
    lang: "en",
    author: news,
    ago: hour,
    visibility: "unlisted",
    boostedBy: ken,
    text: "This week: three servers federate their first polls, and a guide to moving your account without losing your followers. https://social.example.org/weekly/42 #OpenSource",
    media: [{ alt: "A sea under a pale blue sky, with low hills on the horizon", src: scenes.sea }],
    counts: { replies: 18, boosts: 1_234, favourites: 12_840 },
    boosted: true,
  },
  {
    id: "takuya-cw",
    lang: "ja",
    author: takuya,
    ago: 2 * hour,
    visibility: "public",
    cw: "映画の結末に触れています",
    text: "最後の十分で全部ひっくり返るの、二回目に観るとちゃんと伏線があってすごい。冒頭の電話のシーン、あれが全部だった。",
    counts: { replies: 0, boosts: 0, favourites: 3 },
  },
  {
    id: "ravi-two",
    lang: "en",
    author: ravi,
    ago: 3 * hour,
    visibility: "public",
    text: "First snow on the ridge this morning, and the same ridge at dusk. #photography",
    media: [
      { alt: "A snowy ridge under a pale sky", src: scenes.snow },
      { alt: "The same ridge at dusk, in orange light", src: scenes.dawn },
    ],
    counts: { replies: 3, boosts: 5, favourites: 41 },
  },
  {
    id: "hana-four",
    lang: "ja",
    author: hana,
    ago: 5 * hour,
    visibility: "public",
    text: "三連休の旅行の写真をまとめて。どれも空がきれいだった :sparkle:",
    media: [
      { alt: "朝焼けの山並み", src: scenes.dawn },
      { alt: "青い海と低い丘", src: scenes.sea },
      { alt: "緑の森の上の空", src: scenes.forest },
      { alt: "紅葉した山と夕方の空", src: scenes.autumn },
    ],
    counts: { replies: 6, boosts: 9, favourites: 120 },
    bookmarked: true,
    reactions: [
      { emoji: "😍", count: 14 },
      { emoji: ":sparkle:", count: 6 },
    ],
  },
  {
    id: "lena-quiet",
    lang: "en",
    author: lena,
    ago: 7 * hour,
    visibility: "public",
    text: "Quiet morning. Tea, rain, and a book I have meant to finish since spring.",
    counts: { replies: 0, boosts: 0, favourites: 0 },
  },
  {
    id: "ken-reply",
    lang: "ja",
    author: ken,
    ago: 9 * hour,
    visibility: "public",
    replyTo: "@mika@mstdn.example.jp",
    text: "@mika おつかれさまです！ブログ読みました、DNS のところで詰まってたので助かります :blobcat:",
    counts: { replies: 1, boosts: 0, favourites: 4 },
  },
  {
    id: "hana-quote",
    lang: "ja",
    author: hana,
    ago: 11 * hour,
    visibility: "public",
    text: "これ、うちのサーバーでもやりたい。手順がていねいでわかりやすい :sparkle:",
    quote: {
      lang: "ja",
      author: mika,
      ago: day,
      visibility: "public",
      text: "週末に小さなサーバーを立ててみました :mycro: 最初はひとりで使うつもりでしたが、友だちが三人来てくれて、いまはタイムラインがちょうどいい速さで流れています。設定の手順はブログにまとめました https://example.jp/blog/2026/11/small-server-from-scratch",
    },
    counts: { replies: 0, boosts: 1, favourites: 6 },
    translation: { en: "I want to do this on our server too. The steps are careful and easy to follow :sparkle:" },
  },
  {
    id: "alice-long",
    lang: "en",
    author: alice,
    ago: 20 * hour,
    visibility: "public",
    text: "A few things I learned running a small server for a year:\n\n1. Most of the work is talking to people, not to the machine.\n2. Write the rules down before you need them.\n3. Back up, then check that the backup restores. :party:\n\nThe longer version, with the scripts I use, is on my blog: https://alice.example.social/notes/one-year-of-a-small-server-and-what-it-taught-me — thanks @ravi@fedi.example.net and @news for the nudge. #selfhosting #fediverse #SmallWeb",
    counts: { replies: 27, boosts: 310, favourites: 1_480 },
    favourited: true,
    boosted: true,
    reactions: [
      { emoji: "👍", count: 42 },
      { emoji: "🙏", count: 12 },
      { emoji: ":party:", count: 7 },
    ],
    translation: {
      ja: "小さなサーバーを一年運営して学んだこと:\n\n1. 仕事のほとんどは機械ではなく人と話すこと。\n2. 必要になる前にルールを書いておくこと。\n3. バックアップを取り、それが戻せるか確かめること。 :party:\n\n使っているスクリプト付きの長い版はブログに: https://alice.example.social/notes/one-year-of-a-small-server-and-what-it-taught-me — 後押ししてくれた @ravi@fedi.example.net と @news に感謝。 #selfhosting #fediverse #SmallWeb",
    },
  },
  {
    id: "mika-boost",
    lang: "ja",
    author: takuya,
    ago: day + 2 * hour,
    visibility: "public",
    boostedBy: mika,
    text: "駅前の古本屋、来月で閉まるらしい。学生のころ毎週通ってたので寂しい。最後にもう一度行ってきます。",
    counts: { replies: 5, boosts: 22, favourites: 64 },
    translation: {
      en: "The used bookshop by the station is closing next month, I hear. I went every week as a student, so it's sad. I'll go one last time.",
    },
  },
  {
    id: "lena-cw-photo",
    lang: "en",
    author: lena,
    ago: 2 * day,
    visibility: "public",
    cw: "Food, close up",
    text: "Made the soup from @alice's recipe. Worth every minute.",
    media: [{ alt: "A bowl of orange soup on a wooden table, seen from above", src: plate("#8a5a3c", "#e38b4a") }],
    counts: { replies: 2, boosts: 1, favourites: 17 },
  },
  {
    id: "hana-followers",
    lang: "ja",
    author: hana,
    ago: 3 * day,
    visibility: "followers",
    text: "フォロワーさん限定で。来週の集まり、場所が決まりました。詳細は DM で送ります。",
    counts: { replies: 3, boosts: 0, favourites: 8 },
  },
  {
    id: "ravi-direct",
    lang: "en",
    author: ravi,
    ago: 5 * day,
    visibility: "direct",
    text: "@kumi thanks for the invite! Signed up just now, see you on the timeline.",
    counts: { replies: 1, boosts: 0, favourites: 1 },
  },
  {
    id: "news-old",
    lang: "en",
    author: news,
    ago: 9 * day,
    visibility: "public",
    text: "Reminder: our server moves to new hardware on Saturday. Expect a few minutes of downtime around 03:00 UTC. Status: https://status.social.example.org",
    media: [{ alt: "A night sky over dark hills, with a pale moon", src: scenes.night }],
    counts: { replies: 9, boosts: 88, favourites: 203 },
  },
];

/** A page's image for a link card: a scene at the size the media proxy reports for it. */
const cardImage = (src: string, alt?: string) => ({ src, width: 640, height: 400, ...(alt ? { alt } : {}) });

/**
 * The sample posts of what a post carries besides images (FR-104, FR-105, FR-117): link cards, a video, a GIF, a sound,
 * a file of an unknown kind, the kinds mixed, and sensitive media alone, with a content warning, with a sound, and on a
 * card.
 */
export const mediaPosts: TimelinePost[] = [
  {
    id: "card-large",
    lang: "en",
    author: lena,
    ago: 25 * minute,
    visibility: "public",
    text: "A long read on how small servers keep their moderation humane, and what it costs. https://fieldnotes.example.org/2026/small-servers",
    card: {
      url: "https://fieldnotes.example.org/2026/small-servers",
      title: "Keeping it humane: moderation on small fediverse servers",
      description:
        "Five admins of servers under a thousand people talk about reports, burnout, and the rules they wrote together with their members.",
      site: "Field Notes",
      lang: "en",
      image: cardImage(scenes.sea, "A calm sea under a pale sky"),
    },
    counts: { replies: 4, boosts: 21, favourites: 57 },
  },
  {
    id: "card-ja",
    lang: "ja",
    author: hana,
    ago: 2 * hour,
    visibility: "public",
    text: "週末のお茶会の会場、ここにしました https://kissa.example.jp/shops/ekimae",
    card: {
      url: "https://kissa.example.jp/shops/ekimae",
      title: "喫茶 えきまえ｜駅から徒歩 1 分、昭和から続く純喫茶",
      description: "自家焙煎のブレンドと、厚切りのトーストが名物。日曜は 8 時から営業しています。",
      site: "喫茶ガイド",
      lang: "ja",
      image: cardImage(scenes.autumn),
    },
    counts: { replies: 2, boosts: 0, favourites: 9 },
  },
  {
    id: "card-text",
    lang: "en",
    author: ravi,
    ago: 5 * hour,
    visibility: "unlisted",
    text: "The spec for the new signature scheme is out: https://w3c.example.org/TR/http-signatures-next",
    card: {
      url: "https://w3c.example.org/TR/http-signatures-next",
      title: "HTTP Message Signatures, the next draft",
      description: "",
      site: "w3c.example.org",
    },
    counts: { replies: 0, boosts: 3, favourites: 5 },
  },
  {
    id: "card-author",
    lang: "en",
    author: alice,
    ago: 1 * day,
    visibility: "public",
    text: "Mika wrote up the whole trip, with the photos: https://travel.example.jp/mika/hokkaido-in-winter",
    card: {
      url: "https://travel.example.jp/mika/hokkaido-in-winter",
      title: "Hokkaido in winter, by train and on foot",
      description: "Ten days from Hakodate to Abashiri: snow, quiet stations, and the best soup curry I have had.",
      site: "Slow Travel",
      lang: "en",
      image: cardImage(scenes.snow, "Snow-covered hills under a white sky"),
      author: mika,
    },
    counts: { replies: 3, boosts: 12, favourites: 40 },
  },
  {
    id: "video",
    lang: "en",
    author: ravi,
    ago: 40 * minute,
    visibility: "public",
    text: "The tide coming in at dusk, ninety seconds of it.",
    media: [
      {
        kind: "video",
        alt: "Waves rolling onto a beach at dusk while the sky turns from orange to purple",
        src: scenes.dawn,
        duration: 92,
      },
    ],
    counts: { replies: 1, boosts: 4, favourites: 18 },
  },
  {
    id: "gif",
    lang: "ja",
    author: takuya,
    ago: 3 * hour,
    visibility: "public",
    text: "デプロイが通ったときの気持ち",
    media: [{ kind: "gifv", alt: "", src: scenes.forest }],
    counts: { replies: 0, boosts: 2, favourites: 11 },
  },
  {
    id: "audio",
    lang: "en",
    author: lena,
    ago: 6 * hour,
    visibility: "public",
    text: "Rain on the tin roof of the cabin last night. Headphones recommended.",
    media: [
      {
        kind: "audio",
        alt: "Field recording: steady rain on a tin roof, with distant thunder",
        src: "",
        duration: 194,
      },
    ],
    counts: { replies: 2, boosts: 5, favourites: 22 },
  },
  {
    id: "unknown",
    lang: "en",
    author: news,
    ago: 2 * day,
    visibility: "public",
    text: "The slides from this week's talk, as the presenter shared them.",
    media: [{ kind: "unknown", alt: "", src: "", href: "https://files.social.example.org/media/slides-2026.key" }],
    counts: { replies: 0, boosts: 6, favourites: 14 },
  },
  {
    id: "mixed",
    lang: "en",
    author: alice,
    ago: 8 * hour,
    visibility: "public",
    text: "Weekend at the coast: a photo, a GIF of the gulls, the waves on video, and the sunset.",
    media: [
      { alt: "A sea under a pale blue sky", src: scenes.sea },
      { kind: "gifv", alt: "Gulls circling over the pier", src: scenes.snow },
      { kind: "video", alt: "", src: scenes.night, duration: 41 },
      { alt: "The sun setting behind low hills", src: scenes.dawn },
    ],
    counts: { replies: 5, boosts: 9, favourites: 61 },
  },
  {
    id: "sensitive",
    lang: "en",
    author: ken,
    ago: 30 * minute,
    visibility: "public",
    text: "Finally had the stitches out. Healing well, but not for the squeamish.",
    sensitive: true,
    media: [{ alt: "A healing scar along a forearm, a week after surgery", src: scenes.autumn }],
    counts: { replies: 6, boosts: 0, favourites: 24 },
  },
  {
    id: "sensitive-four",
    lang: "ja",
    author: mika,
    ago: 4 * hour,
    visibility: "public",
    text: "解剖学の授業のスケッチ、4 枚",
    sensitive: true,
    media: [
      { alt: "腕の筋肉のスケッチ", src: scenes.dawn },
      { alt: "手の骨のスケッチ", src: scenes.sea },
      { alt: "肩の関節のスケッチ", src: scenes.forest },
      { alt: "脚の筋肉のスケッチ", src: scenes.snow },
    ],
    counts: { replies: 1, boosts: 2, favourites: 15 },
  },
  {
    id: "sensitive-cw",
    lang: "en",
    author: takuya,
    ago: 7 * hour,
    visibility: "public",
    cw: "Horror film, spoilers",
    text: "The last scene, which nobody in the cinema was ready for.",
    sensitive: true,
    media: [{ alt: "A dark corridor lit by a single flickering bulb", src: scenes.night }],
    counts: { replies: 3, boosts: 1, favourites: 7 },
  },
  {
    id: "sensitive-mixed",
    lang: "en",
    author: alice,
    ago: 8 * hour,
    visibility: "public",
    text: "What the vet showed us, and what she said about it. Not for the squeamish.",
    sensitive: true,
    media: [
      { alt: "An X-ray of a dog's leg with a pin in the bone", src: scenes.snow },
      { kind: "audio", alt: "The vet explaining the surgery, two minutes", src: "", duration: 131 },
    ],
    counts: { replies: 4, boosts: 0, favourites: 19 },
  },
  {
    id: "sensitive-card",
    lang: "en",
    author: lena,
    ago: 9 * hour,
    visibility: "public",
    text: "A hard read about what the flood left behind. https://news.example.org/2026/flood-aftermath",
    sensitive: true,
    card: {
      url: "https://news.example.org/2026/flood-aftermath",
      title: "After the flood: the towns that are still waiting",
      description: "Six months on, families in the valley are still living in shelters, and the aid has slowed.",
      site: "The Example News",
      lang: "en",
      image: cardImage(scenes.night, "A flooded street at night"),
    },
    counts: { replies: 8, boosts: 30, favourites: 45 },
  },
];

const tile = (body: string) => svg(32, 32, body);

/** The custom emoji the sample text uses, by shortcode. */
const emoji: Record<string, string> = {
  wave: tile(
    `<circle cx="16" cy="16" r="14" fill="#ffcf5a"/><path d="M9 18 Q16 25 23 18" stroke="#6b4a1f" stroke-width="2.4" fill="none" stroke-linecap="round"/><circle cx="11" cy="12" r="2" fill="#6b4a1f"/><circle cx="21" cy="12" r="2" fill="#6b4a1f"/>`,
  ),
  blobcat: tile(
    `<path d="M5 12 L8 3 L13 9 L19 9 L24 3 L27 12 Q30 28 16 28 Q2 28 5 12Z" fill="#f2c14e"/><circle cx="12" cy="16" r="2" fill="#3a2a10"/><circle cx="20" cy="16" r="2" fill="#3a2a10"/><path d="M14 21 Q16 23 18 21" stroke="#3a2a10" stroke-width="1.6" fill="none"/>`,
  ),
  mycro: tile(
    `<rect width="32" height="32" rx="7" fill="#ff6b6b"/><path d="M8 22 V12 Q12 9 16 13 V22 M16 13 Q20 9 24 12 V22" stroke="#111" stroke-width="2.6" fill="none" stroke-linecap="round"/>`,
  ),
  party: tile(
    `<path d="M5 28 L13 8 L24 19Z" fill="#7a5cff"/><path d="M8 21 L18 13" stroke="#ffd34d" stroke-width="2.4"/><circle cx="24" cy="7" r="2.4" fill="#ff6b6b"/><circle cx="27" cy="14" r="1.8" fill="#4cc38a"/><circle cx="19" cy="4" r="1.6" fill="#ffd34d"/>`,
  ),
  sparkle: tile(
    `<path d="M16 2 L19 13 L30 16 L19 19 L16 30 L13 19 L2 16 L13 13Z" fill="#ffc94a"/><path d="M26 3 L27 6 L30 7 L27 8 L26 11 L25 8 L22 7 L25 6Z" fill="#ffe08a"/>`,
  ),
};

/** The same emoji as a status's `emojis` lists them; none is animated. */
export const emojis: ContentEmoji[] = Object.entries(emoji).map(([shortcode, src]) => ({
  shortcode,
  url: src,
  static_url: src,
}));

/** The server the reader is on: an account of it has a handle without its domain in `acct`. */
const here = "my.example";

/** An account as a status embeds it; its handle is its id. */
export function accountOf(account: Account): PostAccount {
  const [, username = "", domain = here] = account.handle.split("@");
  return {
    id: account.handle,
    username,
    acct: domain === here ? username : `${username}@${domain}`,
    domain,
    display_name: account.name,
    emojis,
    avatar: account.avatar === undefined ? undefined : { small: { url: account.avatar } },
  };
}

const visibilities: Record<Visibility | "local", PostVisibility> = {
  public: "public",
  local: "local",
  unlisted: "unlisted",
  followers: "private",
  direct: "direct",
};

/** The poll the API would send for a sample: the votes of each option left out while hidden and not ended. */
function pollOf(poll: SamplePoll, id: string): PostPoll {
  const expired = poll.endsIn <= 0;
  const hidden = (poll.hideTotals ?? false) && !expired;
  return {
    id,
    expires_at: new Date(Date.now() + poll.endsIn).toISOString(),
    expired,
    multiple: poll.multiple ?? false,
    votes_count: poll.options.reduce((n, o) => n + o.votes, 0),
    voters_count: poll.voters,
    options: poll.options.map((o) => (hidden ? { title: o.title } : { title: o.title, votes_count: o.votes })),
    voted: poll.voted !== undefined,
    own_votes: poll.voted ?? [],
  };
}

/** The status the API would send for a fixture: its text rendered as the server would, its times before now. */
export function statusOf(post: TimelinePost | QuotedPost, id = "quoted"): PostStatus {
  const full: Partial<TimelinePost> = post;
  const replyTo = full.replyTo === undefined ? undefined : accountOf({ name: "", handle: full.replyTo });
  const shown: PostStatus = {
    id,
    account: accountOf(post.author),
    content: serverHtml(post.text),
    spoiler_text: full.cw ?? "",
    visibility: visibilities[post.visibility],
    language: post.lang,
    in_reply_to_account_id: replyTo?.id,
    created_at: new Date(Date.now() - post.ago).toISOString(),
    sensitive: full.sensitive ?? false,
    card:
      full.card === undefined
        ? undefined
        : {
            url: full.card.url,
            title: full.card.title,
            description: full.card.description,
            site_name: full.card.site,
            language: full.card.lang,
            image:
              full.card.image === undefined
                ? undefined
                : {
                    url: full.card.image.src,
                    width: full.card.image.width,
                    height: full.card.image.height,
                    description: full.card.image.alt,
                    blurhash: blurhashes.get(full.card.image.src),
                  },
            author: full.card.author === undefined ? undefined : accountOf(full.card.author),
          },
    media_attachments: (post.media ?? []).map((m, i) => ({
      id: `${id}-${i}`,
      type: m.kind ?? "image",
      description: m.alt === "" ? undefined : m.alt,
      preview_url: m.src === "" ? undefined : m.src,
      url: m.href ?? (m.src === "" ? undefined : m.src),
      meta: m.duration === undefined ? undefined : { duration: m.duration },
      blurhash: blurhashes.get(m.src),
    })),
    mentions: replyTo === undefined ? [] : [{ id: replyTo.id, username: replyTo.username, acct: replyTo.acct }],
    emojis,
    replies_count: full.counts?.replies ?? 0,
    boosts_count: full.counts?.boosts ?? 0,
    favourites_count: full.counts?.favourites ?? 0,
    reactions: (full.reactions ?? []).map((r) => {
      const custom = emojis.find((e) => `:${e.shortcode}:` === r.emoji);
      return custom === undefined
        ? { name: r.emoji, count: r.count, me: r.mine ?? false }
        : { name: custom.shortcode, shortcode: custom.shortcode, emoji: custom, count: r.count, me: r.mine ?? false };
    }),
    favourited: full.favourited ?? false,
    boosted: full.boosted ?? false,
    bookmarked: full.bookmarked ?? false,
    quote:
      full.quote === undefined ? undefined : { state: "accepted", quoted_status: statusOf(full.quote, `${id}-quote`) },
    poll: full.poll === undefined ? undefined : pollOf(full.poll, `${id}-poll`),
    ...(full.edited === undefined ? {} : { edited_at: new Date(Date.now() - full.edited).toISOString() }),
  };
  if (full.boostedBy === undefined) return shown;
  return { ...shown, id: `${id}-boost`, account: accountOf(full.boostedBy), boost: shown };
}

/** The translation the server would answer for a fixture in `lang`, by the provider; none when it has none. */
export function translationOf(post: TimelinePost, lang: "en" | "ja"): PostTranslation | undefined {
  const content = post.lang === lang ? undefined : post.translation?.[lang];
  if (content === undefined) return undefined;
  const parts = post.translatedParts?.[lang];
  return {
    language: lang,
    source_language: post.lang,
    content: serverHtml(content),
    spoiler_text: parts?.cw ?? "",
    media_attachments: (parts?.alt ?? []).map((description, i) => ({ id: `${post.id}-${i}`, description })),
    poll_options: parts?.poll ?? [],
    provider: { name: translationProvider },
  };
}

/** The versions of an edited fixture (the API's StatusHistory), oldest first: its history, then itself. */
export function editsOf(post: TimelinePost): PostEdit[] {
  const at = (ago: number) => new Date(Date.now() - ago).toISOString();
  // An image keeps its id across the versions that hold it, as the server's does: here, by its description.
  const media = (m: Media[] | undefined) =>
    (m ?? []).map((x) => ({ id: x.alt, type: "image" as const, description: x.alt, preview_url: x.src }));
  const versions = (post.history ?? []).map((v) => ({
    content: serverHtml(v.text),
    spoiler_text: v.cw ?? "",
    created_at: at(v.ago),
    media_attachments: media(v.media),
    emojis,
  }));
  return [
    ...versions,
    {
      content: serverHtml(post.text),
      spoiler_text: post.cw ?? "",
      created_at: at(post.edited ?? post.ago),
      media_attachments: media(post.media),
      emojis,
    },
  ];
}

/**
 * A post in each language with every part a translation covers: a content warning, a poll, and an image with its
 * alt text, each translated into the other language.
 */
export const translatedWhole: Record<"en" | "ja", TimelinePost> = {
  en: {
    id: "whole-en",
    lang: "en",
    author: lena,
    ago: 50 * minute,
    visibility: "public",
    cw: "Book club pick, the ending",
    text: "Finished it last night. The last chapter made the slow first half worth it. Did you see it coming?",
    media: [
      { alt: "A paperback open face down on a wooden table, beside a cup of tea", src: plate("#8a5a3c", "#c9a27a") },
    ],
    counts: { replies: 6, boosts: 1, favourites: 9 },
    poll: {
      options: [
        { title: "From the first chapter", votes: 3 },
        { title: "Halfway through", votes: 7 },
        { title: "Not at all", votes: 12 },
      ],
      voters: 22,
      endsIn: 20 * hour,
    },
    translation: {
      ja: "昨夜読み終えました。最後の章のおかげで、ゆっくりした前半も読んだかいがありました。予想できました？",
    },
    translatedParts: {
      ja: {
        cw: "読書会の本、結末について",
        poll: ["最初の章から", "半分くらいで", "まったく"],
        alt: ["木のテーブルに伏せて開いた文庫本と、その横の紅茶"],
      },
    },
  },
  ja: {
    id: "whole-ja",
    lang: "ja",
    author: takuya,
    ago: 2 * hour,
    visibility: "public",
    cw: "映画の結末に触れています",
    text: "二回目に観たら、冒頭の電話のシーンが全部だった。みんなはどこで気づいた？",
    media: [{ alt: "暗い丘の上に浮かぶ淡い月", src: scenes.night }],
    counts: { replies: 4, boosts: 0, favourites: 11 },
    poll: {
      options: [
        { title: "冒頭の電話のシーン", votes: 4 },
        { title: "最後の十分", votes: 9 },
        { title: "二回目に観てから", votes: 6 },
      ],
      voters: 19,
      endsIn: 6 * hour,
    },
    translation: {
      en: "On a second watch, the phone call at the start was everything. Where did you all notice it?",
    },
    translatedParts: {
      en: {
        cw: "Mentions the ending of the film",
        poll: ["The phone call at the start", "The last ten minutes", "Only on a second watch"],
        alt: ["A pale moon over dark hills"],
      },
    },
  },
};
