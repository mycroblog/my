// SPDX-License-Identifier: AGPL-3.0-only
import {
  Avatar,
  ContentWarning,
  EmojiText,
  Hint,
  LinkCard,
  MediaGrid,
  PostCard,
  PostCardAuthorLink,
  PostCardAvatarLink,
  PostCardContext,
  PostCardNameLink,
  PostCardOrigin,
  PostCardTimeLink,
  PostContent,
  Stack,
  Text,
} from "@my/ui";
import type { Ref } from "react";
import { ErrorBoundary } from "react-error-boundary";
import { useIntl } from "react-intl";
import { useBoundaryErrorReport } from "../../../lib/errorReporting";
import { useT } from "../../../lib/i18n";
import type { Status } from "../api/status";
import { useMediaViewerLabels } from "../hooks/useMediaViewerLabels";
import { useAudioPlayerLabels } from "../hooks/useAudioPlayerLabels";
import { useStatusView } from "../hooks/useStatusView";
import { StatusActions } from "./StatusActions";
import { QuotedStatus } from "./QuotedStatus";
import { StatusPoll } from "./StatusPoll";
import { StatusReactions } from "./StatusReactions";

export type StatusCardProps = {
  status: Status;
  /**
   * `focused` for the post a thread page is about: highlighted and named apart from the rest, its body
   * larger, its full date and its counts in words under the body, and no link to the page it is on.
   * `compact` (the default) for timelines and the rest of a thread.
   */
  variant?: "compact" | "focused";
  /** Whether the post replies to the post above it in a thread, and whether the post below replies to it. */
  thread?: { lineAbove: boolean; lineBelow: boolean };
  /** Called after the viewer deleted the post. */
  onDeleted?: () => void;
  /**
   * Whether the card has the viewer's actions on the post (reply, boost, favourite, the menu); true by default.
   * A moderator's page about the post (a report) shows it without them, so that nothing there acts on it by mistake.
   */
  withActions?: boolean;
  /** The card's article, for the page that scrolls it into view. */
  ref?: Ref<HTMLElement>;
  /** Whether the card is one of the pinned posts at the top of a profile, which a line above it says (FR-403). */
  pinned?: boolean;
};

/**
 * One status: the hook's view of it placed into the slots of PostCard, with the action bar. Its names link
 * to the profiles and its time to the post's page (FR-113).
 */
