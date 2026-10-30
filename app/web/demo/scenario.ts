// SPDX-License-Identifier: AGPL-3.0-only
// The demo scenario (ADR-0117 §1): the people, posts, relationships, moderation, roles, supporters, invites, and
// announcements of a server in use, as data only. demo/seed.ts applies it, in this order, through `bin/my` and the
// client and admin APIs; every item has a key, which the seed sends as `Idempotency-Key: demo:<key>`. The passwords
// are fixed so that people and the L4 agent can sign in as anyone; a server takes them only with the fake payments of
// a test build, which runs in the `development` profile only. URLs use the reserved `.test` and `.invalid` hosts.
import type { Drawing, RGB } from "./assets/draw.ts";

/** A person of the scenario, signed in with `email` and `password`. */
export type Persona = {
  username: string;
  email: string;
  password: string;
  /** `admin` holds the built-in admin role (the first account), `moderator` the built-in moderator role. */
  staff?: "admin" | "moderator";
  displayName: string;
  /** The profile text, plain text. */
  note: string;
  /** The UI locale and the language of the person's posts. */
  locale: "en" | "ja";
  /** Whether follows need the person's approval. */
  locked?: boolean;
  /** The color of the person's avatar, a disc on a gradient. */
  color: RGB;
  /**
   * A person who has just joined and done nothing yet: the seed leaves their profile empty (no display name, profile
   * text, or avatar; `displayName` is only what the L4 agent is told), and they follow no one and post nothing, so that
   * the home page shows them the first-run checklist and the suggested accounts.
   */
  newcomer?: boolean;
};

/** An image attached to a post, drawn by demo/assets/draw.ts, with its alt text, which also finds it again. */
type Image = { name: string; description: string; drawing: Drawing };

/** A post of a persona; `replyTo` and `quote` name earlier posts by key. */
export type Post = {
  key: string;
  author: string;
  text: string;
  visibility: "public" | "local" | "unlisted" | "private" | "direct";
  language: string;
  replyTo?: string;
  quote?: string;
  spoiler?: string;
  image?: Image;
  poll?: { options: string[]; expiresIn: number };
};

/** A favourite, an emoji reaction, or a boost of a post by a persona. */
export type Interaction =
  | { kind: "favourite"; by: string; post: string }
  | { kind: "boost"; by: string; post: string }
  | { kind: "reaction"; by: string; post: string; emoji: string };

/** A follow; the follow of a locked account waits until `accepted`. */
export type Follow = { follower: string; followee: string; accepted?: boolean };

/** A report of an account to the moderators; `resolve` handles it, otherwise it stays open. */
export type Report = {
  key: string;
  reporter: string;
  target: string;
  posts: string[];
  category: "spam" | "legal" | "violation" | "other";
  /** The rules it cites, by their position in `rules` (from 0); only with `violation`. */
  rules?: number[];
  comment: string;
  resolve?: { by: string; outcome: "actioned" | "dismissed" };
};

/** A moderation action on an account and, when `appeal` is set, the account's appeal of it, left open. */
export type ModerationAction = {
  key: string;
  moderator: string;
  target: string;
  kind: "warn";
  reasonCategory: "spam" | "legal" | "violation" | "other";
  publicReason: string;
  appeal?: string;
};

/** A role with a badge, granted to `holders`, who show the badge on their profile. */
export type Role = {
  key: string;
  position: number;
  permissions: ("create_invites" | "create_custom_emojis" | "start_voice_rooms")[];
  badge: {
    label: string;
    color: "neutral" | "red" | "orange" | "yellow" | "green" | "teal" | "blue" | "purple" | "pink";
  };
  holders: string[];
};

/** A cost of the instance, entered by an administrator. */
export type CostItem = {
  key: string;
  schedule: "once" | "weeks" | "months" | "years";
  interval: number;
  startsOn: string;
  name: string;
  provider: string;
  category: "compute" | "database" | "storage" | "network" | "domain" | "email" | "software" | "other";
  amount: string;
  currency: string;
  memo?: string;
};

