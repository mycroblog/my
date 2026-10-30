// SPDX-License-Identifier: AGPL-3.0-only
import { Box, Cluster, EmptyState, Feed, Link, RegionError, Stack, VisuallyHidden } from "@my/ui";
import type { ReactNode, Ref } from "react";
import { useT } from "../../../lib/i18n";
import type { Status, Timeline } from "../api/timelines";
import { useTimeline } from "../hooks/useTimeline";
import { StatusCard } from "../../status";

export type TimelineFeedProps = {
  timeline: Timeline;
  /** The ID of the page heading that names the timeline. */
  "aria-labelledby": string;
  /**
   * What the timeline holds of a source that has more elsewhere, and the link to it (a remote account's
   * profile, ADR-0044): shown above the posts, and in the empty state when there are none.
   */
  origin?: { notice: string; link: ReactNode };
  /** The posts pinned to the top of the timeline (a profile's, FR-403), before its own posts. */
  pinned?: readonly Status[];
  /** A ref to the feed element, and how the feed keeps the reader's place when posts are inserted above (the home timeline's). */
  feedRef?: Ref<HTMLDivElement> | undefined;
  keepPosition?: boolean | undefined;
  onTopChange?: ((atTop: boolean) => void) | undefined;
};

/** No pinned posts: one array for every render, so that the default is stable. */
const noPosts: readonly Status[] = [];

/**
 * A timeline as a feed (web-client/README.md Y6) with its older pages loaded as the reader nears the end, or an
 * empty state. The first page comes from the route loader; render it inside the route that loads it.
 */
export function TimelineFeed({
  timeline,
  "aria-labelledby": labelledBy,
  origin,
  pinned = noPosts,
  feedRef,
  keepPosition,
  onTopChange,
}: TimelineFeedProps) {
  const t = useT();
  const { statuses, hasMore, isLoadingMore, loadMore, loadMoreError } = useTimeline(timeline);
  // A pinned post is in the timeline too, further down, so its entry at the top has a key of its own.
  const entries = [
    ...pinned.map((status) => ({ key: `pinned:${status.id}`, status, pinned: true })),
    ...statuses.map((status) => ({ key: status.id, status, pinned: false })),
  ];

  if (entries.length === 0 && !hasMore) return <TimelineEmpty timeline={timeline} origin={origin} />;

  return (
    <Stack>
      {origin !== undefined && (
        <Cluster
          align="baseline"
          gap="3"
          rowGap="1"
          border="bottom"
          paddingX="4"
          paddingY="3"
          textSize="sm"
          tone="muted"
        >
          <p>{origin.notice}</p>
          <p>{origin.link}</p>
        </Cluster>
      )}
      {/* The feed role has no keyboard convention, so its keys are told before it (Feed's documentation). */}
      <VisuallyHidden>
        <p>{t("client:timeline.keyboard_hint")}</p>
      </VisuallyHidden>
      <Feed
        aria-labelledby={labelledBy}
        feedRef={feedRef}
        keepPosition={keepPosition}
        onTopChange={onTopChange}
        items={entries}
        getKey={(entry) => entry.key}
        renderItem={(entry) => <StatusCard status={entry.status} pinned={entry.pinned} />}
        isLoading={isLoadingMore}
        // After a failure the feed stops loading by itself; the retry button below asks again.
        hasMore={hasMore && loadMoreError === null}
        {...(hasMore && { setSize: -1 })}
        onLoadMore={loadMore}
        loadMoreLabel={t("client:timeline.load_more")}
        loadingLabel={t("client:timeline.loading")}
      />
      {loadMoreError !== null && (
        // Announced by useTimeline when it fails; shown here, under the posts already loaded, in the one form of a
        // region's failure (web-client/16-look-states-and-wording.md G16).
        <Box padding="4">
          <RegionError
            heading={t("client:timeline.load_more.failed.heading")}
            reason={loadMoreError}
            retryLabel={t("client:timeline.retry")}
            onRetry={loadMore}
            isRetrying={isLoadingMore}
          />
        </Box>
      )}
    </Stack>
  );
}

function TimelineEmpty({ timeline, origin }: { timeline: Timeline; origin: TimelineFeedProps["origin"] }) {
  const t = useT();
  switch (timeline.kind) {
    case "home":
      return (
        <EmptyState
          heading={t("client:timeline.empty.home.heading")}
          description={t("client:timeline.empty.home.description")}
          action={
            <Link href="/local" variant="standalone">
              {t("client:timeline.empty.home.action")}
            </Link>
          }
        />
      );
    case "local":
      return (
        <EmptyState
          heading={t("client:timeline.empty.local.heading")}
          description={t("client:timeline.empty.local.description")}
        />
      );
    case "public":
      return (
        <EmptyState
          heading={t("client:timeline.empty.public.heading")}
          description={t("client:timeline.empty.public.description")}
        />
      );
    case "tag":
      return (
        <EmptyState
          heading={t("client:timeline.empty.tag.heading", { tag: timeline.tag })}
          description={t("client:timeline.empty.tag.description", { tag: timeline.tag })}
        />
      );
    case "list":
      // A list with members whose posts have not come yet; a list without members says so before the feed (features/lists).
      return (
        <EmptyState
          heading={t("client:timeline.empty.list.heading")}
          description={t("client:timeline.empty.list.description")}
        />
      );
    case "bookmarks":
      return (
        <EmptyState
          heading={t("client:timeline.empty.bookmarks.heading")}
          description={t("client:timeline.empty.bookmarks.description")}
        />
      );
    case "quotes":
      return (
        <EmptyState
          heading={t("client:timeline.empty.quotes.heading")}
          description={t("client:timeline.empty.quotes.description")}
        />
      );
    case "account":
      // A profile's timeline sits under the level-two heading of its tab.
      return (
        <EmptyState
          headingLevel={3}
          heading={t("client:timeline.empty.account.heading")}
          {...(origin !== undefined && { description: origin.notice, action: origin.link })}
        />
      );
  }
}
