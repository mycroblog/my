// SPDX-License-Identifier: AGPL-3.0-only
// The error groups below their heading (ADR-0103 §8), in the admin mode's list form: a toolbar with the state and
// source filters (kept in the URL by the route, web-client/README.md S3), a table of one page, and previous / next.
// The groups are the most recently seen first; each row's first cell links to the group's page, where it is resolved
// or muted. Types, places and messages are the server's own text, in English, read left to right.
import { EmptyState, Hint, Link, ListToolbar, Select, SelectItem, Stack, TableCell, TableColumnHeader, TableRow, TableRowHeader, Text } from "@my/ui";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { adminErrorGroupPath } from "../../../lib/paths";
import { DateTime, RecordTable, RequirePermission } from "../../moderation";
import {
  type ErrorFilter,
  type ErrorGroup,
  type ErrorGroupKind,
  type ErrorGroupSource,
  type ErrorGroupState,
  errorSourceFilters,
  type ErrorSourceFilter,
  errorStateFilters,
  type ErrorStateFilter,
} from "../api/errors";
import { useErrorGroups } from "../hooks/useErrors";

const errorStateLabels: Readonly<Record<ErrorGroupState, ClientMessageKey>> = {
  open: "client:admin.errors.state.open",
  resolved: "client:admin.errors.state.resolved",
  muted: "client:admin.errors.state.muted",
};

export const errorSourceLabels: Readonly<Record<ErrorGroupSource, ClientMessageKey>> = {
  server: "client:admin.errors.source.server",
  web: "client:admin.errors.source.web",
};

export const errorKindLabels: Readonly<Record<ErrorGroupKind, ClientMessageKey>> = {
  panic: "client:admin.errors.kind.panic",
  error: "client:admin.errors.kind.error",
};

const stateFilterLabels: Readonly<Record<ErrorStateFilter, ClientMessageKey>> = {
  open: "client:admin.errors.state.open",
  resolved: "client:admin.errors.state.resolved",
  muted: "client:admin.errors.state.muted",
  all: "client:admin.errors.filter.all_states",
};

const sourceFilterLabels: Readonly<Record<ErrorSourceFilter, ClientMessageKey>> = {
  all: "client:admin.errors.filter.all_sources",
  server: "client:admin.errors.source.server",
  web: "client:admin.errors.source.web",
};

function isStateFilter(key: unknown): key is ErrorStateFilter {
  return errorStateFilters.some((filter) => filter === key);
}

function isSourceFilter(key: unknown): key is ErrorSourceFilter {
  return errorSourceFilters.some((filter) => filter === key);
}

/** A group's state in words, not by color alone (WCAG 1.4.1): the color only repeats it. */
export function ErrorStateLabel({ state }: { state: ErrorGroupState }) {
  const t = useT();
  return (
    <Text weight="semibold" {...(state === "open" && { tone: "danger" as const })} {...(state === "resolved" && { tone: "success" as const })}>
      {t(errorStateLabels[state])}
    </Text>
  );
}

export type ErrorGroupsProps = {
  /** The selected filters, from the URL. */
  filter: ErrorFilter;
  /** Selects other filters: the route puts them into the URL. */
  onFilterChange: (filter: ErrorFilter) => void;
};

export function ErrorGroups(props: ErrorGroupsProps) {
  return (
    <RequirePermission permission="manage_errors">
      <ErrorGroupsView {...props} />
    </RequirePermission>
  );
}

