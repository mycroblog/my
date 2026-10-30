// SPDX-License-Identifier: AGPL-3.0-only
// The heading text of an announcement (ADR-0125 §1): its title, the administrators' plain text in any language and
// direction (web-client/README.md Y10), or, for one written before titles, the day it started to show.
import { useIntl } from "react-intl";
import { useT } from "../../../lib/i18n";
import type { Announcement } from "../api/announcements";

/** The path of the list page opened at `announcement` (from a title or a notification). */
export function announcementPath(announcement: Pick<Announcement, "id">): string {
  return `/announcements?id=${encodeURIComponent(announcement.id)}`;
}

export function AnnouncementTitle({ announcement }: { announcement: Announcement }) {
  const t = useT();
  const intl = useIntl();
  if (announcement.title === "") {
    return t("client:announcements.heading", {
      date: intl.formatDate(announcement.published_at, { dateStyle: "medium" }),
    });
  }
  return <bdi dir="auto">{announcement.title}</bdi>;
}