export function StatusCard({
  status,
  variant = "compact",
  thread,
  onDeleted,
  withActions = true,
  ref,
  pinned = false,
}: StatusCardProps) {
  const t = useT();
  const intl = useIntl();
  const view = useStatusView(status);
  const viewerLabels = useMediaViewerLabels();
  const audioLabels = useAudioPlayerLabels();
  const reportBoundaryError = useBoundaryErrorReport();

  const card =
    view.card === null ? undefined : (
      // The card is drawn from what another site declared: should it fail to render, only the card is lost
      // (web-client/README.md E1).
      <ErrorBoundary fallback={<Hint>{t("client:timeline.card.failed")}</Hint>} onError={reportBoundaryError}>
        <LinkCard
          href={view.card.href}
          title={view.card.title}
          description={view.card.description}
          siteName={view.card.siteName}
          lang={view.card.lang}
          image={view.card.image}
          {...(view.card.byline !== null && { byline: view.card.byline })}
          sensitive={view.sensitive}
          revealLabel={t("client:timeline.media.reveal")}
        />
      </ErrorBoundary>
    );
  // A post with attachments has no card (ADR-0062 §1); the card takes the place of the media.
  const media =
    view.media.length > 0 ? (
      <MediaGrid
        items={view.media}
        sensitive={view.sensitive}
        noDescriptionLabel={t("client:timeline.media.no_description")}
        revealLabel={t("client:timeline.media.reveal")}
        viewerLabels={viewerLabels}
        audioLabels={audioLabels}
      />
    ) : (
      card
    );
  // The poll goes with the body, under the content warning too (FR-112).
  const poll =
    (status.boost ?? status).poll === undefined ? undefined : <StatusPoll status={status} canVote={withActions} />;
  // The quoted post goes with the body too, after the media (FR-114).
  const quoted = (status.boost ?? status).quote;
  const quote = quoted === undefined ? undefined : <QuotedStatus quote={quoted} />;
  const extras = [poll, media, quote].filter((part) => part !== undefined);
  const body = (
    <PostContent
      html={view.html}
      lang={view.lang}
      emojis={view.emojis}
      onLinkClick={view.onLinkClick}
      {...(variant === "focused" && { size: "lg" as const })}
    />
  );
  // The mark of an edited post leads to its revisions (FR-109); it stays out of the header's one row (G7).
  const edited =
    view.historyPath === null ? null : (
      <PostCardTimeLink href={view.historyPath}>{t("client:status.edited")}</PostCardTimeLink>
    );
  const time =
    variant === "focused" ? (
      // The relative time as everywhere else (G8); the full date is in the details.
      <time dateTime={view.time.dateTime}>{view.time.relative}</time>
    ) : (
      <PostCardTimeLink href={view.path}>
        <time dateTime={view.time.dateTime} title={view.time.absolute}>
          {view.time.relative}
        </time>
      </PostCardTimeLink>
    );

  return (
    <PostCard
      ref={ref}
      {...(thread !== undefined && { lineAbove: thread.lineAbove, lineBelow: thread.lineBelow })}
      {...(variant === "focused" && {
        label: t("client:status.focused.label"),
        highlighted: true,
        details: (
          <>
            <time dateTime={view.time.dateTime}>{view.time.absolute}</time>
            {view.counts.replies > 0 && (
              <Text numeric>{t("client:status.focused.replies", { count: view.counts.replies })}</Text>
            )}
            {view.counts.boosts > 0 && (
              <Text numeric>{t("client:status.focused.boosts", { count: view.counts.boosts })}</Text>
            )}
            {view.counts.favourites > 0 && (
              <Text numeric>{t("client:status.focused.favourites", { count: view.counts.favourites })}</Text>
            )}
            {edited}
          </>
        ),
      })}
      {...(variant === "compact" && edited !== null && { details: edited })}
      {...(pinned && {
        // Part of the article's name, so that a reader of the feed hears it before the author.
        context: <PostCardContext kind="pin">{t("client:status.pin.label")}</PostCardContext>,
      })}
      {...(!pinned &&
        view.boostedBy !== null && {
          context: (
            <PostCardContext kind="boost">
              {/* The booster's name is the user's text, isolated from the sentence around it (Y10). */}
              {intl.formatMessage(
                { id: "client:timeline.boosted_by" },
                {
                  name: (
                    <PostCardNameLink key="name" href={view.boostedBy.href}>
                      <EmojiText text={view.boostedBy.name} emojis={view.boostedBy.emojis} />
                    </PostCardNameLink>
                  ),
                },
              )}
            </PostCardContext>
          ),
        })}
      avatar={
        <PostCardAvatarLink href={view.author.href}>
          <Avatar sources={view.author.avatar} name={view.author.name} seed={view.author.id} decorative />
        </PostCardAvatarLink>
      }
      author={
        <PostCardAuthorLink
          href={view.author.href}
          name={<EmojiText text={view.author.name} emojis={view.author.emojis} />}
          handle={view.author.handle}
        />
      }
      time={time}
      visibility={view.visibility}
      content={
        view.contentWarning === null ? (
          body
        ) : (
          <ContentWarning
            summary={<EmojiText text={view.contentWarning} emojis={view.emojis} />}
            summaryLang={view.lang}
            toggleLabel={t("client:timeline.content_warning.toggle")}
          >
            {body}
            {poll}
            {media}
            {quote}
          </ContentWarning>
        )
      }
      {...(view.contentWarning === null &&
        extras.length > 0 && {
          media:
            extras.length === 1 ? (
              extras[0]
            ) : (
              <Stack gap="3" shrink>
                {poll}
                {media}
                {quote}
              </Stack>
            ),
        })}
      {...(withActions &&
        (status.boost ?? status).reactions.length > 0 && { reactions: <StatusReactions status={status} /> })}
      {...((withActions || view.original !== null) && {
        actions: (
          <>
            {withActions && (
              <StatusActions
                status={status}
                {...(onDeleted !== undefined && { onDeleted })}
                // The focused post's replies are below it on its own page.
                {...(variant === "compact" && { repliesHref: view.path })}
              />
            )}
            {view.original !== null && (
              // The post's page on its own server, shown also while nobody is signed in. The host is what it
              // shows; the name says where it leads, and contains the host (WCAG 2.5.3, label in name).
              <PostCardOrigin
                href={view.original.href}
                host={view.original.host}
                label={t("client:status.original", { host: view.original.host })}
              />
            )}
          </>
        ),
      })}
    />
  );
}
