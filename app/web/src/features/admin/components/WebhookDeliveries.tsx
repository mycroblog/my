// SPDX-License-Identifier: AGPL-3.0-only
// The deliveries of one webhook on its page (ADR-0060, manage_webhooks): those of the last 14 days, newest first, each
// with its event and message ID, whether it succeeded with how many attempts and its last one, when it was made, and a
// button that sends it again with the same webhook-id. What a redelivery did stays above the list in a StatusMessage,
// and a failure in an alert.
import { Button, Cluster, EmptyState, Hint, InlineError, Stack, StatusMessage, Text } from "@my/ui";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { DateTime, type RecordColumns, RecordTable } from "../../moderation";
import type { WebhookAttemptOutcome, WebhookDelivery, WebhookDeliveryStatus } from "../api/webhooks";
import { useRedeliver, useWebhookDeliveries } from "../hooks/useWebhooks";
import { webhookEventLabels } from "./webhookParts";

const statusLabels: Readonly<Record<WebhookDeliveryStatus, ClientMessageKey>> = {
  pending: "client:admin.webhooks.deliveries.status.pending",
  succeeded: "client:admin.webhooks.deliveries.status.succeeded",
  failing: "client:admin.webhooks.deliveries.status.failing",
};

const outcomeLabels: Readonly<Record<WebhookAttemptOutcome, ClientMessageKey>> = {
  succeeded: "client:admin.webhooks.deliveries.outcome.succeeded",
  http_error: "client:admin.webhooks.deliveries.outcome.http_error",
  timeout: "client:admin.webhooks.deliveries.outcome.timeout",
  connection_error: "client:admin.webhooks.deliveries.outcome.connection_error",
  forbidden_destination: "client:admin.webhooks.deliveries.outcome.forbidden_destination",
};

/** The deliveries of a webhook, for the section of its page. */
export function WebhookDeliveryList({ id }: { id: string }) {
  const t = useT();
  const list = useWebhookDeliveries(id);
  const redeliver = useRedeliver(id);
  const columns: RecordColumns<WebhookDelivery> = [
    {
      id: "event",
      header: t("client:admin.webhooks.events"),
      wrap: "anywhere",
      cell: (delivery) => (
        <Stack gap="1">
          <Text weight="semibold">{t(webhookEventLabels[delivery.event_type])}</Text>
          <Text size="sm" tone="muted" mono wrap="anywhere" dir="ltr">
            {delivery.message_id}
          </Text>
        </Stack>
      ),
    },
    {
      id: "status",
      header: t("client:admin.webhooks.deliveries.status"),
      cell: (delivery) => {
        const last = delivery.last_attempt;
        return (
          <Stack gap="1">
            <Text {...(delivery.status === "failing" && { tone: "warning" as const })}>
              {t(statusLabels[delivery.status])}
            </Text>
            <Text size="sm" tone="muted">
              {t("client:admin.webhooks.deliveries.attempts.count", { count: delivery.attempts })}
            </Text>
            {last !== undefined && (
              <Cluster as="span" gap="1" rowGap="0">
                <Text size="sm" tone="muted">
                  {t(outcomeLabels[last.outcome])}
                </Text>
                {last.status_code !== undefined && (
                  <Text size="sm" tone="muted" dir="ltr">
                    {t("client:admin.webhooks.deliveries.status_code", { code: String(last.status_code) })}
                  </Text>
                )}
                <Text size="sm" tone="muted">
                  <DateTime value={last.attempted_at} />
                </Text>
              </Cluster>
            )}
          </Stack>
        );
      },
    },
    {
      id: "created_at",
      header: t("client:admin.webhooks.deliveries.created_at"),
      cell: (delivery) => <DateTime value={delivery.created_at} />,
    },
    {
      id: "actions",
      kind: "actions",
      header: t("client:admin.list.column.actions"),
      cell: (delivery) => (
        <Button
          size="sm"
          isPending={redeliver.pendingId === delivery.id}
          aria-label={t("client:admin.webhooks.deliveries.redeliver_of", {
            action: t("client:admin.webhooks.deliveries.redeliver"),
            event: t(webhookEventLabels[delivery.event_type]),
            id: delivery.message_id,
          })}
          onPress={() => {
            redeliver.redeliver(delivery);
          }}
        >
          {t("client:admin.webhooks.deliveries.redeliver")}
        </Button>
      ),
    },
  ];
  return (
    <Stack gap="3">
      <Hint>{t("client:admin.webhooks.deliveries.description")}</Hint>
      <StatusMessage>{redeliver.status}</StatusMessage>
      {redeliver.error !== undefined && (
        <InlineError size="inherit" alert>
          {t("client:admin.webhooks.deliveries.redeliver_failed", { reason: redeliver.error })}
        </InlineError>
      )}
      <RecordTable
        list={list}
        caption={t("client:admin.webhooks.deliveries.title")}
        columns={columns}
        getId={(delivery) => delivery.id}
        headingLevel={3}
        empty={<EmptyState headingLevel={3} heading={t("client:admin.webhooks.deliveries.empty")} />}
      />
    </Stack>
  );
}
