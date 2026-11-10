// SPDX-License-Identifier: AGPL-3.0-only
import {
  Avatar,
  Button,
  ContentWarning,
  EmojiText,
  FilteredPost,
  Hint,
  LanguageIcon,
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
  PostContentNodes,
  Stack,
  Text,
} from "@my/ui";
import { type Ref, useEffect, useId, useRef, useState } from "react";
import { ErrorBoundary } from "react-error-boundary";
import { useIntl } from "react-intl";
import { useBoundaryErrorReport } from "../../../lib/errorReporting";
import { useT } from "../../../lib/i18n";
import type { Status } from "../api/status";
import { useMediaViewerLabels } from "../hooks/useMediaViewerLabels";
import { useAudioPlayerLabels } from "../hooks/useAudioPlayerLabels";
import { useLinkCardSize } from "../hooks/useLinkCardSize";
import { translatedView, useStatusTranslation } from "../hooks/useStatusTranslation";
import { useFilteredLabel } from "../hooks/useFilteredLabel";
import { useStatusView } from "../hooks/useStatusView";
import { StatusActions } from "./StatusActions";
import { QuotedStatus } from "./QuotedStatus";
import { StatusPoll } from "./StatusPoll";
import { StatusReactions } from "./StatusReactions";
import { StatusTranslationNote } from "./StatusTranslationNote";

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
  /**
   * The report that points at the post, on a moderator's page about it: the card offers its translation through the
   * report (ADR-0148 §13), also without the viewer's actions.
   */
  report?: string;
};

/**
 * One status: the hook's view of it placed into the slots of PostCard, with the action bar. Its names link
 * to the profiles and its time to the post's page (FR-113). A post that the viewer's keyword filters fold (its
 * `filtered`; a boost's row carries the boosted post's) shows only what it matched and the button that shows it
 * (ADR-0157 §2); once shown, the card takes its place and focus moves to the card's article. The card folds again
 * when it is mounted again.
 */
export function StatusCard({ ref, ...props }: StatusCardProps) {
  const t = useT();
  const label = useFilteredLabel(props.status.filtered);
  // `revealed` once the button showed the post, which moves focus to its article.
  const [shown, setShown] = useState<"folded" | "revealed">("folded");
  const articleId = useId();
  useEffect(() => {
    // Focus is moved in the DOM once the card has replaced the folded line (the button that had it is gone).
    if (shown === "revealed") document.getElementById(articleId)?.focus();
  }, [shown, articleId]);
  if (label !== null && shown === "folded") {
    return (
      <FilteredPost
        ref={ref}
        label={label}
        showLabel={t("client:filters.folded.show")}
        onShow={() => {
          setShown("revealed");
        }}
      />
    );
  }
  return <StatusCardContent {...props} {...(ref !== undefined && { ref })} {...(label !== null && { articleId })} />;
}

