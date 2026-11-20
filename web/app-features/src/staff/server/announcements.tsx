// SPDX-License-Identifier: AGPL-3.0-only

// The server's announcements as a holder of `manage_announcements` keeps them (FR-726, ADR-0125, ADR-0058 §1): at
// /admin/announcements, every one, the newest first, a page at a time by the server's cursor, kept to one state where
// the address says (a draft, waiting for its window, showing now, over, withdrawn); each its title, as written, where it
// stands and when it shows, opening its page (announcement.tsx); and Write an announcement at the end of the list's
// heading, as every list of the administration has its way to a new one. Where the list has nothing to show, it says
// why.

import {
  announcementFilters,
  announcementStanding,
  loadAdminAnnouncements,
  useAdminAnnouncements,
  type AdminAnnouncement,
  type AdminAnnouncementFilter,
  type AdminAnnouncementQuery,
  type AppData,
} from "@my/app-data";
import {
  Pager,
  FeedNotice,
  FilterBar,
  FilterFacet,
  LinkButton,
  RecordTable,
  ScreenStack,
  ScreenTitle,
  SettingsSection,
  StateMark,
  type FilterValue,
  type IconName,
  type RecordRow,
  type RecordState,
} from "@my/design-system";
import { useState, type ReactNode } from "react";
import type { StaffEntryProps, StaffSearch } from "../entries";
import { cursorOf, filtered, useAddressPages, ReadNotice, useMoment, useStaffStrings, type T } from "../parts";

/** The address of the announcements. */
export const adminAnnouncementsPath = "/admin/announcements";
/** The address of a new announcement's form. */
const newAnnouncementPath = `${adminAnnouncementsPath}/new`;
/** The address of the announcement of `id`. */
export const adminAnnouncementPath = (id: string): string => `${adminAnnouncementsPath}/${id}`;

/** What the list shows at an address's query: every announcement unless it names a state. */
function announcementQueryOf(search: StaffSearch): AdminAnnouncementQuery {
  return { state: announcementFilters.find((f) => f === search["state"]), cursor: cursorOf(search) };
}

/** Reads what the list shows at `search`, for the route's loader. */
export const loadAnnouncementsScreen = (data: AppData, search: StaffSearch): Promise<void> =>
  loadAdminAnnouncements(data, announcementQueryOf(search));

/** The mark of each standing of an announcement, and its glyph in the filter. */
const standingLook: Record<AdminAnnouncementFilter, { readonly state: RecordState; readonly icon: IconName }> = {
  draft: { state: "open", icon: "drafts" },
  scheduled: { state: "pending", icon: "schedule" },
  showing: { state: "active", icon: "stateActive" },
  ended: { state: "expired", icon: "stateExpired" },
  withdrawn: { state: "disabled", icon: "stateDisabled" },
};

/** Where an announcement stands, in words. */
const standingWords = (t: T, standing: AdminAnnouncementFilter): string =>
  t("staff_announcements.standing", { standing });

/** Where an announcement stands, as its mark. */
export function StandingMark({ standing }: { standing: AdminAnnouncementFilter }): ReactNode {
  const { t } = useStaffStrings();
  return <StateMark state={standingLook[standing].state}>{standingWords(t, standing)}</StateMark>;
}

/** When an announcement shows, in words: its window, each side a moment never broken inside. */
export function useWindowWords(): (a: Pick<AdminAnnouncement, "starts_at" | "ends_at">) => string {
  const { t } = useStaffStrings();
  const moment = useMoment();
  return (a) => {
    if (a.starts_at !== undefined && a.ends_at !== undefined)
      return t("staff_announcements.window.both", { from: moment(a.starts_at), until: moment(a.ends_at) });
    if (a.starts_at !== undefined) return t("staff_announcements.window.from", { from: moment(a.starts_at) });
    if (a.ends_at !== undefined) return t("staff_announcements.window.until", { until: moment(a.ends_at) });
    return t("staff_announcements.window.open");
  };
}

type Column = "window" | "state";

/** The announcements: the filter, a page of them, and the pages. */
export function AdminAnnouncementsScreen({ search, onSearch }: StaffEntryProps): ReactNode {
  const pages = useAddressPages({ search, onSearch });
  const { t } = useStaffStrings();
  const windowWords = useWindowWords();
  // Where each stands is told at the moment the screen opened.
  const [now] = useState(() => Date.now());
  const q = announcementQueryOf(search);
  const page = useAdminAnnouncements(q);
  const values: FilterValue[] = announcementFilters.map((f) => ({
    id: f,
    label: standingWords(t, f),
    icon: standingLook[f].icon,
  }));
  const head = (
    <>
      <ScreenTitle glyph="announcements">{t("staff_announcements.entry")}</ScreenTitle>
      <FilterBar
        label={t("staff_announcements.filters")}
        applied={q.state !== undefined}
        onClear={() => {
          onSearch(filtered(search, { state: undefined }));
        }}
      >
        <FilterFacet
          label={t("staff_announcements.filter.state")}
          any={t("staff_announcements.filter.any")}
          values={values}
          chosen={q.state}
          onChoose={(state) => {
            onSearch(filtered(search, { state: announcementFilters.find((f) => f === state) }));
          }}
        />
      </FilterBar>
    </>
  );
  if (page.items === undefined)
    return (
      <ScreenStack>
        {head}
        <ReadNotice
          status={page.status === "failed" ? "error" : "pending"}
          loading={t("staff_announcements.loading")}
          failed={t("staff_announcements.failed")}
          retry={page.retry}
        />
      </ScreenStack>
    );
  const rows = page.items.map((a): RecordRow<Column> => ({
    id: a.id,
    head: a.title,
    href: adminAnnouncementPath(a.id),
    cells: {
      window: windowWords(a),
      state: <StandingMark standing={announcementStanding(a, now)} />,
    },
  }));
  return (
    <ScreenStack busy={page.stale}>
      {head}
      {/* As the other lists of the administration, the way to a new one ends the heading of the list. */}
      <SettingsSection
        heading={q.state === undefined ? t("staff_announcements.list.all") : standingWords(t, q.state)}
        action={
          <LinkButton size="timeline" href={newAnnouncementPath}>
            {t("staff_announcements.write")}
          </LinkButton>
        }
      >
        <RecordTable
          label={t("staff_announcements.table")}
          head={{ label: t("staff_announcements.column.title") }}
          columns={[
            { key: "window", label: t("staff_announcements.column.window"), bare: true },
            { key: "state", label: t("staff_announcements.column.state"), kind: "fit", beside: true },
          ]}
          rows={rows}
          busy={page.stale}
          even
          contained
          empty={
            <FeedNotice>
              {t(q.state === undefined ? "staff_announcements.empty" : "staff_announcements.empty.filtered")}
            </FeedNotice>
          }
        />
      </SettingsSection>
      <Pager {...pages.pager(page.next)} />
    </ScreenStack>
  );
}
