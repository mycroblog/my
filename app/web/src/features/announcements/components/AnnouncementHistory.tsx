// SPDX-License-Identifier: AGPL-3.0-only
// The list page of the announcements below its heading (web-client/screens/announcements.md, ADR-0125 §3): those that
// have started to show, the ended ones included, the latest first. Each is an article with its title as heading
// (`h2`), its state beside it in words (Unread, Ended; WCAG 1.4.1), its date, and its body, the server's sanitized
// HTML of the administrator's Markdown (web-client/README.md A8). An unread one that shows now has "Mark as read";
// once marked, the button goes and focus moves to its heading. "Show more" adds the next page and moves focus to the
// first announcement it added. Opened for one announcement (`targetId`, from its notification or a title of the
// panel), the list moves focus to its heading, which scrolls it into view, after the layout has focused the page's
// heading; when no page has it, it was withdrawn, which the top of the list says.
import {
  Button,
  Callout,
  Cluster,
  EmptyState,
  Heading,
  Hint,
  InlineError,
  PostContent,
  RegionError,
  serverSanitizedHtml,
  Skeleton,
  Stack,
  Tag,
  Text,
  VisuallyHidden,
} from "@my/ui";
import { useEffect, useId, useRef } from "react";
import { useIntl } from "react-intl";
import { useContentLinkClick } from "../../../lib/contentLinks";
import { useT } from "../../../lib/i18n";
import { useFocusHeadingAfterLift } from "../../../lib/useFocusAfterRemoval";
import type { Announcement } from "../api/announcements";
import { type AnnouncementHistory as History, useAnnouncementHistory } from "../hooks/useAnnouncementHistory";
import { AnnouncementTitle } from "./AnnouncementTitle";

/** The heading of the announcement of `id` in `list`. */
function headingOf(list: HTMLElement | null, id: string): HTMLElement | null {
  return list?.querySelector<HTMLElement>(`[data-row-id="${CSS.escape(id)}"] h2`) ?? null;
}

export function AnnouncementHistory({ targetId }: { targetId?: string | undefined }) {
  const t = useT();
  const history = useAnnouncementHistory(targetId);
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
      return <HistoryList history={history} targetId={targetId} />;
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

function HistoryList({ history, targetId }: { history: ReadyHistory; targetId: string | undefined }) {
  const t = useT();
  const listRef = useRef<HTMLUListElement>(null);
  const read = useFocusHeadingAfterLift(listRef, "h2");
  const { items, target } = history;
  // Focus moves to the announcement the page was opened for once, after the frame in which the layout focuses the
  // page's heading (useViewChange, on the router's event of the same commit).
  const focusedRef = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (target !== "found" || targetId === undefined || focusedRef.current === targetId) return;
    focusedRef.current = targetId;
    const frame = requestAnimationFrame(() => {
      headingOf(listRef.current, targetId)?.focus();
    });
    return () => {
      cancelAnimationFrame(frame);
    };
  }, [target, targetId]);
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
      {target === "missing" && (
        <Callout>
          <Text as="p">{t("client:announcements.withdrawn")}</Text>
        </Callout>
      )}
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
  const t = useT();
  const intl = useIntl();
  const headingId = useId();
  const onLinkClick = useContentLinkClick();
  const unread = !announcement.read && !announcement.ended;
  return (
    <Stack as="article" aria-labelledby={headingId} gap="3" border="lg" padding="4">
      <Stack gap="1">
        <Heading level={2} id={headingId} wrap="anywhere" focusable>
          <Cluster as="span" align="center" gap="2">
            <AnnouncementTitle announcement={announcement} />
            {unread && <Tag variant="solid">{t("client:announcements.unread")}</Tag>}
            {announcement.ended && <Tag>{t("client:announcements.ended")}</Tag>}
          </Cluster>
        </Heading>
        <Hint>
          <time dateTime={announcement.published_at}>
            {intl.formatDate(announcement.published_at, { dateStyle: "medium" })}
          </time>
        </Hint>
      </Stack>
      <PostContent html={serverSanitizedHtml(announcement.content)} onLinkClick={onLinkClick} />
      {announcement.ends_at !== undefined && (
        <Hint>
          {t(announcement.ended ? "client:announcements.ended_at" : "client:announcements.until", {
            date: intl.formatDate(announcement.ends_at, { dateStyle: "medium", timeStyle: "short" }),
          })}
        </Hint>
      )}
      {unread && (
        <div>
          <Button size="sm" isPending={isMarking} aria-describedby={headingId} onPress={onMarkRead}>
            {t("client:announcements.mark_read")}
          </Button>
        </div>
      )}
    </Stack>
  );
}
