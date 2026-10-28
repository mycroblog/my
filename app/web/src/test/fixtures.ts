// SPDX-License-Identifier: AGPL-3.0-only
// The fixtures that more than one feature's tests and MSW handlers use: the instance, the accounts and the
// posts. They take their types from the TypeScript types generated from api/openapi.yaml (web-client/README.md S9);
// API types are never written by hand here. A fixture that only one feature uses lives in that feature's
// test/handlers.ts.
import type { components } from "@mycroblog/mykit";

/** The schemas of api/openapi.yaml, for the fixtures and the MSW handlers of the features. */
export type Schemas = components["schemas"];

/** A valid `Instance` for GET /instance (operationId getInstance). */
export const instanceFixture: Schemas["Instance"] = {
  domain: "my.example",
  name: "my example",
  version: "0.0.0-test",
  source_url: "https://github.com/mycroblog/my",
  registrations: { mode: "approval" },
  limits: { max_post_characters: 500, require_media_description: false },
  timelines: { local: "everyone", federated: "everyone" },
  federation: { mode: "both" },
  sends_email: true,
  search_indexing: "deny",
  ai_usage: "deny",
  finance_visibility: "private",
  income_visibility: "private",
  supporters_enabled: false,
  supporter_list_visibility: "off",
  voice_enabled: false,
  voice_room_starters: "permitted",
  rules: [],
};

/** The signed-in user's `CredentialAccount` for GET /accounts/verify_credentials (operationId verifyAccountCredentials). */
export const viewerFixture: Schemas["CredentialAccount"] = {
  id: "0192f0c4-7a8e-7000-8000-000000000001",
  username: "alice",
  acct: "alice",
  domain: "my.example",
  uri: "https://my.example/ap/actors/0192f0c4-7a8e-7000-8000-000000000001",
  display_name: "Alice",
  note: "",
  fields: [],
  emojis: [],
  badges: [],
  url: "https://my.example/@alice",
  locked: false,
  bot: false,
  created_at: "2026-10-01T00:00:00Z",
  discoverable: true,
  indexable: true,
  ai_opt_out: false,
  source: { note: "", fields: [] },
  posting: { visibility_ceiling: "public", visibilities: ["public", "local", "unlisted", "private", "direct"] },
  permissions: [],
};

/**
 * An `Account` of another user named `name` on `domain` (another server by default); `n` makes its ID
 * unique. An account of this instance has its username as its acct.
 */
export function accountFixture(n: number, name: string, domain = "remote.example"): Schemas["Account"] {
  const username = name.toLowerCase();
  const id = `0192f0c4-7a8e-7000-8000-${String(n).padStart(12, "0")}`;
  return {
    id,
    username,
    acct: domain === instanceFixture.domain ? username : `${username}@${domain}`,
    domain,
    uri:
      domain === instanceFixture.domain ? `https://${domain}/ap/actors/${id}` : `https://${domain}/users/${username}`,
    display_name: name,
    note: "",
    fields: [],
    emojis: [],
    badges: [],
    url: `https://${domain}/@${username}`,
    locked: false,
    bot: false,
    created_at: "2026-10-01T00:00:00Z",
  };
}

/** Bob, another local account: the author of the posts of the timelines and notifications. */
export const otherAccountFixture: Schemas["Account"] = accountFixture(2, "Bob", instanceFixture.domain);

/** An account of another server that the instance knows, which the viewer does not follow and is not followed by. */
export const remoteAccountFixture: Schemas["Account"] = accountFixture(3, "Carol");

/** An `Account` like `accountFixture`'s, with a display name other than its username. */
function namedAccountFixture(n: number, username: string, displayName: string, domain: string): Schemas["Account"] {
  return { ...accountFixture(n, username, domain), display_name: displayName };
}

/**
 * Accounts whose names test the layout of a post's header at 320 px (web-client/16-look-states-and-wording.md G7, G9): a local account
 * with a 30-character display name, an account of another server with an Arabic (right-to-left) display
 * name, and one on a long domain whose handle does not fit the card.
 */
export const longNameAccountFixture: Schemas["Account"] = namedAccountFixture(
  21,
  "maximiliana",
  "Maximiliana Theodora Wolfharte",
  instanceFixture.domain,
);
export const rtlNameAccountFixture: Schemas["Account"] = namedAccountFixture(
  22,
  "abdulrahman",
  "عبد الرحمن بن محمد الهاشمي",
  "remote.example",
);
export const longDomainAccountFixture: Schemas["Account"] = namedAccountFixture(
  23,
  "dmitri",
  "Dmitri",
  "the-longest-community-server-name-in-the-fediverse.social.example.com",
);

/** The `Relationship` with account `id` when the viewer has none, or with `overrides`. */
export function relationshipFixture(
  id: string,
  overrides: Partial<Schemas["Relationship"]> = {},
): Schemas["Relationship"] {
  return {
    id,
    following: false,
    requested: false,
    followed_by: false,
    requested_by: false,
    show_boosts: true,
    notify: false,
    blocking: false,
    muting: false,
    muting_notifications: false,
    ...overrides,
  };
}

