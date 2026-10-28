// SPDX-License-Identifier: AGPL-3.0-only
// The remote servers screen below its heading (federation.md §7.3), in the admin mode's table form: a toolbar
// with the state filter, kept in the URL by the route (web-client/README.md S3), and one page of the servers of the
// selected filter at a time, previous / next, each with its state, since when, and its failures, linking to
// the server's page with the reasons.
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
} from "@my/ui";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { type InstanceFilter, instanceFilters, type InstanceHealth } from "../api/staff";
import { useInstances } from "../hooks/useInstances";
import { Day, DateTime, DomainName, InstanceStateLabel, RecordTable, RequirePermission } from "./parts";

const filterLabels: Readonly<Record<InstanceFilter, ClientMessageKey>> = {
  all: "client:admin.instances.filter.all",
  unavailable: "client:admin.instances.filter.unavailable",
  available: "client:admin.instances.filter.available",
};

function isFilter(key: unknown): key is InstanceFilter {
  return instanceFilters.some((filter) => filter === key);
}

/** The address of a server's page. */
function instancePath(domain: string): string {
  return `/moderation/instances/${encodeURIComponent(domain)}`;
}

export type InstanceListProps = {
  /** The selected filter, from the URL. */
  filter: InstanceFilter;
  /** Selects another filter: the route puts it into the URL. */
  onFilterChange: (filter: InstanceFilter) => void;
};

export function InstanceList(props: InstanceListProps) {
  return (
    <RequirePermission permission="manage_domains">
      <InstanceTabs {...props} />
    </RequirePermission>
  );
}

function InstanceTabs({ filter, onFilterChange }: InstanceListProps) {
  const t = useT();
  return (
    <Stack gap="4">
      <ListToolbar label={t("client:admin.list.filters")}>
        <Select
          variant="toolbar"
          aria-label={t("client:admin.instances.filter.label")}
          value={filter}
          onChange={(key) => {
            if (isFilter(key)) onFilterChange(key);
          }}
        >
          {instanceFilters.map((item) => (
            <SelectItem key={item} id={item}>
              {t(filterLabels[item])}
            </SelectItem>
          ))}
        </Select>
      </ListToolbar>
      {/* Another filter is another list: it starts at its first page. */}
      <Instances key={filter} filter={filter} />
    </Stack>
  );
}

function Instances({ filter }: { filter: InstanceFilter }) {
  const t = useT();
  const list = useInstances(filter);
  return (
    <RecordTable
      list={list}
      caption={t(filterLabels[filter])}
      getId={(health) => health.domain}
      empty={
        <EmptyState
          headingLevel={2}
          heading={t("client:admin.instances.empty")}
          description={t("client:admin.instances.empty.description")}
        />
      }
      head={
        <>
          <TableColumnHeader>{t("client:admin.instances.column.server")}</TableColumnHeader>
          <TableColumnHeader>{t("client:admin.instance.state")}</TableColumnHeader>
          <TableColumnHeader>{t("client:admin.instance.since")}</TableColumnHeader>
          <TableColumnHeader>{t("client:admin.instance.failure_days")}</TableColumnHeader>
          <TableColumnHeader>{t("client:admin.instance.last_failure")}</TableColumnHeader>
        </>
      }
    >
      {(health) => <InstanceRow health={health} />}
    </RecordTable>
  );
}

function InstanceRow({ health }: { health: InstanceHealth }) {
  const t = useT();
  return (
    <TableRow data-row-id={health.domain}>
      <TableRowHeader wrap="anywhere">
        <Link href={instancePath(health.domain)} variant="standalone" wrap="anywhere">
          <DomainName domain={health.domain} />
        </Link>
      </TableRowHeader>
      <TableCell>
        <InstanceStateLabel state={health.state} />
      </TableCell>
      <TableCell>
        {health.since === undefined ? t("client:admin.instance.none") : <DateTime value={health.since} />}
      </TableCell>
      <TableCell numeric>{t("client:admin.instance.failure_days.value", { count: health.failure_days })}</TableCell>
      <TableCell>
        {health.last_failure_day === undefined ? (
          t("client:admin.instance.none")
        ) : (
          <Day value={health.last_failure_day} />
        )}
      </TableCell>
    </TableRow>
  );
}
