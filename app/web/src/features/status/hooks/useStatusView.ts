// SPDX-License-Identifier: AGPL-3.0-only
// What a status card shows, as plain values for the slots of @my/ui's PostCard (web-client/README.md A4): the
// hook decides what to show and formats it; the component only places it. It returns no JSX.
import {
  type CustomEmojiImage,
  type ImageSources,
  type LinkCardImage,
  type MediaItem,
  type SanitizedHtml,
  serverSanitizedHtml,
} from "@my/ui";
import { useIntl } from "react-intl";
import { useContentLinkClick } from "../../../lib/contentLinks";
import { handleOf, isRemoteAcct } from "../../../lib/handle";
import { hostOf } from "../../../lib/host";
import { profilePath } from "../../../lib/paths";
import { type TimeView, useTimeView } from "../../../lib/time";
import { useT } from "../../../lib/i18n";
import { visibilityLabel } from "../../compose";
import {
  type LinkCard,
  type MediaAttachment,
  type Status,
  type Visibility,
  statusHistoryPath,
  statusPath,
  statusShareUrl,
} from "../api/status";
import { imageSources, originalImage } from "../../../lib/images";

export type StatusView = {
  /** The display name of the account that boosted the post and its profile page, or null for a post of its own. */
  boostedBy: { name: string; emojis: readonly CustomEmojiImage[]; href: string } | null;
  /**
   * The author's display name (the username when there is none), handle (`@alice` for a local account, `@bob@example.com` for a remote one),
   * profile page, and avatar URL (undefined when the account has none, and the avatar shows its fallback),
   * with the custom emoji of the name.
   */
  author: {
    id: string;
    name: string;
    emojis: readonly CustomEmojiImage[];
    handle: string;
    href: string;
    avatar: ImageSources | undefined;
  };
  /** When the post was written: the ISO date for `<time>`, the relative time to show, and the full date and time. */
  time: TimeView;
  /** Who can see the post (ADR-0017): its level, for the level's icon, and the level's name. */
  visibility: { level: Visibility; label: string };
  /** The web client's address of the post (the boosted post for a boost), which its time links to. */
  path: string;
  /** For a post that was edited, the address of its edit history (FR-109); null for one that never was. */
  historyPath: string | null;
  /**
   * For a post of another server, its page there (its ActivityPub ID when it has none) and the host of that
   * page; null for a post of this instance.
   */
  original: { href: string; host: string } | null;
  /** The body, as the server sanitized it. */
  html: SanitizedHtml;
  /** Opens the links of the body to this instance's pages through the router (the body's `onLinkClick`). */
  onLinkClick: (event: MouseEvent, link: HTMLAnchorElement) => void;
  /** The custom emoji of the body and the content warning. */
  emojis: readonly CustomEmojiImage[];
  /** The language of the post (BCP 47), or undefined when the server does not know it. */
  lang: string | undefined;
  /** The author's content warning, or null when there is none. */
  contentWarning: string | null;
  /** The pictures, videos, and audio files that are ready to show. */
  media: MediaItem[];
  /** Whether the author marked the media sensitive; it covers the image of the link card too. */
  sensitive: boolean;
  /** The card of the first link in the post (ADR-0062), or null when it has none. */
  card: LinkCardView | null;
  /** How many times the post was boosted and favourited. */
  counts: { boosts: number; favourites: number };
};

/** What the link card of a post shows, for @my/ui's LinkCard. */
type LinkCardView = {
  href: string;
  title: string;
  description: string;
  siteName: string;
  lang: string | undefined;
  image: LinkCardImage | undefined;
  /** Who wrote the linked page and when, in one line; null when the page says neither. */
  byline: string | null;
};

/**
 * What the card of a link shows: the page's texts as plain text, the image through the media proxy, and a byline
 * of the verified author's account (or the author's name as the page gives it) and the date of publication.
 */
