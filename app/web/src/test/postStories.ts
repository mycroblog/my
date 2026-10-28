// SPDX-License-Identifier: AGPL-3.0-only
// The posts of the screen stories of the timelines, the post pages, compose, and notifications (ADR-0112 §1): a
// timeline as people see one, with long and right-to-left names, a content warning, a poll, a link card, a quote, a
// boost, a reply, and more pictures than a post shows, and the pictures themselves, which MSW's worker draws.
import { http, HttpResponse } from "msw";
import {
  accountFixture,
  imageFixture,
  instanceFixture,
  longDomainAccountFixture,
  longNameAccountFixture,
  otherAccountFixture,
  remoteAccountFixture,
  remoteStatusFixture,
  rtlNameAccountFixture,
  type Schemas,
  viewerFixture,
} from "./fixtures";

type Account = Schemas["Account"];
type Status = Schemas["Status"];

/** The colors of the drawn pictures, picked by the picture's address. */
const palette = ["#3b6ea8", "#a8573b", "#3ba86e", "#8a3ba8", "#a89a3b", "#3b98a8"];

/** A picture of 640 × 360 in one of the palette's colors, with a circle, as an SVG. */
function drawnPicture(seed: string): string {
  let hash = 0;
  for (const char of seed) hash = (hash * 31 + char.charCodeAt(0)) >>> 0;
  const color = palette[hash % palette.length] ?? "#3b6ea8";
  const x = 120 + (hash % 400);
  return `<svg xmlns="http://www.w3.org/2000/svg" width="640" height="360" viewBox="0 0 640 360"><rect width="640" height="360" fill="${color}"/><circle cx="${x}" cy="180" r="90" fill="#ffffff" fill-opacity="0.35"/></svg>`;
}

/**
 * Draws every picture of the stories' posts and accounts: the media of this server (its renditions and its media
 * proxy) and of other servers. The page has no server of those domains; without this the pictures would be broken.
 */
export const pictureHandlers = [
  http.get(`https://${instanceFixture.domain}/media/*`, ({ request }) =>
    HttpResponse.text(drawnPicture(new URL(request.url).pathname), { headers: { "Content-Type": "image/svg+xml" } }),
  ),
];

/** An account with an avatar, which the picture handlers draw. */
function withAvatar(account: Account): Account {
  return { ...account, avatar: imageFixture(`https://${instanceFixture.domain}/media/avatars/${account.id}`) };
}

export const bob = withAvatar(otherAccountFixture);
export const carol = withAvatar(remoteAccountFixture);
export const maximiliana = withAvatar(longNameAccountFixture);
export const abdulrahman = withAvatar(rtlNameAccountFixture);
export const dmitri = longDomainAccountFixture;
export const newsBot: Account = {
  ...withAvatar(accountFixture(31, "Weather", instanceFixture.domain)),
  display_name: "Weather report",
  bot: true,
};
export const haruka: Account = {
  ...withAvatar(accountFixture(32, "haruka", "mstdn.example.jp")),
  display_name: "春香",
};

/** The viewer's own account, as it shows on its posts. */
export const viewer: Account = viewerFixture;

/** The ID of the `n`th post of the stories. */
export function postId(n: number): string {
  return `0192f0c4-7a8e-7000-8000-0000000a${String(n).padStart(4, "0")}`;
}

/** A public English post `n` by `account`, `minutes` before the stories' last post, with `overrides`. */
export function post(n: number, account: Account, overrides: Partial<Status> = {}): Status {
  const id = postId(n);
  const local = account.domain === instanceFixture.domain;
  return {
    id,
    uri: local ? `https://${instanceFixture.domain}/ap/statuses/${id}` : `https://${account.domain}/statuses/${n}`,
    url: local
      ? `https://${instanceFixture.domain}/@${account.username}/status/${id}`
      : `https://${account.domain}/@${account.username}/${n}`,
    account,
    content: "",
    spoiler_text: "",
    sensitive: false,
    visibility: "public",
    language: "en",
    created_at: new Date(Date.UTC(2026, 9, 28, 12, 0) - n * 7 * 60_000).toISOString(),
    media_attachments: [],
    mentions: [],
    tags: [],
    emojis: [],
    replies_count: 0,
    boosts_count: 0,
    favourites_count: 0,
    reactions: [],
    ...overrides,
  };
}

/** A ready picture `n` of this server, with its renditions, described as `description` unless it is undefined. */
export function picture(n: number, description?: string): Schemas["MediaAttachment"] {
  const id = `0192f0c4-7a8e-7000-8000-0000000b${String(n).padStart(4, "0")}`;
  const variant = (size: string) => ({
    url: `https://${instanceFixture.domain}/media/${id}/r/${size}.svg`,
    content_type: "image/svg+xml",
    width: 640,
    height: 360,
    size: 1000,
  });
  return {
    id,
    type: "image",
    state: "ready",
    meta: { width: 640, height: 360, focus: { x: 0, y: 0 } },
    blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj",
    variants: { small: variant("small"), medium: variant("medium"), large: variant("large") },
    url: variant("large").url,
    preview_url: variant("small").url,
    ...(description === undefined ? {} : { description }),
  };
}

/** A tag of a post's content. */
function tag(name: string): Schemas["StatusTag"] {
  return { name, url: `https://${instanceFixture.domain}/tags/${name}` };
}

/** A paragraph linking hashtag `name`, as the server renders one. */
function hashtagLink(name: string): string {
  return `<a href="https://${instanceFixture.domain}/tags/${name}" class="mention hashtag" rel="tag">#<span>${name}</span></a>`;
}

