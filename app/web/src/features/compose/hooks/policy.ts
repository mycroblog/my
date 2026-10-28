// SPDX-License-Identifier: AGPL-3.0-only
// The rules of a new post that the client applies before the server does: the narrower of two visibilities
// (in the order of visibilityMessages.ts, ADR-0017), the default of a reply, the account's visibility ceiling, the
// mentions a reply starts with and the remote ones a post names, and the character count.
import { textTokens } from "../../../lib/textTokens";
import type { PostingLimits, Status, Visibility } from "../api/compose";
import { visibilities } from "./visibilityMessages";

/** The narrower of two visibilities. */
export function narrower(a: Visibility, b: Visibility): Visibility {
  return visibilities.indexOf(a) >= visibilities.indexOf(b) ? a : b;
}

/**
 * The visibility a post gets when the user chooses none: the preference, and for a reply the narrower of
 * the preference and the replied-to post's (a reply to a `local` post is `local` by default, ADR-0017).
 */
export function defaultVisibility(preference: Visibility, replyTo: Status | undefined): Visibility {
  return replyTo === undefined ? preference : narrower(preference, replyTo.visibility);
}

/**
 * The visibility a post gets: the chosen one when the account's posting limits keep it, and their
 * ceiling otherwise. The server decides the limits and applies the same rule when it posts (PostingLimits
 * in api/openapi.yaml); without limits (not loaded yet) the choice stands.
 */
export function postedVisibility(chosen: Visibility, limits: PostingLimits | undefined): Visibility {
  return limits === undefined || limits.visibilities.includes(chosen) ? chosen : limits.visibility_ceiling;
}

/**
 * The mentions a reply starts with: the author of the replied-to post and the accounts it mentions,
 * without the viewer, each once.
 */
export function replyMentions(replyTo: Status, viewerId: string): string {
  const accts = [
    ...(replyTo.account.id === viewerId ? [] : [replyTo.account.acct]),
    ...replyTo.mentions.filter((m) => m.id !== viewerId).map((m) => m.acct),
  ];
  const unique = [...new Set(accts)];
  return unique.length === 0 ? "" : `${unique.map((acct) => `@${acct}`).join(" ")} `;
}

/**
 * The handles (`username@domain`, lowercase, each once) of the mentions in `text` that name a domain:
 * those that can be accounts of other servers. A mention without a domain is a local account. Mentions are
 * found as the server finds them (textTokens), so e-mail addresses and URLs hold none.
 */
export function domainMentions(text: string): string[] {
  const handles = textTokens(text)
    .filter((t) => t.kind === "mention" && t.text.includes("@", 1))
    .map((t) => t.text.slice(1).toLowerCase());
  return [...new Set(handles)];
}

/** Characters as the server counts them: Unicode code points (data-model/README.md). */
export function countCharacters(text: string): number {
  return Array.from(text).length;
}

/** The most attachments a post can have (StatusMediaInput in api/openapi.yaml). */
export const maxAttachments = 4;

/** The longest alternative text (StatusMediaItem.description in api/openapi.yaml). */
export const maxDescriptionLength = 1500;

/** The image types the server accepts (the uploadMedia operation). */
const acceptedImageTypes: readonly string[] = ["image/jpeg", "image/png", "image/gif", "image/webp"];

/** The video types the server accepts: MP4, QuickTime, and WebM (the uploadMedia operation, ADR-0054 §6). */
const acceptedVideoTypes: readonly string[] = ["video/mp4", "video/quicktime", "video/webm", "video/x-m4v"];

/** The audio types the server accepts: MP3, Ogg, M4A, and WebM (the uploadMedia operation, ADR-0054 §6). */
const acceptedAudioTypes: readonly string[] = [
  "audio/mpeg",
  "audio/mp3",
  "audio/ogg",
  "audio/opus",
  "audio/vorbis",
  "application/ogg",
  "audio/mp4",
  "audio/x-m4a",
  "audio/m4a",
  "audio/aac",
  "audio/webm",
];

/** Every type the file picker offers. */
export const acceptedMediaTypes: readonly string[] = [
  ...acceptedImageTypes,
  ...acceptedVideoTypes,
  ...acceptedAudioTypes,
];

/** What a picked file is by its declared type, or null for a type the server does not accept. */
export function mediaKind(type: string): "image" | "video" | "audio" | null {
  if (acceptedImageTypes.includes(type)) return "image";
  if (acceptedVideoTypes.includes(type)) return "video";
  if (acceptedAudioTypes.includes(type)) return "audio";
  return null;
}
