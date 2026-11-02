// SPDX-License-Identifier: AGPL-3.0-only
// What the list page and the page of one announcement show alike of an announcement: its state in words (Unread,
// Ended; WCAG 1.4.1), the day it started to show, and its body, the server's sanitized HTML of the administrator's
// Markdown (web-client/README.md A8), with the end of its window and, while it is unread and shows now, "Mark as read".
import { Button, Hint, PostContent, serverSanitizedHtml, Tag } from "@my/ui";
import { useIntl } from "react-intl";
import { useContentLinkClick } from "../../../lib/contentLinks";
import { useT } from "../../../lib/i18n";
import type { Announcement } from "../api/announcements";

/** Whether the viewer can still mark the announcement read. */
function isUnread(announcement: Announcement): boolean {
  return !announcement.read && !announcement.ended;
}

/** The announcement's state; nothing for a read one that shows now. */
export function AnnouncementState({ announcement }: { announcement: Announcement }) {
  const t = useT();
  return (
    <>
      {isUnread(announcement) && <Tag variant="solid">{t("client:announcements.unread")}</Tag>}
      {announcement.ended && <Tag>{t("client:announcements.ended")}</Tag>}
    </>
  );
}

/** The day the announcement started to show. */
export function AnnouncementDate({ announcement }: { announcement: Announcement }) {
  const intl = useIntl();
  return (
    <Hint>
      <time dateTime={announcement.published_at}>
        {intl.formatDate(announcement.published_at, { dateStyle: "medium" })}
      </time>
    </Hint>
  );
}

type AnnouncementBodyProps = {
  announcement: Announcement;
  /** The ID of the announcement's heading, which describes the button. */
  headingId: string;
  isMarking: boolean;
  onMarkRead: () => void;
};

/** The children of the article below its heading and date. */
export function AnnouncementBody({ announcement, headingId, isMarking, onMarkRead }: AnnouncementBodyProps) {
  const t = useT();
  const intl = useIntl();
  const onLinkClick = useContentLinkClick();
  return (
    <>
      <PostContent html={serverSanitizedHtml(announcement.content)} onLinkClick={onLinkClick} />
      {announcement.ends_at !== undefined && (
        <Hint>
          {t(announcement.ended ? "client:announcements.ended_at" : "client:announcements.until", {
            date: intl.formatDate(announcement.ends_at, { dateStyle: "medium", timeStyle: "short" }),
          })}
        </Hint>
      )}
      {isUnread(announcement) && (
        <div>
          <Button size="sm" isPending={isMarking} aria-describedby={headingId} onPress={onMarkRead}>
            {t("client:announcements.mark_read")}
          </Button>
        </div>
      )}
    </>
  );
}
