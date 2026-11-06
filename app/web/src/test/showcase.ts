// SPDX-License-Identifier: AGPL-3.0-only
// The data of the showcase stories (the `Showcase` story of the home timeline, a post, a voice room, the supporter
// list, and the report queue): a busy server as a visitor would picture it, which the screenshots of the
// documentation site and the README show (`scripts/task.sh docs:screenshots`). The other stories test the edges (long
// and right-to-left names, missing pictures); these show the everyday. Every person, server, and picture is made up:
// the avatars, photos, and custom emoji are drawn here as SVG, and the servers use the reserved `.example` domains.
import { http, HttpResponse } from "msw";
import { accountFixture, imageFixture, instanceFixture, type Schemas, viewerFixture } from "./fixtures";
import { pictureHandlers, post } from "./postStories";

type Account = Schemas["Account"];
type Status = Schemas["Status"];

const media = `https://${instanceFixture.domain}/media/showcase`;

/** The two colors of an avatar's background and the color of the figure on it, by the avatar's name. */
const avatarColors: Record<string, [string, string, string]> = {
  alice: ["#f6c177", "#eb6f92", "#fff4e6"],
  minori: ["#9ccfd8", "#3e8fb0", "#eef8fb"],
  lena: ["#c4a7e7", "#7c5cbf", "#f5efff"],
  tomas: ["#a3d9a5", "#3f8f4f", "#effaf0"],
  priya: ["#ffb4a2", "#e5737a", "#fff1ee"],
  kofi: ["#f9d77e", "#d08c2c", "#fff8e6"],
  yuto: ["#8fb8f0", "#4a6fd1", "#eef3ff"],
  sofia: ["#f4a6c8", "#b8538a", "#fff0f7"],
  hana: ["#ffd1a1", "#e08a3c", "#fff5ea"],
  omar: ["#b6c8a0", "#6d8a4a", "#f4f8ee"],
  ren: ["#a0c4ff", "#5a7fd6", "#f0f5ff"],
  ava: ["#ffc6a8", "#d9774b", "#fff3ec"],
};

/** An avatar: a figure (head and shoulders) on a diagonal gradient. */
function drawAvatar(name: string): string {
  const [from, to, figure] = avatarColors[name] ?? ["#cccccc", "#888888", "#ffffff"];
  return `<svg xmlns="http://www.w3.org/2000/svg" width="128" height="128" viewBox="0 0 128 128"><defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="${from}"/><stop offset="1" stop-color="${to}"/></linearGradient></defs><rect width="128" height="128" fill="url(#g)"/><circle cx="64" cy="52" r="24" fill="${figure}" fill-opacity="0.92"/><ellipse cx="64" cy="122" rx="42" ry="34" fill="${figure}" fill-opacity="0.92"/></svg>`;
}

