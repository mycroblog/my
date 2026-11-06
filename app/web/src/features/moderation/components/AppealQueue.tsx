// SPDX-License-Identifier: AGPL-3.0-only
// The appeals screen below its heading (FR-707): the appeals that await a decision and those decided, newest
// first, in the admin mode's list form: a toolbar with the state filter (kept in the URL by the route,
// web-client/README.md S3), a table of one page, and previous / next. Each row's first cell links to the appeal's page,
// where it is decided.
import { EmptyState, Link, ListToolbar, Select, SelectItem, Stack, Text } from "@my/ui";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { type Appeal, type AppealFilter, appealFilters } from "../api/appeals";
import { appealStateLabels, appealTitle, useAppeals } from "../hooks/useAppeals";
import { accountActionStateLabels } from "../hooks/useReports";
import { DateTime, type RecordColumns, RecordTable, RequirePermission } from "./parts";
import { AccountByIdLabel } from "./reportParts";

const filterLabels: Readonly<Record<AppealFilter, ClientMessageKey>> = {
  pending: "client:admin.appeals.filter.pending",
  decided: "client:admin.appeals.filter.decided",
};

function isFilter(key: unknown): key is AppealFilter {
  return appealFilters.some((filter) => filter === key);
}

/** The address of an appeal's page. */
function appealPath(id: string): string {
  return `/moderation/appeals/${encodeURIComponent(id)}`;
}

export type AppealQueueProps = {
  /** The selected state, from the URL. */
  filter: AppealFilter;
  /** Selects another state: the route puts it into the URL. */
  onFilterChange: (filter: AppealFilter) => void;
};

export function AppealQueue(props: AppealQueueProps) {
  return (
    <RequirePermission permission="manage_accounts">
      <AppealsView {...props} />
    </RequirePermission>
  );
}

function AppealsView({ filter, onFilterChange }: AppealQueueProps) {
  const t = useT();
  return (
    <Stack gap="4">
      <ListToolbar label={t("client:admin.list.filters")}>
        <Select
          variant="toolbar"
          aria-label={t("client:admin.appeals.filter.label")}
          value={filter}
          onChange={(key) => {
            if (isFilter(key)) onFilterChange(key);
          }}
        >
          {appealFilters.map((item) => (
            <SelectItem key={item} id={item}>
              {t(filterLabels[item])}
            </SelectItem>
          ))}
        </Select>
      </ListToolbar>
      {/* Another state is another list: it starts at its first page. */}
      <AppealList key={filter} filter={filter} />
    </Stack>
  );
}

function AppealList({ filter }: { filter: AppealFilter }) {
  const t = useT();
  const list = useAppeals(filter);
  const decided = filter === "decided";
  const empty = decided ? (
    <EmptyState headingLevel={2} heading={t("client:admin.appeals.empty.decided")} />
  ) : (
    <EmptyState
      headingLevel={2}
      heading={t("client:admin.appeals.empty.pending")}
      description={t("client:admin.appeals.empty.pending.description")}
    />
  );
  const columns: RecordColumns<Appeal> = [
    {
      id: "appeal",
      header: t("client:admin.appeals.column.appeal"),
      wrap: "anywhere",
      cell: (appeal) => <AppealName appeal={appeal} />,
    },
    {
      id: "account",
      header: t("client:admin.appeal.account"),
      wrap: "anywhere",
      cell: (appeal) => <AccountByIdLabel id={appeal.account_id} />,
    },
    {
      id: "action_state",
      header: t("client:admin.appeal.action_state"),
      cell: (appeal) => t(accountActionStateLabels[appeal.action.state]),
    },
    {
      id: "decision",
      header: t(decided ? "client:admin.appeal.decision" : "client:admin.appeal.submitted_at"),
      cell: (appeal) =>
        appeal.state === "pending" ? (
          <DateTime value={appeal.submitted_at} />
        ) : (
          <Text weight="semibold">{t(appealStateLabels[appeal.state])}</Text>
        ),
    },
    ...(decided
      ? [
          {
            id: "decided_at",
            header: t("client:admin.appeal.decided_at"),
            cell: (appeal: Appeal) => appeal.decided_at !== undefined && <DateTime value={appeal.decided_at} />,
          },
        ]
      : []),
  ];
  return (
    <RecordTable
      list={list}
      caption={t(filterLabels[filter])}
      columns={columns}
      getId={(appeal) => appeal.id}
      headingLevel={2}
      empty={empty}
    />
  );
}

/** The name of an appeal: a link to its page and the start of its text. */
function AppealName({ appeal }: { appeal: Appeal }) {
  const t = useT();
  return (
    <Stack as="span" gap="1">
      <Link href={appealPath(appeal.id)} variant="standalone" wrap="anywhere">
        {appealTitle(t, appeal)}
      </Link>
      {/* Written by the user who appealed, in any language; the appeal's page shows it whole (Y10). */}
      <Text size="sm" dir="auto" lineClamp={2} align="start" wrap="anywhere" whitespace="preWrap">
        {appeal.text}
      </Text>
    </Stack>
  );
}
