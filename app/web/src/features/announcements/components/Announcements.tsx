// SPDX-License-Identifier: AGPL-3.0-only
// The server's announcements above the home timeline below xl (FR-726, ADR-0125 §4; from xl they are in the panel of
// the third column instead): a region with a heading and the unread count, shown while one of them is unread. Each
// announcement is an article with its title as heading and its date under it; an unread one says so in its heading,
// in words rather than by color alone (WCAG 1.4.1), and has a "Mark as read" button. Once marked, the button goes and
// focus moves to the announcement's heading (useFocusHeadingAfterLift); once none is unread, the region goes and
// focus moves to the page's heading (`onAllRead`). The body is the server's sanitized HTML of the administrator's
// Markdown (web-client/README.md A8), in the direction of its own text (Y10). The region ends with a link to the
// list page, which has the ended ones too.
import {
  Badge,
  Button,
  Cluster,
  Heading,
  Hint,
  InlineError,
  Link,
  PostContent,
  Section,
  serverSanitizedHtml,
  Stack,
  Tag,
  Text,
} from "@my/ui";
import { useId, useRef } from "react";
import { useIntl } from "react-intl";
import { useContentLinkClick } from "../../../lib/contentLinks";
import { useT } from "../../../lib/i18n";
import { useFocusHeadingAfterLift } from "../../../lib/useFocusAfterRemoval";
import type { Announcement } from "../api/announcements";
import { type Announcements as AnnouncementsState, useAnnouncements } from "../hooks/useAnnouncements";
import { AnnouncementTitle } from "./AnnouncementTitle";

export type AnnouncementsProps = {
  /** Called once the last unread announcement is marked read and the region goes, to move focus elsewhere. */
  onAllRead: () => void;
};

export function Announcements({ onAllRead }: AnnouncementsProps) {
  const t = useT();
  const announcements = useAnnouncements();
  const listRef = useRef<HTMLUListElement>(null);
  const read = useFocusHeadingAfterLift(listRef);
  if (announcements.unread === 0) return null;
  return (
    <Section
      surface="band"
      title={
        <Cluster as="span" wrap={false} align="center" gap="2">
          {t("client:announcements.title")}
          <Badge
            count={announcements.unread}
            label={t("client:announcements.unread_count", { count: announcements.unread })}
          />
        </Cluster>
      }
    >
      {announcements.error !== undefined && (
        <InlineError alert size="inherit">
          {t("client:announcements.mark_read_failed", { reason: announcements.error })}
        </InlineError>
      )}
      <Stack as="ul" ref={listRef} gap="3">
        {announcements.items.map((announcement) => (
          <li key={announcement.id} data-row-id={announcement.id}>
            <AnnouncementItem
              announcement={announcement}
              state={announcements}
              onRead={(unread) => {
                if (unread === 0) onAllRead();
                else read(announcement.id);
              }}
            />
          </li>
        ))}
      </Stack>
      <Text as="p" size="sm">
        <Link href="/announcements" variant="standalone">
          {t("client:announcements.all")}
        </Link>
      </Text>
    </Section>
  );
}

type AnnouncementItemProps = {
  announcement: Announcement;
  state: AnnouncementsState;
  onRead: (unread: number) => void;
};

function AnnouncementItem({ announcement, state, onRead }: AnnouncementItemProps) {
  const t = useT();
  const intl = useIntl();
  const headingId = useId();
  const onLinkClick = useContentLinkClick();
  return (
    <Stack as="article" aria-labelledby={headingId} gap="2" border="lg" padding="3">
      <Stack gap="1">
        <Heading level={3} id={headingId} size="subsection" wrap="anywhere" focusable>
          <Cluster as="span" align="center" gap="2">
            <AnnouncementTitle announcement={announcement} />
            {!announcement.read && <Tag variant="solid">{t("client:announcements.unread")}</Tag>}
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
              state.markRead(announcement, onRead);
            }}
          >
            {t("client:announcements.mark_read")}
          </Button>
        </div>
      )}
    </Stack>
  );
}
