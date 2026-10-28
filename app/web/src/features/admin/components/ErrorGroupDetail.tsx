// SPDX-License-Identifier: AGPL-3.0-only
// One error group's page below its heading (ADR-0103 §8, manage_errors): its state with resolving, muting and
// unmuting it; what the group is (where, the type, the message, when and on which versions it was seen, and how
// often); its latest occurrence, with the request ID and the trace ID that find it in the logs and the traces, each
// with a copy button, and its stack; its occurrences by the hour, newest first, a day to a page; and the changes of its
// state, newest first. The actions change which buttons there are, so focus then moves to the state's heading, and
// what happened stays in the section's StatusMessage. Types, places, messages and stacks are the server's own text,
// in English, read left to right.
import {
  Button,
  Cluster,
  CodeBlock,
  DescriptionItem,
  DescriptionList,
  EmptyState,
  Hint,
  Stack,
  StatusMessage,
  TableCell,
  TableColumnHeader,
  TableRow,
  TableRowHeader,
  Text,
} from "@my/ui";
import { useId, useRef } from "react";
import { useIntl } from "react-intl";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import {
  DateTime,
  DetailSection,
  type PagedPages,
  RecordTable,
  RequirePermission,
  useFocusAfterChange,
} from "../../moderation";
import type { ErrorGroupDetail as Detail, ErrorGroupEvent, ErrorGroupHourCount, ErrorGroupSample } from "../api/errors";
import { type ErrorGroupActions, useCopyErrorValue, useErrorGroup, useErrorGroupActions } from "../hooks/useErrors";
import { errorKindLabels, errorSourceLabels, ErrorStateLabel, isolatedVersion, SeenAt } from "./ErrorGroups";

const eventLabels: Readonly<Record<ErrorGroupEvent["kind"], ClientMessageKey>> = {
  resolved: "client:admin.errors.event.resolved",
  regressed: "client:admin.errors.event.regressed",
  muted: "client:admin.errors.event.muted",
  unmuted: "client:admin.errors.event.unmuted",
};

const actorLabels: Readonly<Record<ErrorGroupEvent["actor"], ClientMessageKey>> = {
  user: "client:admin.errors.actor.user",
  cli: "client:admin.errors.actor.cli",
  system: "client:admin.errors.actor.system",
};

/** The hours of a page of the hourly counts: a day. */
const hoursPerPage = 24;

export function ErrorGroupDetail({ id }: { id: string }) {
  return (
    <RequirePermission permission="manage_errors">
      <ErrorGroupView id={id} />
    </RequirePermission>
  );
}

function ErrorGroupView({ id }: { id: string }) {
  const t = useT();
  const group = useErrorGroup(id);
  return (
    <Stack gap="8">
      <StateSection group={group} />
      <DetailSection title={t("client:admin.errors.details")}>
        <Facts group={group} />
      </DetailSection>
      <DetailSection title={t("client:admin.errors.sample")}>
        {group.sample === undefined ? (
          <Hint size="inherit">{t("client:admin.errors.sample.none")}</Hint>
        ) : (
          <Sample sample={group.sample} />
        )}
      </DetailSection>
      <DetailSection title={t("client:admin.errors.hourly")}>
        <HourlyCounts counts={group.hourly_counts} />
      </DetailSection>
      <DetailSection title={t("client:admin.errors.history")}>
        <History events={group.events} />
      </DetailSection>
    </Stack>
  );
}

function StateSection({ group }: { group: Detail }) {
  const t = useT();
  const headingRef = useRef<HTMLHeadingElement>(null);
  const focusHeading = useFocusAfterChange(headingRef, group.state);
  const actions = useErrorGroupActions(group.id, focusHeading);
  return (
    <DetailSection title={t("client:admin.errors.state")} headingRef={headingRef}>
      <Stack gap="4">
        <StatusMessage>{actions.status}</StatusMessage>
        <DescriptionList columns={3}>
          <DescriptionItem term={t("client:admin.errors.state")}>
            <ErrorStateLabel state={group.state} />
          </DescriptionItem>
          <DescriptionItem term={t("client:admin.errors.state_since")}>
            <DateTime value={group.state_since} />
          </DescriptionItem>
          {group.resolved_version !== undefined && (
            <DescriptionItem term={t("client:admin.errors.resolved_version")}>
              <Text wrap="anywhere" dir="ltr">
                {group.resolved_version}
              </Text>
            </DescriptionItem>
          )}
        </DescriptionList>
        <Actions group={group} actions={actions} />
      </Stack>
    </DetailSection>
  );
}