function useLinkCardView(card: LinkCard | undefined): LinkCardView | null {
  const t = useT();
  const intl = useIntl();
  if (card === undefined) return null;
  const author =
    card.author === undefined
      ? card.author_name
      : t("client:timeline.card.account", {
          name: card.author.display_name || card.author.username,
          handle: handleOf(card.author.acct),
        });
  const date =
    card.published_at === undefined
      ? undefined
      : intl.formatDate(Date.parse(card.published_at), { dateStyle: "medium" });
  let byline: string | null = null;
  if (author !== undefined && date !== undefined) byline = t("client:timeline.card.author_published", { author, date });
  else if (author !== undefined) byline = t("client:timeline.card.author", { author });
  else if (date !== undefined) byline = t("client:timeline.card.published", { date });
  return {
    href: card.url,
    title: card.title,
    description: card.description,
    siteName: card.site_name,
    lang: card.language,
    image:
      card.image === undefined
        ? undefined
        : {
            src: card.image.url,
            width: card.image.width,
            height: card.image.height,
            blurhash: card.image.blurhash,
            alt: card.image.description,
          },
    byline,
  };
}

/**
 * The pictures, videos, and audio files of a status that are ready to show (api/openapi.yaml, MediaAttachment). For a
 * picture, `preview_url` is the small still rendition of an upload and the media proxy's small one of a remote file; an
 * upload also has its renditions to choose from by size, still and moving. A video or audio file plays from `url`, with
 * its poster's renditions (an upload's, when it has one) as the poster. The blurhash stands in while it is concealed or loading.
 * The image viewer shows a picture at its original size: the large rendition of an upload, or `url` (the proxy's) of a remote file.
 */
export function mediaItems(attachments: readonly MediaAttachment[], lang: string | undefined): MediaItem[] {
  return attachments.flatMap((attachment): MediaItem[] => {
    if (attachment.state !== "ready") return [];
    const common = { id: attachment.id, alt: attachment.description ?? null, lang, blurhash: attachment.blurhash };
    switch (attachment.type) {
      case "image":
        return attachment.preview_url === undefined
          ? []
          : [
              {
                ...common,
                src: attachment.preview_url,
                sources: imageSources(attachment.variants),
                original: originalImage(attachment.variants, attachment.url, attachment.meta),
              },
            ];
      case "video":
      case "audio":
      case "gifv":
        return attachment.url === undefined
          ? []
          : [
              {
                ...common,
                // The poster: the large still rendition fills the cell as a video would.
                src: attachment.variants?.large.url ?? attachment.preview_url,
                sources: imageSources(attachment.variants),
                playback: { kind: attachment.type, url: attachment.url },
              },
            ];
      default:
        return [];
    }
  });
}

/** The page of a remote post on its server and that page's host; null for a local post. */
function originalOf(post: Status): StatusView["original"] {
  if (!isRemoteAcct(post.account.acct)) return null;
  const href = statusShareUrl(post);
  // The server gives absolute URIs (format: uri); the account's domain stands in should one not parse.
  return { href, host: hostOf(href, post.account.domain) };
}

/** Returns what the card of `status` shows. A boost shows the boosted post, with the booster above it. */
export function useStatusView(status: Status): StatusView {
  const t = useT();
  const post = status.boost ?? status;
  const lang = post.language === "und" ? undefined : post.language;
  const time = useTimeView(post.created_at);
  const onLinkClick = useContentLinkClick();
  const card = useLinkCardView(post.card);
  return {
    boostedBy:
      status.boost === undefined
        ? null
        : {
            name: status.account.display_name || status.account.username,
            emojis: status.account.emojis,
            href: profilePath(status.account.acct),
          },
    author: {
      id: post.account.id,
      name: post.account.display_name || post.account.username,
      emojis: post.account.emojis,
      handle: handleOf(post.account.acct),
      href: profilePath(post.account.acct),
      avatar: imageSources(post.account.avatar),
    },
    time,
    visibility: { level: post.visibility, label: t(visibilityLabel[post.visibility]) },
    path: statusPath(post),
    historyPath: post.edited_at === undefined ? null : statusHistoryPath(post),
    original: originalOf(post),
    // `content` is sanitized HTML (api/openapi.yaml, Status.content).
    html: serverSanitizedHtml(post.content),
    onLinkClick,
    emojis: post.emojis,
    lang,
    contentWarning: post.spoiler_text === "" ? null : post.spoiler_text,
    media: mediaItems(post.media_attachments, lang),
    sensitive: post.sensitive,
    card,
    counts: { boosts: post.boosts_count, favourites: post.favourites_count },
  };
}