/** The supporter program: a role, a plan on it, and a persona who subscribed through the fake checkout. */
export type Supporters = {
  role: Omit<Role, "holders">;
  plan: {
    key: string;
    name: string;
    description: string;
    monthlyPrice: { amount: string; currency: string };
    annualDiscountPercent: number;
    emojiLimit: number;
    rank: number;
  };
  subscriber: { username: string; interval: "month" | "year"; listed: boolean };
};

/** The password of a persona: fixed, so that anyone can sign in as them. */
const password = (username: string) => `demo-password-${username}`;

const persona = (p: Omit<Persona, "email" | "password">): Persona => ({
  ...p,
  email: `${p.username}@demo.test`,
  password: password(p.username),
});

export const personas: readonly Persona[] = [
  persona({
    username: "hana",
    staff: "admin",
    displayName: "Hana 花",
    note: "このデモサーバーの管理者です。困ったことがあれば気軽にどうぞ。",
    locale: "ja",
    color: [214, 92, 120],
  }),
  persona({
    username: "marco",
    staff: "moderator",
    displayName: "Marco",
    note: "Moderator. I read every report, usually within a day.",
    locale: "en",
    color: [64, 120, 200],
  }),
  persona({
    username: "yuki",
    displayName: "Yuki 雪",
    note: "写真と散歩。週末は山にいます。",
    locale: "ja",
    color: [90, 170, 210],
  }),
  persona({
    username: "sam",
    displayName: "Sam Rivera",
    note: "Writes about bread, bikes, and the occasional bug. Links: https://sam.example.test",
    locale: "en",
    color: [230, 150, 60],
  }),
  persona({
    username: "layla",
    displayName: "ليلى Layla",
    note: "Translator. أكتب بالعربية والإنجليزية.",
    locale: "en",
    color: [120, 90, 190],
  }),
  persona({
    username: "ren",
    displayName: "Ren 蓮",
    note: "鍵アカウントです。フォローは承認制です。",
    locale: "ja",
    locked: true,
    color: [70, 160, 110],
  }),
  persona({
    username: "quickdeals",
    displayName: "QuickDeals",
    note: "Best deals every day!!!",
    locale: "en",
    color: [200, 200, 70],
  }),
  persona({
    username: "kai",
    displayName: "kai",
    note: "",
    locale: "ja",
    color: [150, 150, 150],
    newcomer: true,
  }),
];

/**
 * The person who signs up from the landing page in the L4 task of the whole first run: not created by the seed, so
 * the agent creates the account with this email address and reads the confirmation email in Mailpit.
 */
export const applicant: Persona = persona({
  username: "mio",
  displayName: "Mio",
  note: "",
  locale: "ja",
  color: [240, 130, 90],
});

/** The persona the seed signs in as for the server's settings and the admin API. */
export const admin = "hana";

/** The instance texts, set with `my instance set`. */
export const instanceTexts: Readonly<Record<string, string>> = {
  name: "My Demo",
  short_description: "A demo server of my, filled with a small community. 小さなコミュニティのデモサーバーです。",
  description: [
    "This server runs the demo scenario of my: a few people who post, follow, react, report, and support the server.",
    "Nothing here is real; the payments go through a fake provider and no money moves.",
    "",
    "このサーバーは my のデモです。決済は試験用の fake で、お金は動きません。",
  ].join("\n"),
  terms: ["# Terms", "", "Be kind. One account per person.", "", "- No spam", "- No harassment"].join("\n"),
  privacy_policy: ["# Privacy", "", "This demo keeps its data until it restarts, and then forgets it."].join("\n"),
};

/** The instance settings, set with `my instance set` (docs/operations/instance-settings.md). */
export const instanceSettings: Readonly<Record<string, string>> = {
  federation_mode: "off",
  default_locale: "en",
  registration_mode: "approval",
  invite_creators: "everyone",
  voice_room_starters: "everyone",
  contact_account: admin,
  contact_email: "contact@demo.test",
  finance_visibility: "breakdown",
  income_visibility: "totals",
  supporter_list_visibility: "public",
  public_counts: "true",
};

/** The instance rules, added with `my instance rule add`, in order. */
export const rules: readonly { text: string; hint?: string }[] = [
  { text: "Be kind to each other.", hint: "Disagree with ideas, not people." },
  { text: "No spam or unsolicited advertising.", hint: "Repeated promotional posts and links count as spam." },
  { text: "Mark sensitive media as sensitive." },
];

