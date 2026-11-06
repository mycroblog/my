// SPDX-License-Identifier: AGPL-3.0-only
// The appeals page of the automated reviews screen below its heading and local navigation (ADR-0147 §9): the authors'
// appeals of the posts that the review of local posts refused, newest first, in the admin mode's table form: a toolbar
// with the filter (pending by default, kept in the URL by the route, web-client/README.md S3), one page of the table at
// a time, previous / next. Each row's first cell names the author and links to the appeal's page, where the appeal is
// read whole and decided (HookAppealDetail.tsx); below the link, the start of the text the author tried to post, in its
// own direction (Y10).
import { EmptyState, Link, ListToolbar, Select, SelectItem, Stack, Text } from "@my/ui";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { moderationHookAppealPath } from "../../../lib/paths";
import { type HookAppeal, type HookAppealFilter, hookAppealFilters } from "../api/hookDecisions";
import { hookAppealStateLabels, useHookAppeals } from "../hooks/useHookDecisions";
import { DateTime, type RecordColumns, RecordTable, RequirePermission } from "./parts";
import { AccountByIdLabel } from "./reportParts";

const filterLabels: Readonly<Record<HookAppealFilter, ClientMessageKey>> = {
  pending: "client:admin.hook_appeals.filter.pending",
  all: "client:admin.hook_appeals.filter.all",
};

function isFilter(key: unknown): key is HookAppealFilter {
  return hookAppealFilters.some((item) => item === key);
}

export type HookAppealsProps = {
  /** The selected filter, from the URL. */
  filter: HookAppealFilter;
  /** Selects another filter: the route puts it into the URL. */
  onFilterChange: (filter: HookAppealFilter) => void;
};

export function HookAppeals(props: HookAppealsProps) {
  return (
    <RequirePermission permission="manage_reports">
      <HookAppealsView {...props} />
    </RequirePermission>
  );
}

function HookAppealsView({ filter, onFilterChange }: HookAppealsProps) {
  const t = useT();
  return (
    <Stack gap="4">
      <ListToolbar label={t("client:admin.list.filters")}>
        <Select
          variant="toolbar"
          aria-label={t("client:admin.hook_appeals.filter.label")}
          value={filter}
          onChange={(key) => {
            if (isFilter(key)) onFilterChange(key);
          }}
        >
          {hookAppealFilters.map((item) => (
            <SelectItem key={item} id={item}>
              {t(filterLabels[item])}
            </SelectItem>
          ))}
        </Select>
      </ListToolbar>
      {/* Another filter is another list: it starts at its first page. */}
      <HookAppealList key={filter} filter={filter} />
    </Stack>
  );
}

function HookAppealList({ filter }: { filter: HookAppealFilter }) {
  const t = useT();
  const list = useHookAppeals(filter);
  const columns: RecordColumns<HookAppeal> = [
    {
      id: "account",
      header: t("client:admin.hook_appeals.column.account"),
      wrap: "anywhere",
      cell: (appeal) => <AppealName appeal={appeal} />,
    },
    {
      id: "state",
      header: t("client:admin.hook_appeals.column.state"),
      cell: (appeal) => <Text weight="semibold">{t(hookAppealStateLabels[appeal.state])}</Text>,
    },
    {
      id: "submitted_at",
      header: t("client:admin.hook_appeals.submitted_at"),
      cell: (appeal) => <DateTime value={appeal.submitted_at} />,
    },
  ];
  return (
    <RecordTable
      list={list}
      caption={t(filterLabels[filter])}
      columns={columns}
      getId={(appeal) => appeal.id}
      headingLevel={2}
      empty={
        <EmptyState
          headingLevel={2}
          heading={t(filter === "pending" ? "client:admin.hook_appeals.empty" : "client:admin.hook_appeals.empty_all")}
        />
      }
    />
  );
}

/** The name of an appeal: its author, a link to its page, and the start of the text they tried to post. */
function AppealName({ appeal }: { appeal: HookAppeal }) {
  return (
    <Stack as="span" gap="1">
      <Link href={moderationHookAppealPath(appeal.id)} variant="standalone" wrap="anywhere">
        <AccountByIdLabel id={appeal.account_id} />
      </Link>
      {/* Written by the author, in any language; the appeal's page shows it whole (Y10). */}
      <Text size="sm" dir="auto" lineClamp={2} align="start" wrap="anywhere" whitespace="preWrap">
        {appeal.status_text}
      </Text>
    </Stack>
  );
}