/** The signed-in user's `Preferences` for GET /preferences (operationId getPreferences). */
export const preferencesFixture: Schemas["Preferences"] = {
  locale: "en",
  time_zone: "UTC",
  time_zone_chosen: true,
  default_visibility: "public",
  default_quote_policy: "public",
  default_post_language: "en",
  require_media_description: false,
  timeline_auto_insert: false,
  timeline_languages: [],
};

/** The renditions of an image whose every size is served at `url`, as a remote account's avatar is. */
export function imageFixture(url: string): Schemas["MediaVariants"] {
  const variant = { url };
  return { small: variant, medium: variant, large: variant };
}

/**
 * A `MediaAttachment` with `id` in `state`; a ready one has its renditions, their URLs as `url` and
 * `preview_url`, and its blurhash, as GET /media/{id} answers once the server has processed the file.
 */
export function mediaFixture(
  id: string,
  state: Schemas["MediaAttachment"]["state"] = "ready",
  description?: string,
): Schemas["MediaAttachment"] {
  const variant = (size: string) => ({
    url: `https://my.example/media/${id}/r/${size}.webp`,
    content_type: "image/webp",
    width: 640,
    height: 360,
    size: 1000,
  });
  return {
    id,
    type: "image",
    state,
    meta: { width: 640, height: 360, focus: { x: 0, y: 0 } },
    ...(state === "ready"
      ? {
          blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj",
          variants: { small: variant("small"), medium: variant("medium"), large: variant("large") },
          url: variant("large").url,
          preview_url: variant("small").url,
        }
      : {}),
    ...(description === undefined ? {} : { description }),
  };
}

/**
 * An image of a remote status: always ready, with no renditions of this instance; `url` and `preview_url` are
 * its media proxy URL and `remote_url` its URL on the remote server (api/openapi.yaml, MediaAttachment).
 */
function remoteMediaFixture(id: string, description?: string): Schemas["MediaAttachment"] {
  const proxied = `https://my.example/media/proxy/${id}`;
  return {
    id,
    type: "image",
    state: "ready",
    meta: { focus: { x: 0, y: 0 } },
    url: proxied,
    preview_url: proxied,
    remote_url: `https://remote.example/files/${id}.png`,
    ...(description === undefined ? {} : { description }),
  };
}

/** A public English `Status` `id` by `account` with no media, mentions, tags or counts, with `overrides`. */
function postFixture(
  account: Schemas["Account"],
  id: string,
  overrides: Partial<Schemas["Status"]>,
): Schemas["Status"] {
  return {
    id,
    uri: `https://my.example/users/${account.username}/statuses/${id}`,
    account,
    content: "",
    spoiler_text: "",
    sensitive: false,
    visibility: "public",
    language: "en",
    created_at: "2026-10-16T09:00:00Z",
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

/** A `Status` by the viewer, with `overrides`. */
export function viewerStatusFixture(overrides: Partial<Schemas["Status"]> = {}): Schemas["Status"] {
  return postFixture(viewerFixture, "0192f0c4-7a8e-7000-8000-0000000000a1", {
    content: "<p>Hello</p>",
    created_at: "2026-10-16T00:00:00Z",
    ...overrides,
  });
}

/** A post by Bob that mentions Alice. */
export function statusFixture(id: string): Schemas["Status"] {
  return postFixture(otherAccountFixture, id, {
    url: `https://my.example/@bob/status/${id}`,
    content: '<p><a href="https://my.example/@alice">@alice</a> hello</p>',
    mentions: [{ id: viewerFixture.id, username: "alice", acct: "alice", url: viewerFixture.url }],
  });
}

/** A local post of another account, for the timelines. */
export const timelineStatusFixture: Schemas["Status"] = postFixture(
  otherAccountFixture,
  "0192f0c4-7a8e-7000-8000-0000000000a1",
  {
    uri: "https://my.example/ap/statuses/0192f0c4-7a8e-7000-8000-0000000000a1",
    url: "https://my.example/@bob/status/0192f0c4-7a8e-7000-8000-0000000000a1",
    content: "<p>Hello from the local timeline!</p>",
  },
);

/** A page of a timeline with the given statuses and, when there are older ones, the cursor to them. */
export function statusPage(items: Schemas["Status"][], nextCursor?: string): Schemas["StatusPage"] {
  return nextCursor === undefined ? { items } : { items, next_cursor: nextCursor };
}

/**
 * A post of Carol on another server: its page is on that server, it has a content warning, and its two
 * pictures come through the media proxy, the second without a description.
 */
export const remoteStatusFixture: Schemas["Status"] = postFixture(
  remoteAccountFixture,
  "0192f0c4-7a8e-7000-8000-0000000003c1",
  {
    uri: "https://remote.example/users/carol/statuses/1",
    url: "https://remote.example/@carol/1",
    content: "<p>Hello from another server!</p>",
    spoiler_text: "Cats",
    media_attachments: [
      remoteMediaFixture("0192f0c4-7a8e-7000-8000-0000000003d1", "A cat on a sofa"),
      remoteMediaFixture("0192f0c4-7a8e-7000-8000-0000000003d2"),
    ],
  },
);
