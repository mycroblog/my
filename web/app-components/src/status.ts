// SPDX-License-Identifier: AGPL-3.0-only

// The data a post is drawn from: the part of the client API's Status (pkg/mykit/ts, `components["schemas"]["Status"]`)
// that the post reads, with the API's own field names and types, so that a Status as the API sends it, or as
// @my/app-data keeps it, is a PostStatus as it is. The package depends on no API client; the shapes are written here.

import type { ContentEmoji } from "./post-content";

/** Who may see a post (the API's Visibility). */
export type PostVisibility = "public" | "local" | "unlisted" | "private" | "direct";

/** An account, as a status embeds it (the API's Account). */
export type PostAccount = {
  id: string;
  /** The username, without the domain; empty for a remote account without a confirmed handle. */
  username: string;
  /** The username for a local account, `username@domain` for a remote one. */
  acct: string;
  domain: string;
  /** The display name; empty when the account has none. */
  display_name: string;
  /** The custom emoji of the display name. */
  emojis: readonly ContentEmoji[];
  /** The avatar's renditions; omitted when there is none. */
  avatar?: { small: { url: string } } | undefined;
};

/** An attached media file (the API's MediaAttachment). */
export type PostMedia = {
  id: string;
  type: "image" | "gifv" | "video" | "audio" | "unknown";
  /** The alternative text; absent when there is none. */
  description?: string | undefined;
  url?: string | undefined;
  preview_url?: string | undefined;
};

/** The reactions to a post with one emoji (the API's StatusReaction). */
export type PostReaction = {
  /** The Unicode emoji, or the name of a custom one. */
  name: string;
  /** The shortcode of a custom emoji; omitted for a Unicode emoji. */
  shortcode?: string | undefined;
  /** The custom emoji with its image; omitted for a Unicode emoji. */
  emoji?: ContentEmoji | undefined;
  count: number;
  /** Whether the reader reacted with it. */
  me?: boolean | undefined;
};

/** An account a status mentions (the API's StatusMention). */
export type PostMention = { id: string; username: string; acct: string };

/** A status (the API's Status), as a post draws it. */
export type PostStatus = {
  id: string;
  account: PostAccount;
  /** The text as the server rendered and sanitized it. */
  content: string;
  /** The content warning; empty when there is none. */
  spoiler_text: string;
  visibility: PostVisibility;
  /** The BCP 47 tag of the text's language; `und` when it is unknown. */
  language: string;
  in_reply_to_account_id?: string | undefined;
  /** When it was posted, as an ISO 8601 date-time. */
  created_at: string;
  /** The status this one boosts: what the post shows, `account` being who boosted it. */
  boost?: PostStatus | undefined;
  media_attachments: readonly PostMedia[];
  mentions: readonly PostMention[];
  /** The custom emoji of the text and the content warning. */
  emojis: readonly ContentEmoji[];
  replies_count: number;
  boosts_count: number;
  favourites_count: number;
  reactions: readonly PostReaction[];
  favourited?: boolean | undefined;
  boosted?: boolean | undefined;
  bookmarked?: boolean | undefined;
  /** The status it quotes, shown one level deep. */
  quote?: { state: string; quoted_status?: PostStatus | undefined } | undefined;
};

/** A post in the reader's language (the API's StatusTranslation). */
export type PostTranslation = {
  /** The language of the translation. */
  language: string;
  /** The language translated from; `und` when it is not known. */
  source_language: string;
  content: string;
  spoiler_text: string;
  /** The alternative text of the media that has any, in the language of the translation. */
  media_attachments: readonly { id: string; description: string }[];
  provider?: { name: string } | undefined;
};
