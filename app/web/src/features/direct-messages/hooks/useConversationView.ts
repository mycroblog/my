// SPDX-License-Identifier: AGPL-3.0-only
// What a conversation's row shows, as plain values for the slots of @my/ui's PostCard (web-client/README.md A4): the
// hook decides what to show and formats it; the component only places it. It returns no JSX.
import { type CustomEmojiImage, type ImageSources, sanitizedHtmlText, serverSanitizedHtml } from "@my/ui";
import { imageSources } from "../../../lib/images";
import { nameOf } from "../../../lib/handle";
import { profilePath } from "../../../lib/paths";
import { type TimeView, useTimeView } from "../../../lib/time";
import { statusPath } from "../../status";
import type { Conversation, ConversationAccount, ConversationStatus } from "../api/conversations";

/** The most participants a row names; the others are counted. */
const namedParticipants = 3;

/** A participant of the conversation: their name, which links to their profile, and their avatar. */
export type ParticipantView = {
  id: string;
  name: string;
  emojis: readonly CustomEmojiImage[];
  href: string;
  avatar: ImageSources | undefined;
};

/** What the excerpt of the newest post says. */
type ExcerptView =
  /** The start of its text, in its own language. */
  | { kind: "text"; text: string; lang: string | undefined }
  /** Behind a content warning: the warning only, as the post's own page shows it first. */
  | { kind: "warning"; summary: string; lang: string | undefined }
  /** A post of pictures only has no text to quote. */
  | { kind: "media" };

export type ConversationView = {
  /** The other accounts of the conversation, as the server lists them; none when the user wrote only to themselves. */
  participants: readonly ParticipantView[];
  /** The ones the row names, and how many more it counts. */
  named: readonly ParticipantView[];
  more: number;
  /** Whether a post by another account is newer than what the user read, on any device. */
  unread: boolean;
  /** When the newest post was written, relative to now as in the timelines (web-client/16-look-states-and-wording.md G8). */
  time: TimeView;
  /** The thread: the newest post's page, which shows the posts before and after it. */
  href: string;
  excerpt: ExcerptView;
};

function participantOf(account: ConversationAccount): ParticipantView {
  return {
    id: account.id,
    name: nameOf(account),
    emojis: account.emojis,
    href: profilePath(account.acct),
    avatar: imageSources(account.avatar),
  };
}

function excerptOf(status: ConversationStatus): ExcerptView {
  const lang = status.language === "und" ? undefined : status.language;
  if (status.spoiler_text !== "") return { kind: "warning", summary: status.spoiler_text, lang };
  // Status.content is the server-sanitized HTML of the post (api/openapi.yaml), parsed by @my/ui under the app's
  // Trusted Types.
  const text = sanitizedHtmlText(serverSanitizedHtml(status.content));
  return text.trim() === "" ? { kind: "media" } : { kind: "text", text, lang };
}

/** The row of `conversation`; it renders again as the clock ticks. */
export function useConversationView(conversation: Conversation): ConversationView {
  const time = useTimeView(conversation.last_status.created_at);
  const participants = conversation.accounts.map(participantOf);
  const named = participants.slice(0, namedParticipants);
  return {
    participants,
    named,
    more: participants.length - named.length,
    unread: conversation.unread,
    time,
    href: statusPath(conversation.last_status),
    excerpt: excerptOf(conversation.last_status),
  };
}