/** The buttons of the group's state: resolving an open group, muting one that is not muted, and unmuting a muted one. */
function Actions({ group, actions }: { group: Detail; actions: ErrorGroupActions }) {
  const t = useT();
  const resolveId = useId();
  const muteId = useId();
  const unmuteId = useId();
  const busy = actions.pending !== undefined;
  return (
    <Stack gap="3">
      {group.state === "open" && (
        <Stack align="start" gap="1">
          <Button
            variant="primary"
            isPending={actions.pending === "resolve"}
            isDisabled={busy}
            aria-describedby={resolveId}
            onPress={() => {
              actions.run("resolve");
            }}
          >
            {t("client:admin.errors.resolve")}
          </Button>
          <Hint id={resolveId}>{t("client:admin.errors.resolve.description")}</Hint>
        </Stack>
      )}
      {group.state !== "muted" && (
        <Stack align="start" gap="1">
          <Button
            isPending={actions.pending === "mute"}
            isDisabled={busy}
            aria-describedby={muteId}
            onPress={() => {
              actions.run("mute");
            }}
          >
            {t("client:admin.errors.mute")}
          </Button>
          <Hint id={muteId}>{t("client:admin.errors.mute.description")}</Hint>
        </Stack>
      )}
      {group.state === "muted" && (
        <Stack align="start" gap="1">
          <Button
            isPending={actions.pending === "unmute"}
            isDisabled={busy}
            aria-describedby={unmuteId}
            onPress={() => {
              actions.run("unmute");
            }}
          >
            {t("client:admin.errors.unmute")}
          </Button>
          <Hint id={unmuteId}>{t("client:admin.errors.unmute.description")}</Hint>
        </Stack>
      )}
    </Stack>
  );
}

function Facts({ group }: { group: Detail }) {
  const t = useT();
  return (
    <Stack gap="4">
      <DescriptionList columns={3}>
        <DescriptionItem term={t("client:admin.errors.source")}>{t(errorSourceLabels[group.source])}</DescriptionItem>
        <DescriptionItem term={t("client:admin.errors.kind")}>{t(errorKindLabels[group.kind])}</DescriptionItem>
        <DescriptionItem term={t("client:admin.errors.count_24h")}>
          {t("client:admin.errors.count", { count: group.count_24h })}
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.errors.count_7d")}>
          {t("client:admin.errors.count", { count: group.count_7d })}
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.errors.first_seen")}>
          <SeenAt at={group.first_seen_at} version={group.first_version} />
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.errors.last_seen")}>
          <SeenAt at={group.last_seen_at} version={group.last_version} />
        </DescriptionItem>
      </DescriptionList>
      <DescriptionList>
        <DescriptionItem term={t("client:admin.errors.where")}>
          <Text mono wrap="anywhere" dir="ltr">
            {group.where}
          </Text>
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.errors.type")}>
          <Text mono wrap="anywhere" dir="ltr">
            {group.type}
          </Text>
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.errors.message")}>
          <Text wrap="anywhere" whitespace="preWrap" dir="ltr">
            {group.message}
          </Text>
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.errors.fingerprint")}>
          <Text mono wrap="anywhere" dir="ltr">
            {group.fingerprint}
          </Text>
        </DescriptionItem>
      </DescriptionList>
    </Stack>
  );
}

/** The latest occurrence: when, on which version and role, the IDs that find it, its types, its message and its stack. */
function Sample({ sample }: { sample: ErrorGroupSample }) {
  const t = useT();
  return (
    <Stack gap="4">
      <Hint size="inherit">{t("client:admin.errors.sample.description")}</Hint>
      <DescriptionList columns={3}>
        <DescriptionItem term={t("client:admin.errors.sample.occurred_at")}>
          <DateTime value={sample.occurred_at} />
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.errors.sample.version")}>
          <Text wrap="anywhere" dir="ltr">
            {sample.version}
          </Text>
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.errors.sample.role")}>
          <Text mono dir="ltr">
            {sample.role}
          </Text>
        </DescriptionItem>
      </DescriptionList>
      <DescriptionList>
        <CopyableItem
          term={t("client:admin.errors.sample.request_id")}
          copyLabel={t("client:admin.errors.copy_request_id")}
          value={sample.request_id}
        />
        <CopyableItem
          term={t("client:admin.errors.sample.trace_id")}
          copyLabel={t("client:admin.errors.copy_trace_id")}
          value={sample.trace_id}
        />
        <DescriptionItem term={t("client:admin.errors.sample.type")}>
          <Text mono wrap="anywhere" dir="ltr">
            {sample.type}
          </Text>
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.errors.message")}>
          <Text wrap="anywhere" whitespace="preWrap" dir="ltr">
            {sample.message}
          </Text>
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.errors.sample.stack")}>
          <CodeBlock dir="ltr">{sample.stack}</CodeBlock>
        </DescriptionItem>
      </DescriptionList>
    </Stack>
  );
}

