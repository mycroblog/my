// SPDX-License-Identifier: AGPL-3.0-only
// What the webhook screens share (ADR-0060): the labels of the events, the fields of a webhook's settings, and the
// secret shown once after it is made.
import { Button, Checkbox, Grid, Hint, InlineError, Stack, Text, TextField } from "@my/ui";
import { useId } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { webhookDescriptionLimit, type WebhookEventType, webhookEvents, type WebhookMessageType, webhookUrlLimit } from "../api/webhooks";
import { type ShownSecret, useCopySecret, type WebhookDraft, type WebhookFieldError, webhookName } from "../hooks/useWebhooks";

export const webhookEventLabels: Readonly<Record<WebhookMessageType, ClientMessageKey>> = {
  "report.created": "client:admin.webhooks.event.report_created",
  "report.resolved": "client:admin.webhooks.event.report_resolved",
  "report.reopened": "client:admin.webhooks.event.report_reopened",
  "registration.pending": "client:admin.webhooks.event.registration_pending",
  "account.created": "client:admin.webhooks.event.account_created",
  "account.updated": "client:admin.webhooks.event.account_updated",
  "moderation_action.created": "client:admin.webhooks.event.moderation_action_created",
  "moderation_action.lifted": "client:admin.webhooks.event.moderation_action_lifted",
  "appeal.created": "client:admin.webhooks.event.appeal_created",
  "domain_moderation_action.created": "client:admin.webhooks.event.domain_moderation_action_created",
  "domain_moderation_action.lifted": "client:admin.webhooks.event.domain_moderation_action_lifted",
  "status.created": "client:admin.webhooks.event.status_created",
  "custom_emoji.created": "client:admin.webhooks.event.custom_emoji_created",
  "custom_emoji.updated": "client:admin.webhooks.event.custom_emoji_updated",
  "custom_emoji.deleted": "client:admin.webhooks.event.custom_emoji_deleted",
  "error_group.created": "client:admin.webhooks.event.error_group_created",
  "error_group.regressed": "client:admin.webhooks.event.error_group_regressed",
  "webhook.test": "client:admin.webhooks.event.webhook_test",
};

type WebhookFieldsProps = {
  draft: WebhookDraft;
  change: (change: Partial<WebhookDraft>) => void;
  /** The server's refusal, shown on its field. */
  error: WebhookFieldError | undefined;
  autoFocus?: boolean;
};

/** The URL, the description and the events of a webhook. */
export function WebhookFields({ draft, change, error, autoFocus = false }: WebhookFieldsProps) {
  const t = useT();
  const eventsId = useId();
  const eventsHintId = useId();
  const eventsErrorId = useId();
  const fieldError = (field: WebhookFieldError["field"]) => (error?.field === field ? { isInvalid: true, errorMessage: error.message } : {});
  const toggle = (event: WebhookEventType, selected: boolean) => {
    change({ events: selected ? webhookEvents.filter((e) => e === event || draft.events.includes(e)) : draft.events.filter((e) => e !== event) });
  };
  return (
    <>
      <TextField
        type="url"
        label={t("client:admin.webhooks.url")}
        description={t("client:admin.webhooks.url.description")}
        value={draft.url}
        onChange={(url) => {
          change({ url });
        }}
        isRequired
        maxLength={webhookUrlLimit}
        autoComplete="off"
        spellCheck="false"
        autoFocus={autoFocus}
        {...fieldError("url")}
      />
      <TextField
        label={t("client:admin.webhooks.description")}
        description={t("client:admin.webhooks.description.description", { max: String(webhookDescriptionLimit) })}
        value={draft.description}
        onChange={(description) => {
          change({ description });
        }}
        maxLength={webhookDescriptionLimit}
        autoComplete="off"
        {...fieldError("description")}
      />
      <Stack gap="2"
        role="group"
        aria-labelledby={eventsId}
        aria-describedby={error?.field === "events" ? `${eventsHintId} ${eventsErrorId}` : eventsHintId}>
        <Text size="sm" tone="fg" weight="medium" id={eventsId}>
          {t("client:admin.webhooks.events")}
        </Text>
        <Hint id={eventsHintId}>
          {t("client:admin.webhooks.events.description")}
        </Hint>
        <Grid at="md" gap="2">
          {webhookEvents.map((event) => (
            <Checkbox
              key={event}
              isSelected={draft.events.includes(event)}
              onChange={(selected) => {
                toggle(event, selected);
              }}
            >
              {t(webhookEventLabels[event])}
            </Checkbox>
          ))}
        </Grid>
        {error?.field === "events" && (
          <Stack gap="1" tone="danger" textSize="sm" id={eventsErrorId} role="alert">
            <p>{error.message}</p>
            {error.missing.length > 0 && <p>{t("client:admin.webhooks.missing_permissions", { permissions: `⁦${error.missing.join(", ")}⁩` })}</p>}
          </Stack>
        )}
      </Stack>
      {error?.field === "form" && (
        <InlineError size="inherit" alert>
          {t("client:admin.webhooks.save_failed", { reason: error.message })}
        </InlineError>
      )}
    </>
  );
}

/** A secret, shown once: the webhook it is for, the secret to copy, and that it is not shown again. */
export function SecretNotice({ shown }: { shown: ShownSecret }) {
  const t = useT();
  const copy = useCopySecret();
  const labelId = useId();
  return (
    <Stack gap="2" border="md" padding="3">
      <Text as="p" weight="medium" id={labelId}>
        {t("client:admin.webhooks.secret.title", { name: webhookName(shown.webhook) })}
      </Text>
      <Hint>{t("client:admin.webhooks.secret.notice")}</Hint>
      <Text as="p" wrap="anywhere" mono selectAll aria-labelledby={labelId} dir="ltr">
        {shown.secret}
      </Text>
      <div>
        <Button
          size="sm"
          onPress={() => {
            copy(shown.secret);
          }}
        >
          {t("client:admin.webhooks.secret.copy")}
        </Button>
      </div>
    </Stack>
  );
}