function ErrorGroupsView({ filter, onFilterChange }: ErrorGroupsProps) {
  const t = useT();
  return (
    <Stack gap="4">
      <Hint>{t("client:admin.errors.description")}</Hint>
      <ListToolbar label={t("client:admin.list.filters")}>
        <Select
          variant="toolbar"
          aria-label={t("client:admin.errors.filter.state")}
          value={filter.state}
          onChange={(key) => {
            if (isStateFilter(key)) onFilterChange({ ...filter, state: key });
          }}
        >
          {errorStateFilters.map((item) => (
            <SelectItem key={item} id={item}>
              {t(stateFilterLabels[item])}
            </SelectItem>
          ))}
        </Select>
        <Select
          variant="toolbar"
          aria-label={t("client:admin.errors.filter.source")}
          value={filter.source}
          onChange={(key) => {
            if (isSourceFilter(key)) onFilterChange({ ...filter, source: key });
          }}
        >
          {errorSourceFilters.map((item) => (
            <SelectItem key={item} id={item}>
              {t(sourceFilterLabels[item])}
            </SelectItem>
          ))}
        </Select>
      </ListToolbar>
      {/* Other filters are another list: it starts at its first page. */}
      <ErrorGroupList key={`${filter.state} ${filter.source}`} filter={filter} />
    </Stack>
  );
}

function ErrorGroupList({ filter }: { filter: ErrorFilter }) {
  const t = useT();
  const list = useErrorGroups(filter);
  const filtered = filter.state !== "open" || filter.source !== "all";
  return (
    <RecordTable
      list={list}
      caption={t("client:admin.errors.list")}
      getId={(group) => group.id}
      empty={
        <EmptyState
          headingLevel={2}
          heading={t(filtered ? "client:admin.errors.empty.filtered" : "client:admin.errors.empty")}
          description={t("client:admin.errors.empty.description")}
        />
      }
      head={
        <>
          <TableColumnHeader>{t("client:admin.errors.column.error")}</TableColumnHeader>
          <TableColumnHeader>{t("client:admin.errors.state")}</TableColumnHeader>
          <TableColumnHeader>{t("client:admin.errors.source")}</TableColumnHeader>
          <TableColumnHeader>{t("client:admin.errors.column.counts")}</TableColumnHeader>
          <TableColumnHeader>{t("client:admin.errors.last_seen")}</TableColumnHeader>
          <TableColumnHeader>{t("client:admin.errors.first_seen")}</TableColumnHeader>
        </>
      }
    >
      {(group) => <ErrorGroupRow group={group} />}
    </RecordTable>
  );
}

function ErrorGroupRow({ group }: { group: ErrorGroup }) {
  const t = useT();
  return (
    <TableRow data-row-id={group.id}>
      <TableRowHeader wrap="anywhere">
        <Stack gap="1">
          <Link href={adminErrorGroupPath(group.id)} variant="standalone" wrap="anywhere">
            <span dir="ltr">{group.type}</span>
          </Link>
          <Text size="sm" tone="muted" mono wrap="anywhere" dir="ltr">
            {group.where}
          </Text>
          <Text as="p" size="sm" lineClamp={2} wrap="anywhere" dir="ltr">
            {group.message}
          </Text>
        </Stack>
      </TableRowHeader>
      <TableCell>
        <ErrorStateLabel state={group.state} />
      </TableCell>
      <TableCell>{t(errorSourceLabels[group.source])}</TableCell>
      <TableCell>
        <Stack gap="1">
          <span>{t("client:admin.errors.count_24h.value", { count: group.count_24h })}</span>
          <span>{t("client:admin.errors.count_7d.value", { count: group.count_7d })}</span>
        </Stack>
      </TableCell>
      <TableCell>
        <SeenAt at={group.last_seen_at} version={group.last_version} />
      </TableCell>
      <TableCell>
        <SeenAt at={group.first_seen_at} version={group.first_version} />
      </TableCell>
    </TableRow>
  );
}

/** A version of the server in a sentence: an identifier, read left to right in every locale (web-client/README.md Y10). */
export function isolatedVersion(version: string): string {
  return `\u2066${version}\u2069`;
}

/** When the group was seen, and on which version of the server. */
export function SeenAt({ at, version }: { at: string; version: string }) {
  const t = useT();
  return (
    <Stack gap="1">
      <DateTime value={at} />
      <Text size="sm" tone="muted" wrap="anywhere">
        {t("client:admin.errors.version", { version: isolatedVersion(version) })}
      </Text>
    </Stack>
  );
}