/** An ID of the occurrence with a button that copies it, or that the occurrence had none. */
function CopyableItem({ term, copyLabel, value }: { term: string; copyLabel: string; value: string | undefined }) {
  const t = useT();
  const copy = useCopyErrorValue();
  return (
    <DescriptionItem term={term}>
      {value === undefined ? (
        t("client:admin.errors.sample.none_id")
      ) : (
        <Cluster gap="2">
          <Text mono wrap="anywhere" selectAll dir="ltr">
            {value}
          </Text>
          <Button
            size="sm"
            aria-label={copyLabel}
            onPress={() => {
              copy(value);
            }}
          >
            {t("client:admin.errors.copy")}
          </Button>
        </Cluster>
      )}
    </DescriptionItem>
  );
}

/** The hourly counts, newest first, a day to a page; the API answers them oldest first, and all at once. */
function hourPages(counts: readonly ErrorGroupHourCount[]): PagedPages<ErrorGroupHourCount> {
  const items = [...counts].reverse();
  const pages: ErrorGroupHourCount[][] = [];
  for (let start = 0; start < items.length; start += hoursPerPage) pages.push(items.slice(start, start + hoursPerPage));
  return { items, pages, hasMore: false, isLoadingMore: false, loadMore: () => undefined };
}

function HourlyCounts({ counts }: { counts: readonly ErrorGroupHourCount[] }) {
  const t = useT();
  const intl = useIntl();
  return (
    <Stack gap="3">
      <Hint size="inherit">{t("client:admin.errors.hourly.description")}</Hint>
      <RecordTable
        list={hourPages(counts)}
        caption={t("client:admin.errors.hourly")}
        getId={(count) => count.hour}
        empty={<EmptyState headingLevel={3} heading={t("client:admin.errors.hourly.empty")} />}
        head={
          <>
            <TableColumnHeader>{t("client:admin.errors.hourly.hour")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.errors.hourly.count")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.errors.hourly.version")}</TableColumnHeader>
          </>
        }
      >
        {(count) => (
          <TableRow data-row-id={count.hour}>
            <TableRowHeader>
              <DateTime value={count.hour} />
            </TableRowHeader>
            <TableCell numeric>{intl.formatNumber(count.count)}</TableCell>
            <TableCell wrap="anywhere">
              <span dir="ltr">{count.last_version}</span>
            </TableCell>
          </TableRow>
        )}
      </RecordTable>
    </Stack>
  );
}

function History({ events }: { events: readonly ErrorGroupEvent[] }) {
  const t = useT();
  const items = [...events].reverse();
  return (
    <RecordTable
      list={{
        items,
        pages: items.length > 0 ? [items] : [],
        hasMore: false,
        isLoadingMore: false,
        loadMore: () => undefined,
      }}
      caption={t("client:admin.errors.history")}
      getId={(event) => `${event.occurred_at} ${event.kind}`}
      empty={<EmptyState headingLevel={3} heading={t("client:admin.errors.history.empty")} />}
      head={
        <>
          <TableColumnHeader>{t("client:admin.errors.history.change")}</TableColumnHeader>
          <TableColumnHeader>{t("client:admin.errors.history.by")}</TableColumnHeader>
          <TableColumnHeader>{t("client:admin.errors.history.at")}</TableColumnHeader>
        </>
      }
    >
      {(event) => (
        <TableRow data-row-id={`${event.occurred_at} ${event.kind}`}>
          <TableRowHeader wrap="anywhere">
            {event.version === undefined
              ? t(eventLabels[event.kind])
              : t("client:admin.errors.event.with_version", {
                  change: t(eventLabels[event.kind]),
                  version: isolatedVersion(event.version),
                })}
          </TableRowHeader>
          <TableCell>{t(actorLabels[event.actor])}</TableCell>
          <TableCell>
            <DateTime value={event.occurred_at} />
          </TableCell>
        </TableRow>
      )}
    </RecordTable>
  );
}
