// SPDX-License-Identifier: AGPL-3.0-only
// The edit history of a post (FR-109), as plain values for the page (web-client/README.md A4): every revision,
// newest first, with when it was made, what it changed from the one before, and its content as the cards
// show it. The route loader has fetched the post and its history.
import { type CustomEmojiImage, type MediaItem, type SanitizedHtml, serverSanitizedHtml } from "@my/ui";
import { useSuspenseQuery } from "@tanstack/react-query";
import { useIntl } from "react-intl";
import { useApi } from "../../../lib/auth/SessionProvider";
import { useContentLinkClick } from "../../../lib/contentLinks";
import { type StatusEdit, statusHistoryQueryOptions, statusPath, statusQueryOptions } from "../api/status";
import { mediaItems } from "./useStatusView";

/** What a revision changed from the one before it. */
export type RevisionChange = "text" | "warning" | "sensitive" | "media";

export type RevisionView = {
  /** The revision's number, from 1 for the post as first written; unique in the list. */
  number: number;
  /** `current` for the content the post has now, `original` for the post as first written. */
  kind: "current" | "original" | "edit";
  /** When it was made: the ISO date for `<time>`, and the full date and time. */
  time: { dateTime: string; absolute: string };
  /** What it changed from the revision before; empty for the original. */
  changes: RevisionChange[];
  /** The body, as the server sanitized it. */
  html: SanitizedHtml;
  /** The content warning, or null when there is none. */
  contentWarning: string | null;
  /** The custom emoji of the body and the content warning. */
  emojis: readonly CustomEmojiImage[];
  media: MediaItem[];
  sensitive: boolean;
};

export type StatusHistoryView = {
  /** The post's page, which the history leads back to. */
  postPath: string;
  /** The language of the post (BCP 47), or undefined when the server does not know it. */
  lang: string | undefined;
  /** The revisions, newest first. */
  revisions: RevisionView[];
  onLinkClick: (event: MouseEvent, link: HTMLAnchorElement) => void;
};

/** Whether two revisions attach different media, or describe them differently. */
function mediaDiffer(a: StatusEdit, b: StatusEdit): boolean {
  if (a.media_attachments.length !== b.media_attachments.length) return true;
  return a.media_attachments.some((media, i) => {
    const other = b.media_attachments[i];
    return other === undefined || media.id !== other.id || (media.description ?? "") !== (other.description ?? "");
  });
}

/** What `revision` changed from `previous`. */
function changesFrom(revision: StatusEdit, previous: StatusEdit | undefined): RevisionChange[] {
  if (previous === undefined) return [];
  const changes: RevisionChange[] = [];
  if (revision.content !== previous.content) changes.push("text");
  if (revision.spoiler_text !== previous.spoiler_text) changes.push("warning");
  if (revision.sensitive !== previous.sensitive) changes.push("sensitive");
  if (mediaDiffer(revision, previous)) changes.push("media");
  return changes;
}

/** Reads post `id` and its revisions, which the route loader has fetched; a failure goes to the route's error boundary. */
export function useStatusHistory(id: string): StatusHistoryView {
  const api = useApi();
  const intl = useIntl();
  const onLinkClick = useContentLinkClick();
  const { data: post } = useSuspenseQuery(statusQueryOptions(api, id));
  const { data: history } = useSuspenseQuery(statusHistoryQueryOptions(api, id));
  const lang = post.language === "und" ? undefined : post.language;
  const last = history.items.length - 1;
  const revisions = history.items.map((revision, index): RevisionView => ({
    number: index + 1,
    kind: index === last ? "current" : index === 0 ? "original" : "edit",
    time: {
      dateTime: revision.created_at,
      absolute: intl.formatDate(Date.parse(revision.created_at), { dateStyle: "full", timeStyle: "short" }),
    },
    changes: changesFrom(revision, history.items[index - 1]),
    // `content` is sanitized HTML (api/openapi.yaml, StatusEdit.content).
    html: serverSanitizedHtml(revision.content),
    contentWarning: revision.spoiler_text === "" ? null : revision.spoiler_text,
    emojis: revision.emojis,
    media: mediaItems(revision.media_attachments, lang),
    sensitive: revision.sensitive,
  }));
  return { postPath: statusPath(post), lang, revisions: revisions.toReversed(), onLinkClick };
}