/** The scenes of the photos: a sky, a sun or a moon, and layers of land, water, or a table. */
const scenes: Record<string, string> = {
  // Sunset over hills.
  sunset:
    '<defs><linearGradient id="s" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#2d1b4e"/><stop offset="0.55" stop-color="#e9724c"/><stop offset="1" stop-color="#ffc857"/></linearGradient></defs><rect width="640" height="360" fill="url(#s)"/><circle cx="430" cy="235" r="62" fill="#ffe8a3"/><path d="M0 250 Q120 200 240 240 T480 230 T640 220 V360 H0 Z" fill="#7a3b4f"/><path d="M0 290 Q160 250 320 290 T640 280 V360 H0 Z" fill="#4a2340"/>',
  // A lake under mountains.
  lake: '<defs><linearGradient id="s" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#87c4e8"/><stop offset="1" stop-color="#e3f2fb"/></linearGradient></defs><rect width="640" height="360" fill="url(#s)"/><circle cx="120" cy="80" r="34" fill="#fff6d5"/><path d="M0 230 L120 110 L210 190 L330 70 L470 210 L560 140 L640 200 V250 H0 Z" fill="#5b7b8f"/><path d="M300 98 L330 70 L360 100 L345 96 L330 108 L315 96 Z" fill="#ffffff"/><rect y="235" width="640" height="125" fill="#3d7ea6"/><path d="M0 235 H640 V255 Q320 245 0 260 Z" fill="#2f6a8f"/><path d="M0 300 Q160 285 320 300 T640 300 V360 H0 Z" fill="#4f8a3f"/>',
  // A loaf of bread on a board.
  bread:
    '<rect width="640" height="360" fill="#f3e5d0"/><rect y="230" width="640" height="130" fill="#b9845a"/><ellipse cx="320" cy="250" rx="230" ry="36" fill="#8f5f3a"/><ellipse cx="320" cy="205" rx="190" ry="88" fill="#c7843f"/><ellipse cx="320" cy="190" rx="170" ry="66" fill="#d99a52"/><path d="M220 160 Q250 140 270 175 M300 150 Q330 130 350 168 M380 158 Q410 140 425 175" stroke="#f3d1a0" stroke-width="10" fill="none" stroke-linecap="round"/>',
  // A city at night.
  city: '<rect width="640" height="360" fill="#1d2440"/><circle cx="530" cy="70" r="30" fill="#f4f1de"/><circle cx="518" cy="62" r="30" fill="#1d2440"/><rect x="30" y="170" width="70" height="190" fill="#2e3a66"/><rect x="110" y="120" width="90" height="240" fill="#36457a"/><rect x="215" y="200" width="60" height="160" fill="#2e3a66"/><rect x="290" y="90" width="80" height="270" fill="#3d4f8c"/><rect x="385" y="150" width="100" height="210" fill="#2e3a66"/><rect x="500" y="190" width="110" height="170" fill="#36457a"/><g fill="#ffd27a"><rect x="125" y="140" width="12" height="16"/><rect x="160" y="180" width="12" height="16"/><rect x="305" y="110" width="12" height="16"/><rect x="340" y="150" width="12" height="16"/><rect x="305" y="210" width="12" height="16"/><rect x="400" y="170" width="12" height="16"/><rect x="450" y="230" width="12" height="16"/><rect x="520" y="210" width="12" height="16"/><rect x="570" y="260" width="12" height="16"/><rect x="45" y="200" width="12" height="16"/></g>',
};

/** The custom emoji of the showcase server: a heart, a party popper, and a loaf. */
const emojiDrawings: Record<string, string> = {
  heart_bounce:
    '<path d="M32 56 C10 40 4 28 8 18 C12 8 26 6 32 18 C38 6 52 8 56 18 C60 28 54 40 32 56 Z" fill="#ff5d8f"/><circle cx="22" cy="22" r="5" fill="#ffffff" fill-opacity="0.6"/>',
  party:
    '<path d="M10 56 L24 14 L50 40 Z" fill="#f4a259"/><path d="M17 35 L36 44 M21 24 L44 36" stroke="#ffffff" stroke-width="4"/><circle cx="44" cy="12" r="4" fill="#5bc0eb"/><circle cx="54" cy="24" r="3" fill="#ff5d8f"/><circle cx="34" cy="8" r="3" fill="#9bc53d"/>',
  fresh_bread:
    '<ellipse cx="32" cy="38" rx="26" ry="18" fill="#c7843f"/><ellipse cx="32" cy="32" rx="22" ry="14" fill="#e0a35c"/><path d="M20 28 Q24 22 28 30 M32 26 Q36 20 40 28" stroke="#f6d7a7" stroke-width="3" fill="none" stroke-linecap="round"/>',
};

function svg(width: number, height: number, body: string): Response {
  return HttpResponse.text(
    `<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}" viewBox="0 0 ${width} ${height}">${body}</svg>`,
    { headers: { "Content-Type": "image/svg+xml" } },
  );
}

/**
 * Draws the showcase's avatars, photos, and custom emoji, before the stories' other pictures (postStories.ts), which
 * every story of the showcase also needs.
 */
export const showcasePictures = [
  http.get(`${media}/avatars/:name`, ({ params }) =>
    HttpResponse.text(drawAvatar(String(params["name"])), { headers: { "Content-Type": "image/svg+xml" } }),
  ),
  http.get(`${media}/photos/:scene`, ({ params }) =>
    svg(640, 360, scenes[String(params["scene"]).replace(/\.svg$/, "")] ?? scenes["lake"] ?? ""),
  ),
  http.get(`${media}/emoji/:shortcode`, ({ params }) =>
    svg(64, 64, emojiDrawings[String(params["shortcode"]).replace(/\.svg$/, "")] ?? ""),
  ),
  ...pictureHandlers,
];

/** The avatar `name` of the showcase. */
function avatar(name: string): Schemas["MediaVariants"] {
  return imageFixture(`${media}/avatars/${name}`);
}