/** The badges of the built-in staff roles, which their holders show, so that the landing page lists the operators. */
export const staffBadges: readonly { role: "admin" | "moderator"; badge: Role["badge"] }[] = [
  { role: "admin", badge: { label: "Admin", color: "red" } },
  { role: "moderator", badge: { label: "Moderator", color: "blue" } },
];

/** The roles that administrators made, with badges. */
export const roles: readonly Role[] = [
  {
    key: "translator",
    position: 50,
    permissions: ["create_invites"],
    badge: { label: "Translator", color: "teal" },
    holders: ["layla"],
  },
];

const photo = (name: string, description: string, top: RGB, bottom: RGB, shapes: Drawing["shapes"]): Image => ({
  name,
  description,
  drawing: { width: 960, height: 640, top, bottom, shapes },
});

export const posts: readonly Post[] = [
  {
    key: "hana-welcome",
    author: "hana",
    text: "ようこそ My Demo へ！このサーバーはデモです。気軽に投稿してみてください。\nWelcome! Read the rules in About, and say hi.",
    visibility: "public",
    language: "ja",
  },
  {
    key: "yuki-mountain",
    author: "yuki",
    text: "朝の山。空気が冷たくて気持ちよかった。#散歩",
    visibility: "public",
    language: "ja",
    image: photo(
      "mountain.png",
      "夜明けの空の下に、二つの青い山の稜線と、昇りかけの太陽。",
      [250, 196, 140],
      [140, 170, 220],
      [
        { kind: "circle", x: 640, y: 300, r: 70, color: [255, 236, 170] },
        { kind: "wave", y: 380, amplitude: 70, period: 900, phase: 0.4, color: [80, 100, 150] },
        { kind: "wave", y: 470, amplitude: 40, period: 520, phase: 2.1, color: [50, 70, 110] },
      ],
    ),
  },
  {
    key: "sam-bread",
    author: "sam",
    text: "Third attempt at a sourdough loaf and it finally rose. Recipe notes: https://sam.example.test/sourdough",
    visibility: "public",
    language: "en",
    image: photo(
      "bread.png",
      "A round brown loaf of bread on a wooden table, seen from above.",
      [240, 228, 206],
      [196, 160, 120],
      [
        { kind: "rect", x: 0, y: 420, w: 960, h: 220, color: [150, 105, 70] },
        { kind: "circle", x: 480, y: 360, r: 190, color: [176, 112, 60] },
        { kind: "circle", x: 480, y: 340, r: 150, color: [200, 140, 80] },
      ],
    ),
  },
  {
    key: "sam-thread-1",
    author: "sam",
    text: "Question for cyclists: do you clean your chain after every rainy ride, or only once a week?",
    visibility: "public",
    language: "en",
  },
  {
    key: "yuki-thread-2",
    author: "yuki",
    replyTo: "sam-thread-1",
    text: "@sam 雨の日のあとは毎回拭いてます。油は週一くらい。",
    visibility: "public",
    language: "ja",
  },
  {
    key: "marco-thread-3",
    author: "marco",
    replyTo: "yuki-thread-2",
    text: "@yuki @sam Same here: a quick wipe every time, a proper clean every couple of weeks.",
    visibility: "public",
    language: "en",
  },
  {
    key: "sam-thread-4",
    author: "sam",
    replyTo: "marco-thread-3",
    text: "@marco @yuki Thanks both, a wipe after every ride it is.",
    visibility: "public",
    language: "en",
  },
  {
    key: "layla-rtl",
    author: "layla",
    text: "صباح الخير! أترجم هذا الأسبوع واجهة الخادم إلى العربية. شكراً لكل من ساعد.",
    visibility: "public",
    language: "ar",
  },
  {
    key: "layla-sea",
    author: "layla",
    text: "Evening by the sea, before the translation sprint.",
    visibility: "public",
    language: "en",
    image: photo(
      "sea.png",
      "Calm sea waves under an orange evening sky with a low sun.",
      [255, 170, 110],
      [60, 90, 150],
      [
        { kind: "circle", x: 300, y: 330, r: 60, color: [255, 220, 150] },
        { kind: "wave", y: 360, amplitude: 12, period: 180, phase: 0, color: [40, 80, 140] },
        { kind: "wave", y: 440, amplitude: 16, period: 240, phase: 1, color: [30, 60, 120] },
      ],
    ),
  },
  {
    key: "ren-followers",
    author: "ren",
    text: "フォロワー限定：来週の読書会、場所が決まりました。",
    visibility: "private",
    language: "ja",
  },
  {
    key: "yuki-local",
    author: "yuki",
    text: "このサーバーの人だけに：週末の写真散歩、一緒に行く人いますか？",
    visibility: "local",
    language: "ja",
  },
  {
    key: "sam-unlisted",
    author: "sam",
    text: "Unlisted: my notes from the bike repair class, for whoever asked. https://notes.example.test/bike",
    visibility: "unlisted",
    language: "en",
  },
  {
    key: "marco-direct",
    author: "marco",
    text: "@layla Thanks for the Arabic translation work. Ping me if a string needs context.",
    visibility: "direct",
    language: "en",
  },
  {
    key: "hana-poll",
    author: "hana",
    text: "次のお知らせはどれがいい？ / Which feature should the next announcement cover?",
    visibility: "public",
    language: "ja",
    poll: {
      options: ["Voice chat / ボイスチャット", "Supporters / サポーター", "Invites / 招待"],
      expiresIn: 7 * 24 * 3600,
    },
  },
  {
    key: "sam-quote",
    author: "sam",
    quote: "yuki-mountain",
    text: "This view is worth getting up early for.",
    visibility: "public",
    language: "en",
  },
  {
    key: "sam-cw",
    author: "sam",
    spoiler: "Spoilers for the last episode",
    text: "The detective was the narrator the whole time. I did not see that coming.",
    visibility: "public",
    language: "en",
  },
  {
    key: "quickdeals-spam-1",
    author: "quickdeals",
    text: "🔥 90% OFF everything today only!!! Click now: https://deals.invalid/now",
    visibility: "public",
    language: "en",
  },
  {
    key: "quickdeals-spam-2",
    author: "quickdeals",
    text: "@yuki @sam You won a prize!!! Claim it at https://deals.invalid/prize",
    visibility: "public",
    language: "en",
  },
];

