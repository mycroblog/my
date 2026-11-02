// SPDX-License-Identifier: AGPL-3.0-only
// The list page of the announcements below its heading (web-client/screens/announcements.md, ADR-0125 §3): those that
// have started to show, the ended ones included, the latest first. Each is an article with its title as heading
// (`h2`), a link to the announcement's own page, with its state beside it, and its date and body (AnnouncementParts).
// An unread one that shows now has "Mark as read"; once marked, the button goes and focus moves to its heading. "Show
// more" adds the next page and moves focus to the first announcement it added.
import {
  Button,
  Cluster,
  EmptyState,
  Heading,
  InlineError,
  Link,
  RegionError,
  Skeleton,
  Stack,
  VisuallyHidden,
} from "@my/ui";
import { useEffect, useId, useRef } from "react";
import { useT } from "../../../lib/i18n";
import { useFocusHeadingAfterLift } from "../../../lib/useFocusAfterRemoval";
import type { Announcement } from "../api/announcements";
import { type AnnouncementHistory as History, useAnnouncementHistory } from "../hooks/useAnnouncementHistory";
import { AnnouncementBody, AnnouncementDate, AnnouncementState } from "./AnnouncementParts";
import { AnnouncementTitle, announcementPath } from "./AnnouncementTitle";

/** The heading of the announcement of `id` in `list`. */
function headingOf(list: HTMLElement | null, id: string): HTMLElement | null {
  return list?.querySelector<HTMLElement>(`[data-row-id="${CSS.escape(id)}"] h2`) ?? null;
}

export function AnnouncementHistory() {
  const t = useT();
  const history = useAnnouncementHistory();
  switch (history.status) {
    case "loading":
      return <AnnouncementHistorySkeleton />;
    case "error":
      return (
        <RegionError
          heading={t("client:announcements.list.failed")}
          reason={history.reason}
          retryLabel={t("client:announcements.retry")}
          onRetry={history.retry}
          isRetrying={history.isRetrying}
        />
      );
    case "ready":
      return <HistoryList history={history} />;
  }
}

/** The shape of the list while its first page loads (web-client/16-look-states-and-wording.md G15). */
export function AnnouncementHistorySkeleton() {
  const t = useT();
  return (
    <Stack gap="4" aria-busy="true">
      <VisuallyHidden>{t("client:announcements.loading")}</VisuallyHidden>
      <Skeleton shape="rect" />
      <Skeleton shape="rect" />
    </Stack>
  );
}

type ReadyHistory = Extract<History, { status: "ready" }>;

function HistoryList({ history }: { history: ReadyHistory }) {
  const t = useT();
  const listRef = useRef<HTMLUListElement>(null);
  const read = useFocusHeadingAfterLift(listRef, "h2");
  const { items } = history;
  // "Show more": once the next page has rendered, focus moves to the first announcement it added.
  const firstAddedRef = useRef<number | null>(null);
  useEffect(() => {
    const index = firstAddedRef.current;
    if (index === null || items.length <= index) return;
    firstAddedRef.current = null;
    const added = items[index];
    if (added !== undefined) headingOf(listRef.current, added.id)?.focus();
  }, [items]);
  if (items.length === 0 && !history.hasMore) {
    return (
      <EmptyState
        headingLevel={2}
        heading={t("client:announcements.empty")}
        description={t("client:announcements.empty.description")}
      />
    );
  }
  return (
    <Stack gap="4">
      {history.markError !== undefined && (
        <InlineError alert size="inherit">
          {t("client:announcements.mark_read_failed", { reason: history.markError })}
        </InlineError>
      )}
      <Stack as="ul" ref={listRef} gap="4">
        {items.map((announcement) => (
          <li key={announcement.id} data-row-id={announcement.id}>
            <HistoryItem
              announcement={announcement}
              isMarking={history.markingIds.includes(announcement.id)}
              onMarkRead={() => {
                history.markRead(announcement, () => {
                  read(announcement.id);
                });
              }}
            />
          </li>
        ))}
      </Stack>
      {history.moreError !== undefined && (
        <RegionError heading={t("client:announcements.more_failed")} reason={history.moreError} />
      )}
      {history.hasMore && (
        <div>
          <Button
            isPending={history.isLoadingMore}
            onPress={() => {
              firstAddedRef.current = items.length;
              history.loadMore();
            }}
          >
            {t("client:announcements.more")}
          </Button>
        </div>
      )}
    </Stack>
  );
}

type HistoryItemProps = { announcement: Announcement; isMarking: boolean; onMarkRead: () => void };

function HistoryItem({ announcement, isMarking, onMarkRead }: HistoryItemProps) {
  const headingId = useId();
  return (
    <Stack as="article" aria-labelledby={headingId} gap="3" border="lg" padding="4">
      <Stack gap="1">
        <Heading level={2} id={headingId} wrap="anywhere" focusable>
          <Cluster as="span" align="center" gap="2">
            <Link href={announcementPath(announcement)} wrap="anywhere">
              <AnnouncementTitle announcement={announcement} />
            </Link>
            <AnnouncementState announcement={announcement} />
          </Cluster>
        </Heading>
        <AnnouncementDate announcement={announcement} />
      </Stack>
      <AnnouncementBody
        announcement={announcement}
        headingId={headingId}
        isMarking={isMarking}
        onMarkRead={onMarkRead}
      />
    </Stack>
  );
}