/** Person `n` of the showcase, `username` on `domain`, named `displayName`, with an avatar. */
function person(n: number, username: string, displayName: string, domain = instanceFixture.domain): Account {
  return { ...accountFixture(n, username, domain), display_name: displayName, avatar: avatar(username) };
}

const supporterBadge: Schemas["AccountBadge"] = {
  role_id: "0192f0c4-7a8e-7000-8000-0000000000b7",
  label: "Supporter",
  color: "green",
};

/** The viewer, Alice, with an avatar. */
export const showcaseViewer: Partial<Schemas["CredentialAccount"]> = {
  display_name: "Alice Moreau",
  avatar: avatar("alice"),
};

export const minori = { ...person(81, "minori", "佐藤みのり"), badges: [supporterBadge] };
export const lena = person(82, "lena", "Lena Fischer", "social.example");
export const tomas = person(83, "tomas", "Tomás Rivera");
const priya = person(84, "priya", "Priya Nair", "photos.example");
export const kofi = { ...person(85, "kofi", "Kofi Mensah"), badges: [supporterBadge] };
const yuto = person(86, "yuto", "山田ゆうと", "mstdn.example.jp");
const sofia = person(87, "sofia", "Sofia Lindqvist");
export const hana = person(89, "hana", "Hana Kobayashi");
const omar = person(90, "omar", "Omar Haddad");
const ren = person(92, "ren", "鈴木れん");
const ava = person(93, "ava", "Ava Thompson");
export const alice: Account = { ...viewerFixture, ...showcaseViewer };

/** A photo of `scene` (sunset, lake, bread, city), described as `description`; `n` makes its ID unique. */
function photo(n: number, scene: string, description: string): Schemas["MediaAttachment"] {
  const id = `0192f0c4-7a8e-7000-8000-0000000e${String(n).padStart(4, "0")}`;
  const variant = {
    url: `${media}/photos/${scene}.svg`,
    content_type: "image/svg+xml",
    width: 640,
    height: 360,
    size: 1000,
  };
  return {
    id,
    type: "image",
    state: "ready",
    meta: { width: 640, height: 360, focus: { x: 0, y: 0 } },
    blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj",
    variants: { small: variant, medium: variant, large: variant },
    url: variant.url,
    preview_url: variant.url,
    description,
  };
}

/** Custom emoji `shortcode` of this server. */
function customEmoji(shortcode: string): Schemas["CustomEmoji"] {
  const url = `${media}/emoji/${shortcode}.svg`;
  return { shortcode, url, static_url: url };
}

/** The reactions with custom emoji `shortcode` of this server. */
function customReaction(shortcode: string, count: number, me = false): Schemas["StatusReaction"] {
  return { name: shortcode, shortcode, emoji: customEmoji(shortcode), count, me };
}

/** A paragraph linking hashtag `name`, as the server renders one. */
function hashtag(name: string): string {
  return `<a href="https://${instanceFixture.domain}/tags/${encodeURIComponent(name)}" class="mention hashtag" rel="tag">#<span>${name}</span></a>`;
}

/** Post `n` of the showcase (IDs of their own, after those of postStories.ts) by `account`. */
function showcasePost(n: number, account: Account, overrides: Partial<Status>): Status {
  return post(200 + n, account, overrides);
}

/** Priya's photos of the lake, which Kofi quotes. */
export const lakePost = showcasePost(1, priya, {
  content: `<p>Woke up at five for this and it was worth every minute. The lake was perfectly still. ${hashtag("photography")}</p>`,
  tags: [{ name: "photography", url: `https://${instanceFixture.domain}/tags/photography` }],
  media_attachments: [
    photo(1, "lake", "A still mountain lake at dawn, with snow on the highest peak"),
    photo(2, "sunset", "The sun setting behind two rows of purple hills"),
  ],
  replies_count: 8,
  boosts_count: 24,
  favourites_count: 96,
  reactions: [
    customReaction("heart_bounce", 31, true),
    { name: "😍", count: 12, me: false },
    { name: "🌄", count: 5, me: false },
  ],
});

/** Kofi's quote of Priya's photos. */
export const quoteOfLake = showcasePost(2, kofi, {
  content: "<p>This is the photo that made me finally buy a tripod. Saving it as my wallpaper for the winter.</p>",
  quote: { state: "accepted", quoted_status: lakePost },
  replies_count: 3,
  boosts_count: 6,
  favourites_count: 41,
  reactions: [customReaction("heart_bounce", 9), customReaction("party", 4), { name: "📸", count: 3, me: false }],
});