export const follows: readonly Follow[] = [
  { follower: "yuki", followee: "sam" },
  { follower: "sam", followee: "yuki" },
  { follower: "marco", followee: "sam" },
  { follower: "marco", followee: "yuki" },
  { follower: "layla", followee: "yuki" },
  { follower: "yuki", followee: "layla" },
  { follower: "sam", followee: "layla" },
  { follower: "hana", followee: "yuki" },
  { follower: "hana", followee: "sam" },
  { follower: "hana", followee: "marco" },
  { follower: "yuki", followee: "hana" },
  { follower: "sam", followee: "hana" },
  { follower: "layla", followee: "hana" },
  // Ren's account is locked: Yuki's request is accepted, Sam's waits in Ren's follow requests.
  { follower: "yuki", followee: "ren", accepted: true },
  { follower: "ren", followee: "yuki" },
  { follower: "sam", followee: "ren" },
];

/** A list of accounts that its owner follows (ADR-0122), which only the owner sees; found again by its name. */
export type List = { key: string; owner: string; name: string; members: string[] };

/**
 * Yuki's lists: one of people she follows whose posts fill its timeline, and one without members yet. Hana, whom she
 * follows too, is in neither, so that putting her in a list is a task of its own (screens/lists.md).
 */
export const lists: readonly List[] = [
  { key: "yuki-close-friends", owner: "yuki", name: "仲良し", members: ["sam", "layla"] },
  { key: "yuki-book-club", owner: "yuki", name: "読書会", members: [] },
];

