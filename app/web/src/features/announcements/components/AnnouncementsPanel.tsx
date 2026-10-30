// SPDX-License-Identifier: AGPL-3.0-only
// The panel of the third column from xl (web-client/09-app-shell.md §9, ADR-0125 §4), at its top, on every page but
// the list page: the announcements that show now, by their titles. Its heading has the unread count beside it; the
// unread ones come first, each kind the latest first, and the first three show, each a link to it on the list page
// with "Unread" in words when unread (WCAG 1.4.1). The bodies and "Mark as read" are on the list page only. Absent
// while there are none, while they load, and when they cannot be loaded.
import { AsidePanel, Badge, Cluster, Link, Stack, Tag, Text } from "@my/ui";
import { useT } from "../../../lib/i18n";
import { useAnnouncements } from "../hooks/useAnnouncements";
import { AnnouncementTitle, announcementPath } from "./AnnouncementTitle";

/** How many announcements the panel shows. */
const panelLimit = 3;

export function AnnouncementsPanel() {
  const t = useT();
  const { items, unread } = useAnnouncements();
  if (items.length === 0) return null;
  // The list is the latest first; a stable sort keeps that order within the unread and the read ones.
  const shown = items.toSorted((a, b) => Number(a.read) - Number(b.read)).slice(0, panelLimit);
  return (
    <AsidePanel
      heading={
        <Cluster as="span" wrap={false} align="center" gap="2">
          {t("client:announcements.title")}
          {unread > 0 && <Badge count={unread} label={t("client:announcements.unread_count", { count: unread })} />}
        </Cluster>
      }
    >
      <Stack as="ul" gap="2">
        {shown.map((announcement) => (
          <li key={announcement.id}>
            {/* "Unread" is part of the link's name, so that it is heard on the way through the links. */}
            <Link href={announcementPath(announcement)} variant="standalone" wrap="anywhere">
              <Cluster as="span" gap="2" align="center">
                <AnnouncementTitle announcement={announcement} />
                {!announcement.read && <Tag variant="solid">{t("client:announcements.unread")}</Tag>}
              </Cluster>
            </Link>
          </li>
        ))}
      </Stack>
      <Text as="p" size="sm">
        <Link href="/announcements" variant="standalone">
          {t("client:announcements.all")}
        </Link>
      </Text>
    </AsidePanel>
  );
}
