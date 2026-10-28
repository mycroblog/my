// SPDX-License-Identifier: AGPL-3.0-only
// The server's announcements above the home timeline (FR-726): a region with a heading, nothing when there are
// none. Each announcement is an article with its date as heading; an unread one says so in its heading, in words
// rather than by color alone (WCAG 1.4.1), and has a "Mark as read" button. Once marked, the button goes and focus
// moves to the announcement's heading (useFocusHeadingAfterLift). The body is the server's sanitized HTML of the
// administrator's Markdown (web-client/README.md A8), in the direction of its own text (Y10).
import {
  Badge,
  Button,
  Cluster,
  Heading,
  Hint,
  InlineError,
  PostContent,
  serverSanitizedHtml,
  Stack,
  Tag,
} from "@my/ui";
import { useId, useRef } from "react";
import { useIntl } from "react-intl";
import { useContentLinkClick } from "../../../lib/contentLinks";
import { useT } from "../../../lib/i18n";
import { useFocusHeadingAfterLift } from "../../../lib/useFocusAfterRemoval";
import type { Announcement } from "../api/announcements";
import { type Announcements as AnnouncementsState, useAnnouncements } from "../hooks/useAnnouncements";

export function Announcements() {
  const t = useT();
  const headingId = useId();
  const announcements = useAnnouncements();
  const listRef = useRef<HTMLUListElement>(null);
  const read = useFocusHeadingAfterLift(listRef);
  if (announcements.items.length === 0) return null;
  return (
    <Stack as="section" aria-labelledby={headingId} gap="3" border="bottom" padding="4">
      <Heading level={2} id={headingId}>
        <Cluster as="span" wrap={false} align="center" gap="2">
          {t("client:announcements.title")}
          {announcements.unread > 0 && (
            <Badge
              count={announcements.unread}
              label={t("client:announcements.unread_count", { count: announcements.unread })}
            />
          )}
        </Cluster>
      </Heading>
      {announcements.error !== undefined && (
        <InlineError alert size="inherit">
          {t("client:announcements.mark_read_failed", { reason: announcements.error })}
        </InlineError>
      )}
      <Stack as="ul" ref={listRef} gap="3">
        {announcements.items.map((announcement) => (
          <li key={announcement.id} data-row-id={announcement.id}>
            <AnnouncementItem announcement={announcement} state={announcements} onRead={read} />
          </li>
        ))}
      </Stack>
    </Stack>
  );
}

type AnnouncementItemProps = { announcement: Announcement; state: AnnouncementsState; onRead: (id: string) => void };

function AnnouncementItem({ announcement, state, onRead }: AnnouncementItemProps) {
  const t = useT();
  const intl = useIntl();
  const headingId = useId();
  const onLinkClick = useContentLinkClick();
  const date = intl.formatDate(announcement.published_at, { dateStyle: "medium" });
  return (
    <Stack as="article" aria-labelledby={headingId} gap="2" border="lg" padding="3">
      <Heading level={3} id={headingId} size="meta" tone="muted" focusable>
        <Cluster as="span" align="center" gap="2">
          <time dateTime={announcement.published_at}>{t("client:announcements.heading", { date })}</time>
          {!announcement.read && <Tag variant="solid">{t("client:announcements.unread")}</Tag>}
        </Cluster>
      </Heading>
      <PostContent html={serverSanitizedHtml(announcement.content)} onLinkClick={onLinkClick} />
      {announcement.ends_at !== undefined && (
        <Hint>
          {t("client:announcements.until", {
            date: intl.formatDate(announcement.ends_at, { dateStyle: "medium", timeStyle: "short" }),
          })}
        </Hint>
      )}
      {!announcement.read && (
        <div>
          <Button
            size="sm"
            isPending={state.markingIds.includes(announcement.id)}
            aria-describedby={headingId}
            onPress={() => {
              state.markRead(announcement, () => {
                onRead(announcement.id);
              });
            }}
          >
            {t("client:announcements.mark_read")}
          </Button>
        </div>
      )}
    </Stack>
  );
}