function StatusCardContent({
  status,
  variant = "compact",
  thread,
  onDeleted,
  withActions = true,
  ref,
  pinned = false,
  report,
  articleId,
}: StatusCardProps & { articleId?: string }) {
  const t = useT();
  const intl = useIntl();
  const view = useStatusView(status);
  const translation = useStatusTranslation(status, withActions || report !== undefined, report);
  const { shown, action } = translation;
  const text = translatedView(view, shown);
  // Where the focus goes once the controls of the translation have changed: the line or the button that replaced the
  // other, or the toggle of the actions row (or, in a narrow card, the button of the menu that holds it).
  const baseId = useId();
  const translateToggle = `${baseId}-translate`;
  const moreActions = `${baseId}-more`;
  const showOriginal = `${baseId}-original`;
  const translateButton = `${baseId}-translate-button`;
  // Set by the press, read once the translation has come or gone: the press itself cannot focus what it shows.
  const focusNextRef = useRef<"line" | "button" | "row" | null>(null);
  useEffect(() => {
    const next = focusNextRef.current;
    if (next === null || (next === "line") !== (shown !== null)) return;
    focusNextRef.current = null;
    if (next === "line") {
      document.getElementById(showOriginal)?.focus();
    } else if (next === "button") {
      document.getElementById(translateButton)?.focus();
    } else {
      // A narrow card hides the toggle (display: none), which then cannot take the focus.
      const toggle = document.getElementById(translateToggle);
      const shownToggle = toggle !== null && (typeof toggle.checkVisibility !== "function" || toggle.checkVisibility());
      (shownToggle ? toggle : document.getElementById(moreActions))?.focus();
    }
  }, [shown, showOriginal, translateButton, translateToggle, moreActions]);
  const viewerLabels = useMediaViewerLabels();
  const audioLabels = useAudioPlayerLabels();
  const reportBoundaryError = useBoundaryErrorReport();
  const linkCardSize = useLinkCardSize();

  const card =
    view.card === null ? undefined : (
      // The card is drawn from what another site declared: should it fail to render, only the card is lost
      // (web-client/README.md E1).
      <ErrorBoundary fallback={<Hint>{t("client:timeline.card.failed")}</Hint>} onError={reportBoundaryError}>
        <LinkCard
          href={view.card.href}
          size={linkCardSize}
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
    text.media.length > 0 ? (
      <MediaGrid
        items={text.media}
        sensitive={view.sensitive}
        noDescriptionLabel={t("client:timeline.media.no_description")}
        revealLabel={t("client:timeline.media.reveal")}
        viewerLabels={viewerLabels}
        audioLabels={audioLabels}
        more={view.moreMedia ?? undefined}
      />
    ) : (
      card
    );
  // The poll goes with the body, under the content warning too (FR-112).
  const poll =
    (status.boost ?? status).poll === undefined ? undefined : (
      <StatusPoll
        status={status}
        canVote={withActions}
        translation={shown === null ? null : { titles: shown.pollOptions, lang: shown.language }}
      />
    );
  // The quoted post goes with the body too, after the media (FR-114).
  const quoted = (status.boost ?? status).quote;
  const quote = quoted === undefined ? undefined : <QuotedStatus quote={quoted} />;
  const extras = [poll, media, quote].filter((part) => part !== undefined);
  const bodyProps = {
    lang: text.lang,
    emojis: view.emojis,
    onLinkClick: view.onLinkClick,
    ...(variant === "focused" && { size: "lg" as const }),
  };
  const body =
    text.body.kind === "html" ? (
      <PostContent html={text.body.html} {...bodyProps} />
    ) : (
      <PostContentNodes nodes={text.body.nodes} {...bodyProps} />
    );
  // The mark of an edited post leads to its revisions (FR-109); it stays out of the header's one row (G7).
  const edited =
    view.historyPath === null ? null : (
      <PostCardTimeLink href={view.historyPath}>{t("client:status.edited")}</PostCardTimeLink>
    );
  // Right after the text: the line about the translation, and on a card without the actions row (a moderator's), the
  // button that translates in its place until a translation shows (web-client §13).
  // The viewer's own reply that its parent's author has not approved (ADR-0155 §7): only they see it, and it says so.
  const approval = (status.boost ?? status).approval;
  const approvalNote =
    approval === undefined ? null : (
      <Text tone={approval === "pending" ? "warning" : "danger"}>
        {t(approval === "pending" ? "client:status.approval.pending" : "client:status.approval.rejected")}
      </Text>
    );
  const translationLine = action !== null && (
    <>
      {!withActions && shown === null && (
        <Button
          id={translateButton}
          variant="subtle"
          size="toolbar"
          isPending={action.isPending}
          onPress={() => {
            action.onPress();
            focusNextRef.current = "line";
          }}
        >
          <LanguageIcon />
          {action.label}
        </Button>
      )}
      <StatusTranslationNote
        shown={shown}
        // The button says it itself on a card without the actions row.
        progress={withActions ? translation.progress : null}
        failure={translation.failure}
        onShowOriginal={() => {
          action.onPress();
          focusNextRef.current = withActions ? "row" : "button";
        }}
        showOriginalId={showOriginal}
      />
    </>
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
      // The article of a post shown from its fold takes focus then; outside a feed only by script (a feed's are tabbable).
      {...(articleId !== undefined && { id: articleId, tabIndex: -1 })}
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
            {approvalNote}
          </>
        ),
      })}
      {...(variant === "compact" &&
        (edited !== null || approvalNote !== null) && {
          details: (
            <>
              {edited}
              {approvalNote}
            </>
          ),
        })}
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
        <>
          {text.contentWarning === null ? (
            body
          ) : (
            <ContentWarning
              summary={<EmojiText text={text.contentWarning} emojis={view.emojis} />}
              summaryLang={text.contentWarningLang}
              toggleLabel={t("client:timeline.content_warning.toggle")}
            >
              {body}
              {poll}
              {media}
              {quote}
            </ContentWarning>
          )}
          {/* After the text, outside the warning, before the poll and the media. */}
          {translationLine}
        </>
      }
      {...(text.contentWarning === null &&
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
                translation={action}
                translateId={translateToggle}
                moreId={moreActions}
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
