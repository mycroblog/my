// SPDX-License-Identifier: AGPL-3.0-only
// The announcements screen below its heading (/admin/announcements, manage_announcements, FR-726,
// web-client/screens/admin-announcements.md): the link to the page that writes one, then the announcements in the
// admin mode's list form: a toolbar with the filter by where they are in their life (kept in the URL by the route,
// web-client/README.md S3), a table of one page, newest first, and previous / next. Each row names the announcement
// (its title, a link to its page, where it is published, edited and withdrawn), its phase and the time it shows; no
// row renders the announcement's text.
import { ButtonLink, EmptyState, Hint, Link, ListToolbar, Select, SelectItem, Stack, Text } from "@my/ui";
import { useIntl } from "react-intl";
import { useNow } from "../../../lib/clock";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { type RecordColumns, RecordTable, RequirePermission } from "../../moderation";
import { type AdminAnnouncement, type AnnouncementFilter, announcementFilters } from "../api/announcements";
import {
  announcementLabel,
  announcementPhase,
  announcementPhaseLabels,
  announcementShowsFrom,
  useAdminAnnouncements,
} from "../hooks/useAnnouncements";

const filterLabels: Readonly<Record<AnnouncementFilter, ClientMessageKey>> = {
  all: "client:admin.announcements.filter.all",
  draft: "client:admin.announcements.filter.draft",
  scheduled: "client:admin.announcements.filter.scheduled",
  showing: "client:admin.announcements.filter.showing",
  ended: "client:admin.announcements.filter.ended",
  withdrawn: "client:admin.announcements.filter.withdrawn",
};

function isFilter(key: unknown): key is AnnouncementFilter {
  return announcementFilters.some((filter) => filter === key);
}

/** The address of an announcement's page. */
function announcementPath(id: string): string {
  return `/admin/announcements/${encodeURIComponent(id)}`;
}

export type AnnouncementListProps = {
  /** The selected list, from the URL. */
  filter: AnnouncementFilter;
  /** Selects another list: the route puts it into the URL. */
  onFilterChange: (filter: AnnouncementFilter) => void;
};

export function AnnouncementList(props: AnnouncementListProps) {
  return (
    <RequirePermission permission="manage_announcements">
      <AnnouncementsView {...props} />
    </RequirePermission>
  );
}

function AnnouncementsView({ filter, onFilterChange }: AnnouncementListProps) {
  const t = useT();
  return (
    <Stack gap="6">
      <Hint size="inherit">{t("client:admin.announcements.description")}</Hint>
      <div>
        <ButtonLink href="/admin/announcements/new" variant="primary">
          {t("client:admin.announcements.new.title")}
        </ButtonLink>
      </div>
      <Stack gap="4">
        <ListToolbar label={t("client:admin.list.filters")}>
          <Select
            variant="toolbar"
            aria-label={t("client:admin.announcements.filter.label")}
            value={filter}
            onChange={(key) => {
              if (isFilter(key)) onFilterChange(key);
            }}
          >
            {announcementFilters.map((item) => (
              <SelectItem key={item} id={item}>
                {t(filterLabels[item])}
              </SelectItem>
            ))}
          </Select>
        </ListToolbar>
        {/* Another list starts at its first page. */}
        <Announcements key={filter} filter={filter} />
      </Stack>
    </Stack>
  );
}

function Announcements({ filter }: { filter: AnnouncementFilter }) {
  const t = useT();
  const list = useAdminAnnouncements(filter);
  const now = useNow();
  const empty =
    filter === "all" ? (
      <EmptyState
        headingLevel={2}
        heading={t("client:admin.announcements.empty")}
        description={t("client:admin.announcements.empty.description")}
      />
    ) : (
      <EmptyState headingLevel={2} heading={t("client:admin.announcements.empty.filtered")} />
    );
  const columns: RecordColumns<AdminAnnouncement> = [
    {
      id: "announcement",
      header: t("client:admin.announcements.column.announcement"),
      wrap: "anywhere",
      cell: (announcement) => (
        <Link href={announcementPath(announcement.id)} variant="standalone" wrap="anywhere">
          {/* Written by an administrator in any language (web-client/README.md Y10). */}
          <span dir="auto">{announcementLabel(announcement)}</span>
        </Link>
      ),
    },
    {
      id: "phase",
      header: t("client:admin.announcements.state"),
      cell: (announcement) => (
        <Text weight="semibold">{t(announcementPhaseLabels[announcementPhase(announcement, now)])}</Text>
      ),
    },
    {
      id: "period",
      header: t("client:admin.announcements.column.period"),
      cell: (announcement) => <Period announcement={announcement} />,
    },
  ];
  return (
    <RecordTable
      list={list}
      caption={t(filterLabels[filter])}
      columns={columns}
      getId={(announcement) => announcement.id}
      headingLevel={2}
      empty={empty}
    />
  );
}

/** When an announcement shows: from its publication or the start of its window, until its end or its withdrawal. */
function Period({ announcement }: { announcement: AdminAnnouncement }) {
  const t = useT();
  const intl = useIntl();
  const format = (value: string) => intl.formatDate(value, { dateStyle: "medium", timeStyle: "short" });
  const from = announcementShowsFrom(announcement) ?? announcement.starts_at;
  return (
    <Stack as="span">
      <span>
        {from === undefined
          ? t("client:admin.announcements.period.on_publication")
          : t("client:admin.announcements.period.from", { date: format(from) })}
      </span>
      <span>
        {announcement.ends_at === undefined
          ? t("client:admin.announcements.until_withdrawn")
          : t("client:admin.announcements.period.until", { date: format(announcement.ends_at) })}
      </span>
    </Stack>
  );
}