export const interactions: readonly Interaction[] = [
  { kind: "favourite", by: "sam", post: "yuki-mountain" },
  { kind: "favourite", by: "hana", post: "yuki-mountain" },
  { kind: "reaction", by: "layla", post: "yuki-mountain", emoji: "😍" },
  { kind: "reaction", by: "marco", post: "yuki-mountain", emoji: "⛰️" },
  { kind: "boost", by: "sam", post: "yuki-mountain" },
  { kind: "boost", by: "layla", post: "yuki-mountain" },
  { kind: "favourite", by: "yuki", post: "sam-bread" },
  { kind: "reaction", by: "hana", post: "sam-bread", emoji: "🍞" },
  { kind: "boost", by: "yuki", post: "sam-bread" },
  { kind: "favourite", by: "yuki", post: "layla-rtl" },
  { kind: "reaction", by: "sam", post: "layla-rtl", emoji: "🙏" },
  { kind: "boost", by: "hana", post: "hana-welcome" },
  { kind: "favourite", by: "sam", post: "hana-welcome" },
  { kind: "favourite", by: "yuki", post: "sam-thread-4" },
];

export const reports: readonly Report[] = [
  {
    key: "report-quickdeals-spam",
    reporter: "yuki",
    target: "quickdeals",
    posts: ["quickdeals-spam-1", "quickdeals-spam-2"],
    category: "violation",
    rules: [1],
    comment: "知らないアカウントから懸賞のメンションが来ました。スパムだと思います。",
  },
  {
    key: "report-sam-cw",
    reporter: "layla",
    target: "sam",
    posts: ["sam-cw"],
    category: "other",
    comment: "Spoilers without a warning? It has one, my mistake.",
    resolve: { by: "marco", outcome: "dismissed" },
  },
];

export const moderationActions: readonly ModerationAction[] = [
  {
    key: "action-quickdeals-warn",
    moderator: "marco",
    target: "quickdeals",
    kind: "warn",
    reasonCategory: "spam",
    publicReason: "Repeated promotional posts that mention people who do not follow you break the rule against spam.",
    appeal: "These are real deals from our shop, not spam. Please reconsider.",
  },
];

export const supporters: Supporters = {
  role: {
    key: "supporter",
    position: 10,
    permissions: ["create_custom_emojis"],
    badge: { label: "Supporter", color: "pink" },
  },
  plan: {
    key: "plan-friend",
    name: "Friend of the server",
    description: "Helps pay for the server. Thank you! サーバー代の支えになります。",
    monthlyPrice: { amount: "500", currency: "JPY" },
    annualDiscountPercent: 15,
    emojiLimit: 5,
    rank: 1,
  },
  subscriber: { username: "yuki", interval: "month", listed: true },
};

export const costItems: readonly CostItem[] = [
  {
    key: "cost-vm",
    schedule: "months",
    interval: 1,
    startsOn: "2026-01-05",
    name: "Virtual machine",
    provider: "Cloud Example",
    category: "compute",
    amount: "1800",
    currency: "JPY",
    memo: "2 vCPU, 4 GB",
  },
  {
    key: "cost-storage",
    schedule: "months",
    interval: 1,
    startsOn: "2026-01-05",
    name: "Object storage",
    provider: "Cloud Example",
    category: "storage",
    amount: "300",
    currency: "JPY",
  },
  {
    key: "cost-domain",
    schedule: "years",
    interval: 1,
    startsOn: "2026-02-01",
    name: "Domain name",
    provider: "Registrar Example",
    category: "domain",
    amount: "1500",
    currency: "JPY",
  },
];

/** Invites: the staff's, created with `my invite create`, and the members' own, created with the client API. */
export const invites: {
  staff: readonly { key: string; maxUses: number; expiresIn: string }[];
  members: readonly { key: string; by: string; maxUses: number }[];
} = {
  staff: [{ key: "invite-staff-launch", maxUses: 10, expiresIn: "720h" }],
  members: [
    { key: "invite-yuki-friends", by: "yuki", maxUses: 3 },
    { key: "invite-layla-translators", by: "layla", maxUses: 5 },
  ],
};

/** Announcements of the administrators, published to every local user. */
export const announcements: readonly { key: string; title: string; text: string }[] = [
  {
    key: "announcement-welcome",
    title: "Welcome to My Demo! / ようこそ",
    text: "**Welcome to My Demo!** Read the [rules](/about) and introduce yourself with the tag #introductions.\n\nようこそ！まずはルールを読んで、自己紹介してみてください。",
  },
  {
    key: "announcement-maintenance",
    title: "Scheduled maintenance on Sunday",
    text: "Scheduled maintenance this Sunday, 02:00–03:00 UTC. Posting may be unavailable for a few minutes.",
  },
];
