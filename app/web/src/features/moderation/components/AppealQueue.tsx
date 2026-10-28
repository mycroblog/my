// SPDX-License-Identifier: AGPL-3.0-only
// The appeals screen below its heading (FR-707): the appeals that await a decision and those decided, newest
// first, in the admin mode's list form: a toolbar with the state filter (kept in the URL by the route,
// web-client/README.md S3), a table of one page, and previous / next. Each row's first cell links to the appeal's page,
// where it is decided.
import {
  EmptyState,
  Link,
  ListToolbar,
  Select,
  SelectItem,
  Stack,
  TableCell,
  TableColumnHeader,
  TableRow,
  TableRowHeader,
  Text,
} from "@my/ui";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { type Appeal, type AppealFilter, appealFilters } from "../api/appeals";
import { appealStateLabels, appealTitle, useAppeals } from "../hooks/useAppeals";
import { accountActionStateLabels } from "../hooks/useReports";
import { DateTime, RecordTable, RequirePermission } from "./parts";
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
  return (
    <RecordTable
      list={list}
      caption={t(filterLabels[filter])}
      getId={(appeal) => appeal.id}
      empty={empty}
      head={
        <>
          <TableColumnHeader>{t("client:admin.appeals.column.appeal")}</TableColumnHeader>
          <TableColumnHeader>{t("client:admin.appeal.account")}</TableColumnHeader>
          <TableColumnHeader>{t("client:admin.appeal.action_state")}</TableColumnHeader>
          <TableColumnHeader>
            {t(decided ? "client:admin.appeal.decision" : "client:admin.appeal.submitted_at")}
          </TableColumnHeader>
          {decided && <TableColumnHeader>{t("client:admin.appeal.decided_at")}</TableColumnHeader>}
        </>
      }
    >
      {(appeal) => <AppealRow appeal={appeal} />}
    </RecordTable>
  );
}

function AppealRow({ appeal }: { appeal: Appeal }) {
  const t = useT();
  return (
    <TableRow data-row-id={appeal.id}>
      <TableRowHeader wrap="anywhere">
        <Stack gap="1">
          <Link href={appealPath(appeal.id)} variant="standalone" wrap="anywhere">
            {appealTitle(t, appeal)}
          </Link>
          {/* Written by the user who appealed, in any language; the appeal's page shows it whole (Y10). */}
          <Text as="p" size="sm" dir="auto" lineClamp={2} align="start" wrap="anywhere" whitespace="preWrap">
            {appeal.text}
          </Text>
        </Stack>
      </TableRowHeader>
      <TableCell wrap="anywhere">
        <AccountByIdLabel id={appeal.account_id} />
      </TableCell>
      <TableCell>{t(accountActionStateLabels[appeal.action.state])}</TableCell>
      {appeal.state === "pending" ? (
        <TableCell>
          <DateTime value={appeal.submitted_at} />
        </TableCell>
      ) : (
        <>
          <TableCell>
            <Text weight="semibold">{t(appealStateLabels[appeal.state])}</Text>
          </TableCell>
          <TableCell>{appeal.decided_at !== undefined && <DateTime value={appeal.decided_at} />}</TableCell>
        </>
      )}
    </TableRow>
  );
}
