// SPDX-License-Identifier: AGPL-3.0-only
import { Button, OutboxItem, Stack } from "@my/ui";
import { useT } from "../../../lib/i18n";
import type { OutboxView } from "../hooks/useComposeForm";

export type OutboxProps = {
  items: readonly OutboxView[];
  onRetry: (idempotencyKey: string) => void;
  onEdit: (idempotencyKey: string) => void;
  canEdit: boolean;
  onDiscard: (idempotencyKey: string) => void;
};

/**
 * The posts on their way to the server, shown from what was sent until the server confirms them
 * (web-client/README.md S8), and the ones that could not be sent, with what to do about them.
 */
export function Outbox({ items, onRetry, onEdit, canEdit, onDiscard }: OutboxProps) {
  const t = useT();
  if (items.length === 0) return null;
  return (
    <Stack as="ul" aria-label={t("client:compose.outbox.label")} gap="2">
      {items.map((item) => (
        <OutboxItem
          key={item.idempotencyKey}
          text={item.text}
          lang={item.language}
          sendingLabel={item.status === "sending" ? t("client:compose.outbox.sending") : undefined}
          error={
            item.status === "sending"
              ? undefined
              : item.status === "failed"
                ? t("client:compose.outbox.failed", { reason: item.message })
                : t("client:compose.outbox.unsent")
          }
        >
          <Button
            size="sm"
            onPress={() => {
              onRetry(item.idempotencyKey);
            }}
          >
            {t("client:compose.outbox.resend")}
          </Button>
          {canEdit && (
            <Button
              size="sm"
              variant="ghost"
              onPress={() => {
                onEdit(item.idempotencyKey);
              }}
            >
              {t("client:compose.outbox.edit")}
            </Button>
          )}
          <Button
            size="sm"
            variant="ghost"
            onPress={() => {
              onDiscard(item.idempotencyKey);
            }}
          >
            {t("client:compose.outbox.discard")}
          </Button>
        </OutboxItem>
      ))}
    </Stack>
  );
}
