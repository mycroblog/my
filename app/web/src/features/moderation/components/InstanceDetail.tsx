// SPDX-License-Identifier: AGPL-3.0-only
// A remote server's page below its heading (federation.md §7.3): its health, why it is in its state, when
// deliveries last resumed, the button that resumes them, the way to its moderation actions (FR-702), the refetch
// of its accounts' posts (FR-407), and the history of its availability, newest first.
import {
  Button,
  Card,
  DescriptionItem,
  DescriptionList,
  EmptyState,
  Heading,
  Hint,
  Link,
  Stack,
  TableCell,
  TableColumnHeader,
  TableRow,
  TableRowHeader,
} from "@my/ui";
import { useId } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import type { AvailabilityEvent } from "../api/staff";
import { useInstance } from "../hooks/useInstances";
import { Day, DateTime, Section, InstanceStateLabel, RecordTable, RequirePermission } from "./parts";
import { RefetchStatusesSection } from "./RefetchStatuses";

const eventLabels: Readonly<Record<AvailabilityEvent["kind"], ClientMessageKey>> = {
  marked_unavailable: "client:admin.instance.event.marked_unavailable",
  marked_available: "client:admin.instance.event.marked_available",
};

const reasonLabels: Readonly<Record<AvailabilityEvent["reason"], ClientMessageKey>> = {
  failure_days: "client:admin.instance.reason.failure_days",
  delivery_succeeded: "client:admin.instance.reason.delivery_succeeded",
  inbound_request: "client:admin.instance.reason.inbound_request",
  probe_succeeded: "client:admin.instance.reason.probe_succeeded",
  operator: "client:admin.instance.reason.operator",
};

export type InstanceDetailProps = { domain: string };

export function InstanceDetail({ domain }: InstanceDetailProps) {
  return (
    <RequirePermission permission="manage_domains">
      <InstanceView domain={domain} />
    </RequirePermission>
  );
}

function InstanceView({ domain }: InstanceDetailProps) {
  const t = useT();
  const descriptionId = useId();
  const moderationId = useId();
  const view = useInstance(domain);
  const { health, events } = view;
  // The newest event says why the server is in its state; the newest marked_available one, when deliveries
  // last resumed. Both are among the loaded events only if the server has any.
  const latest = events.items[0];
  const resumed = events.items.find((event) => event.kind === "marked_available");
  return (
    <Stack gap="6">
      <Card>
        <Stack gap="4">
          <DescriptionList columns={3}>
            <DescriptionItem term={t("client:admin.instance.state")}>
              <InstanceStateLabel state={health.state} />
            </DescriptionItem>
            <DescriptionItem term={t("client:admin.instance.since")}>
              {health.since === undefined ? t("client:admin.instance.none") : <DateTime value={health.since} />}
            </DescriptionItem>
            <DescriptionItem term={t("client:admin.instance.reason")}>
              {latest === undefined ? t("client:admin.instance.none") : t(reasonLabels[latest.reason])}
            </DescriptionItem>
            <DescriptionItem term={t("client:admin.instance.failure_days")}>
              {t("client:admin.instance.failure_days.value", { count: health.failure_days })}
            </DescriptionItem>
            <DescriptionItem term={t("client:admin.instance.last_failure")}>
              {health.last_failure_day === undefined ? (
                t("client:admin.instance.none")
              ) : (
                <Day value={health.last_failure_day} />
              )}
            </DescriptionItem>
            <DescriptionItem term={t("client:admin.instance.last_resumed")}>
              {resumed === undefined ? t("client:admin.instance.none") : <DateTime value={resumed.occurred_at} />}
            </DescriptionItem>
          </DescriptionList>
          {view.canMarkAvailable && (
            <Stack align="start" gap="1">
              <Button
                variant="primary"
                isPending={view.isMarking}
                onPress={view.markAvailable}
                aria-describedby={descriptionId}
              >
                {t("client:admin.instance.mark_available")}
              </Button>
              <Hint id={descriptionId}>{t("client:admin.instance.mark_available.description")}</Hint>
            </Stack>
          )}
        </Stack>
      </Card>
      <Section surface="card" title={t("client:admin.instance.moderation.title")}>
        <Stack align="start" gap="1">
          <Link
            href={`/moderation/domain-actions?domain=${encodeURIComponent(domain)}`}
            variant="standalone"
            aria-describedby={moderationId}
          >
            {t("client:admin.instance.moderation.link")}
          </Link>
          <Hint id={moderationId}>{t("client:admin.instance.moderation.description")}</Hint>
        </Stack>
      </Section>
      <RefetchStatusesSection target={{ kind: "domain", domain }} />
      <Stack gap="3">
        <Heading level={2}>{t("client:admin.instance.history.title")}</Heading>
        <RecordTable
          list={events}
          caption={t("client:admin.instance.history.title")}
          getId={(event) => event.id}
          empty={<EmptyState headingLevel={3} heading={t("client:admin.instance.history.empty")} />}
          head={
            <>
              <TableColumnHeader>{t("client:admin.instance.column.event")}</TableColumnHeader>
              <TableColumnHeader>{t("client:admin.instance.column.occurred_at")}</TableColumnHeader>
              <TableColumnHeader>{t("client:admin.instance.reason")}</TableColumnHeader>
            </>
          }
        >
          {(event) => (
            <TableRow data-row-id={event.id}>
              <TableRowHeader>{t(eventLabels[event.kind])}</TableRowHeader>
              <TableCell>
                <DateTime value={event.occurred_at} />
              </TableCell>
              <TableCell>{t(reasonLabels[event.reason])}</TableCell>
            </TableRow>
          )}
        </RecordTable>
      </Stack>
    </Stack>
  );
}