/** Minori's bread, in Japanese, with a custom emoji in the text. */
const breadPost = showcasePost(3, minori, {
  content: `<p>週末に焼いたカンパーニュ、やっとクープがきれいに開きました :fresh_bread: ${hashtag("パン")}</p>`,
  language: "ja",
  emojis: [customEmoji("fresh_bread")],
  tags: [{ name: "パン", url: `https://${instanceFixture.domain}/tags/%E3%83%91%E3%83%B3` }],
  media_attachments: [photo(3, "bread", "A round country loaf with an open score on a wooden board")],
  replies_count: 5,
  boosts_count: 11,
  favourites_count: 58,
  reactions: [
    customReaction("fresh_bread", 22, true),
    { name: "🤤", count: 14, me: false },
    customReaction("party", 6),
  ],
});

/** Lena's poll. */
const pollOfLena = showcasePost(4, lena, {
  content: "<p>Planning the next community meetup. Which evening works best for you?</p>",
  poll: {
    id: "0192f0c4-7a8e-7000-8000-0000000e0101",
    expires_at: "2026-12-31T12:00:00Z",
    expired: false,
    multiple: false,
    votes_count: 84,
    voters_count: 84,
    options: [
      { title: "Tuesday", votes_count: 19 },
      { title: "Thursday", votes_count: 41 },
      { title: "Saturday afternoon", votes_count: 24 },
    ],
    voted: true,
    own_votes: [1],
  },
  replies_count: 12,
  boosts_count: 9,
  favourites_count: 27,
});

/** Tomás's post behind a content warning. */
const warnedOfTomas = showcasePost(5, tomas, {
  content: "<p>I did not see that ending coming. The whole last episode was one long held breath.</p>",
  spoiler_text: "Season finale spoilers",
  replies_count: 7,
  favourites_count: 19,
  reactions: [{ name: "😱", count: 8, me: false }],
});

/** Yuto's city at night, from another server, in Japanese. */
const cityPost = showcasePost(6, yuto, {
  content: "<p>残業帰りの夜景。今日もおつかれさまでした。</p>",
  language: "ja",
  media_attachments: [photo(4, "city", "Office towers with a few lit windows under a crescent moon")],
  replies_count: 2,
  boosts_count: 4,
  favourites_count: 33,
  reactions: [{ name: "🌙", count: 10, me: false }, customReaction("heart_bounce", 7)],
});

/** The home timeline of the showcase, newest first. */
export const showcaseTimeline: Status[] = [quoteOfLake, breadPost, pollOfLena, lakePost, warnedOfTomas, cityPost];

/** The replies to Kofi's quote, oldest first. */
export const quoteReplies: Status[] = [
  showcasePost(7, priya, {
    content: "<p>Thank you! The tripod was the best thing I ever bought. Happy to share the settings.</p>",
    in_reply_to_id: quoteOfLake.id,
    in_reply_to_account_id: kofi.id,
    favourites_count: 12,
    reactions: [customReaction("heart_bounce", 5)],
  }),
  showcasePost(8, hana, {
    content: "<p>Same here. I would love a little guide for beginners at the next meetup!</p>",
    in_reply_to_id: quoteOfLake.id,
    in_reply_to_account_id: kofi.id,
    favourites_count: 6,
  }),
  showcasePost(9, yuto, {
    content: "<p>この写真、本当に素敵ですね。</p>",
    language: "ja",
    in_reply_to_id: quoteOfLake.id,
    in_reply_to_account_id: kofi.id,
    favourites_count: 3,
  }),
];

/** The people of a voice room, all of this server: the host, a co-host, the other speakers, and the listeners. */
export const voiceHost: Account = kofi;
export const voiceCohost: Account = minori;
export const voiceSpeakers: Account[] = [tomas, sofia];
export const voiceListeners: Account[] = [hana, omar, ren, ava];

/** The people on the supporter list, all of this server, by plan, the cheapest first: coffee, the loaf, the bakery. */
export const supportersByPlan: [Account[], Account[], Account[]] = [
  [omar, ava, alice, ren],
  [sofia, tomas, hana],
  [kofi, minori],
];
