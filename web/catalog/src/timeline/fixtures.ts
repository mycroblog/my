// SPDX-License-Identifier: AGPL-3.0-only

// The sample home timeline of the catalog: plain fixtures in en and ja, of every kind a timeline holds
// (replies, boosts, content warnings, one, two and four images, long text with links, mentions, hashtags and custom
// emoji, a post with no counts, one's own post, quotes, emoji reactions, translations). The images are generated
// scenes, not photos. `statusOf` turns a fixture into the status the API would send for it, and `translationOf` into
// the translation it would answer, which the post of @my/app-components draws.

import type { ContentEmoji, PostAccount, PostStatus, PostTranslation, PostVisibility } from "@my/app-components";
import type { Visibility } from "../Times";
import { images } from "../Avatars";
import { serverHtml } from "../serverHtml";

const minute = 60_000;
const hour = 60 * minute;
const day = 24 * hour;

type Account = { name: string; handle: string; avatar?: string };

type Media = { alt: string; src: string };

export type TimelinePost = {
  id: string;
  lang: "en" | "ja";
  author: Account;
  /** How long ago it was posted, in ms. */
  ago: number;
  /** Who may see it; `local`, seen on its own server alone, has no glyph on the catalog's older specimens. */
  visibility: Visibility | "local";
  /** Who boosted it into the timeline. */
  boostedBy?: Account;
  /** The handle of the account it replies to. */
  replyTo?: string;
  cw?: string;
  /** The text as its author wrote it, which the server renders as HTML (serverHtml.ts), with :shortcode: emoji. */
  text: string;
  media?: Media[];
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
  /** Shown translated at first, to show that state. */
  translated?: boolean;
};

export type QuotedPost = Pick<TimelinePost, "lang" | "author" | "ago" | "visibility" | "text" | "media">;

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

/** The generated scenes the sample posts show, which the rough screens (screens/) take for banners too. */
export const scenes = {
  dawn: scene("#f6c7a1", "#fbe6cf", "#fff4e0", ["#c98f7b", "#9a6a63", "#5d4448"]),
  sea: scene("#7fb3d5", "#d6ecf5", "#fffbe8", ["#5b8fa8", "#3f6f86", "#2a4d5f"]),
  forest: scene("#a9d0b0", "#e8f2df", "#fff9d9", ["#6f9a6a", "#4c7650", "#2f4f36"]),
  night: scene("#1d2440", "#4b4f7a", "#f3efd6", ["#3a3c5e", "#2a2b45", "#181a2c"]),
  autumn: scene("#f2b56b", "#f9dfb0", "#fff1d0", ["#c2683f", "#94462f", "#5e2d22"]),
  snow: scene("#c9d6e3", "#f3f6f9", "#ffffff", ["#e3e9ef", "#cfd8e1", "#9fb0c0"]),
};

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
function accountOf(account: Account): PostAccount {
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
    media_attachments: (post.media ?? []).map((m, i) => ({
      id: `${id}-${i}`,
      type: "image",
      description: m.alt,
      preview_url: m.src,
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
  };
  if (full.boostedBy === undefined) return shown;
  return { ...shown, id: `${id}-boost`, account: accountOf(full.boostedBy), boost: shown };
}

/** The translation the server would answer for a fixture in `lang`, by the provider; none when it has none. */
export function translationOf(post: TimelinePost, lang: "en" | "ja"): PostTranslation | undefined {
  const content = post.lang === lang ? undefined : post.translation?.[lang];
  if (content === undefined) return undefined;
  return {
    language: lang,
    source_language: post.lang,
    content: serverHtml(content),
    spoiler_text: "",
    media_attachments: [],
    provider: { name: translationProvider },
  };
}