/** A post with a poll of four choices, which ends tomorrow, and votes in it. */
export const pollPost = post(2, maximiliana, {
  content: "<p>Which bread should the bakery club try next week? Vote before Friday!</p>",
  poll: {
    id: "0192f0c4-7a8e-7000-8000-0000000c0001",
    expires_at: "2026-12-31T12:00:00Z",
    expired: false,
    multiple: false,
    votes_count: 37,
    voters_count: 37,
    options: [
      { title: "Sourdough", votes_count: 18 },
      { title: "Rye with caraway seeds", votes_count: 9 },
      { title: "Brioche", votes_count: 7 },
      { title: "Something else (tell me in a reply)", votes_count: 3 },
    ],
    voted: false,
    own_votes: [],
  },
  replies_count: 4,
  boosts_count: 2,
  favourites_count: 11,
});

/** A post of another server with more pictures than a post shows: the server kept four. */
export const manyPicturesPost: Status = {
  ...post(4, carol, {
    content: "<p>Day three of the trip: the coast, the lighthouse, the harbour, and far too many gulls.</p>",
    media_attachments: [
      picture(1, "A rocky coast under a grey sky"),
      picture(2, "A white lighthouse on a cliff"),
      picture(3, "Fishing boats in a small harbour"),
      picture(4),
    ],
    replies_count: 1,
    favourites_count: 23,
    reactions: [
      { name: "😍", count: 5, me: false },
      { name: "🌊", count: 2, me: false },
    ],
  }),
  omitted_media_count: 3,
};

/** A post behind a content warning, with a sensitive picture. */
export const warnedPost = post(5, abdulrahman, {
  content: "<p>صورة من العملية الجراحية لقطتي، كل شيء بخير الآن.</p>",
  language: "ar",
  spoiler_text: "Vet visit, a little blood",
  sensitive: true,
  media_attachments: [picture(5, "A cat with a bandage on its leg")],
  favourites_count: 3,
});

/** A post with a link and its card. */
export const cardPost = post(6, newsBot, {
  content:
    '<p>Rain from the afternoon, clearing by evening. <a href="https://weather.example/today" rel="nofollow noopener" target="_blank">weather.example/today</a></p>',
  card: {
    url: "https://weather.example/today",
    title: "Today's forecast: rain from the afternoon, clearing by evening",
    description: "Highs of 14°C, lows of 6°C. Take an umbrella if you go out after lunch.",
    site_name: "Weather Example",
    image: { url: `https://${instanceFixture.domain}/media/proxy/card/weather`, width: 1200, height: 630 },
  },
  boosts_count: 8,
});

/** A post that quotes the warned post's author's earlier post. */
export const quotePost = post(7, haruka, {
  content: "<p>これ、本当にそう思う。週末にもう一度読み直したい。</p>",
  language: "ja",
  quote: {
    state: "accepted",
    quoted_status: post(40, bob, {
      content:
        "<p>Writing every morning for a month changed how I think about drafts: the first one only has to exist.</p>",
      favourites_count: 52,
    }),
  },
  favourites_count: 6,
});

/** A boost by Bob of a post of a server with a long domain. */
const boostPost = post(8, bob, {
  boost: post(41, dmitri, {
    content: `<p>Our community garden opens its winter plots on Saturday. Bring gloves and a friend! ${hashtagLink("gardening")}</p>`,
    tags: [tag("gardening")],
    boosts_count: 14,
    favourites_count: 30,
  }),
});

/** A reply of the viewer to Bob, edited once. */
const replyPost = post(9, viewer, {
  content:
    '<p><span class="h-card"><a href="https://my.example/@bob" class="u-url mention">@<span>bob</span></a></span> Count me in for Saturday, I will bring the coffee.</p>',
  in_reply_to_id: postId(41),
  in_reply_to_account_id: bob.id,
  mentions: [{ id: bob.id, username: "bob", acct: "bob", url: bob.url }],
  edited_at: "2026-10-28T11:30:00Z",
  favourited: true,
  favourites_count: 1,
});

/** A long post with hashtags, a mention and reactions. */
export const longPost = post(1, bob, {
  content: `<p>Finally finished the sourdough guide I promised. It covers the starter, the first rise, shaping, and the bake, with the mistakes I made along the way so you do not have to make them too.</p><p>Thanks to everyone who tested the recipe and sent photos of their loaves. ${hashtagLink("baking")} ${hashtagLink("sourdough")}</p>`,
  tags: [tag("baking"), tag("sourdough")],
  replies_count: 12,
  boosts_count: 31,
  favourites_count: 128,
  reactions: [
    { name: "🍞", count: 24, me: true },
    { name: "👏", count: 9, me: false },
    { name: "🔥", count: 3, me: false },
  ],
  bookmarked: true,
});

/** A short post of the viewer, unlisted. */
export const viewerPost = post(3, viewer, {
  content: "<p>Morning walk done. The park is all orange leaves now.</p>",
  visibility: "unlisted",
  media_attachments: [picture(6, "A path covered in orange leaves"), picture(7, "A bench under a maple tree")],
  favourites_count: 4,
});

/** A timeline of every kind of post, newest first. */
export const variedTimeline: Status[] = [
  longPost,
  pollPost,
  viewerPost,
  manyPicturesPost,
  warnedPost,
  cardPost,
  quotePost,
  boostPost,
  replyPost,
  { ...remoteStatusFixture, account: carol, created_at: "2026-10-28T09:00:00Z" },
];
