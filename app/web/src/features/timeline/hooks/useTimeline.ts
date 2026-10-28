// SPDX-License-Identifier: AGPL-3.0-only
import { useNotice } from "@my/ui";
import { useSuspenseInfiniteQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { type Status, type Timeline, timelineQueryOptions } from "../api/timelines";

/** A timeline as the feed shows it. The statuses are read from the query cache on every render, never copied (S2). */
export type TimelineState = {
  /** The loaded statuses, newest first. */
  statuses: readonly Status[];
  /** Whether the server has older statuses. */
  hasMore: boolean;
  /** Whether older statuses are loading. */
  isLoadingMore: boolean;
  /** Loads the next page. A failure is announced and kept in `loadMoreError` until the next try. */
  loadMore: () => void;
  /** Why the last page failed to load (localized), or null. */
  loadMoreError: string | null;
};

/**
 * Reads a timeline whose first page the route loader has fetched, and loads the older pages on demand.
 * The first page's failure goes to the route's error boundary; a later page's failure stays here, so the
 * statuses already shown stay on screen (web-client/README.md E1, E2).
 */
export function useTimeline(timeline: Timeline): TimelineState {
  const api = useApi();
  const t = useT();
  const notice = useNotice();
  const query = useSuspenseInfiniteQuery(timelineQueryOptions(api, timeline));
  const loadMoreError = query.isFetchNextPageError ? errorMessage(query.error) : null;

  const loadMore = () => {
    if (!query.hasNextPage || query.isFetchingNextPage) return;
    void query.fetchNextPage().then((result) => {
      if (result.isFetchNextPageError) {
        notice.inPlace(t("client:timeline.load_more.failed", { reason: errorMessage(result.error) }), "assertive");
      }
    });
  };

  return {
    statuses: query.data.pages.flatMap((page) => page.items),
    hasMore: query.hasNextPage,
    isLoadingMore: query.isFetchingNextPage,
    loadMore,
    loadMoreError,
  };
}
