// SPDX-License-Identifier: AGPL-3.0-only
// The webhooks screen below its heading (ADR-0060, manage_webhooks), in three pages (web-client/14-admin.md
// 画面の分け方):
// - the list (`WebhookSettings`): the webhooks, newest first, each with its name linking to its page, its state (and
//   why the server disabled it, and its failed attempts) and when it was last changed; above it "Add a webhook" opens
//   the form.
// - the form (`NewWebhook`): the URL, the description and the events. Its secret is shown only once, so setting one
//   up stays on the page: the form gives way to the secret, with a copy button and the link to the webhook's page.
// - the page of one webhook (`WebhookDetail`): its facts, the form of its settings (saved together, the result next to
//   the button), its actions (enabling or disabling it, a test message, a new secret shown once, and deleting it after
//   an alert dialog, which goes back to the list with a toast), and its deliveries (WebhookDeliveries.tsx).
// What an action did stays next to the actions in a StatusMessage (role="status"), and a failure stays there too
// (role="alert").
import {
  Button,
  ButtonLink,
  Card,
  Cluster,
  ConfirmDialog,
  DescriptionItem,
  DescriptionList,
  DialogTrigger,
  EmptyState,
  FormActionBar,
  type FormActionResult,
  InlineError,
  Link,
  Stack,
  StatusMessage,
  Text,
  useToast,
} from "@my/ui";
import { useNavigate } from "@tanstack/react-router";
import { type SubmitEvent, useState } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { DateTime, Section, type RecordColumns, RecordTable, RequirePermission, wholeList } from "../../moderation";
import type { Webhook, WebhookStateReason } from "../api/webhooks";
import {
  draftOf,
  useWebhook,
  useWebhookActions,
  useWebhookForm,
  useWebhooks,
  type WebhookActions,
  type WebhookDraft,
  type WebhookFieldError,
  webhookHost,
  webhookName,
} from "../hooks/useWebhooks";
import { SecretNotice, webhookEventLabels, WebhookFields } from "./webhookParts";
import { WebhookDeliveryList } from "./WebhookDeliveries";

const reasonLabels: Readonly<Record<WebhookStateReason, ClientMessageKey>> = {
  admin: "client:admin.webhooks.reason.admin",
  gone: "client:admin.webhooks.reason.gone",
  failing: "client:admin.webhooks.reason.failing",
};

/** The list of the webhooks, with the link to the form that adds one. */
export function WebhookSettings() {
  const t = useT();
  return (
    <RequirePermission permission="manage_webhooks">
      <Stack gap="6">
        <div>
          <ButtonLink href="/admin/webhooks/new" variant="primary">
            {t("client:admin.webhooks.new.title")}
          </ButtonLink>
        </div>
        <Section surface="card" title={t("client:admin.webhooks.list.title")}>
          <WebhookList />
        </Section>
      </Stack>
    </RequirePermission>
  );
}

function WebhookList() {
  const t = useT();
  const webhooks = useWebhooks();
  const columns: RecordColumns<Webhook> = [
    {
      id: "webhook",
      header: t("client:admin.webhooks.webhook"),
      wrap: "anywhere",
      cell: (webhook) => (
        <Link href={`/admin/webhooks/${encodeURIComponent(webhook.id)}`} wrap="anywhere">
          <WebhookTitle webhook={webhook} />
        </Link>
      ),
    },
    { id: "state", header: t("client:admin.webhooks.state"), cell: (webhook) => <WebhookState webhook={webhook} /> },
    {
      id: "updated_at",
      header: t("client:admin.webhooks.updated_at"),
      cell: (webhook) => <DateTime value={webhook.revised_at} />,
    },
  ];
  return (
    <RecordTable
      list={wholeList(webhooks)}
      caption={t("client:admin.webhooks.list.title")}
      columns={columns}
      getId={(webhook) => webhook.id}
      headingLevel={3}
      empty={
        <EmptyState
          headingLevel={3}
          heading={t("client:admin.webhooks.empty")}
          description={t("client:admin.webhooks.empty.description")}
        />
      }
    />
  );
}

/** A webhook's name: its description, or the host name of its URL. */
function WebhookTitle({ webhook }: { webhook: Webhook }) {
  return webhook.description === undefined || webhook.description === "" ? (
    <span dir="ltr">{webhookHost(webhook.url)}</span>
  ) : (
    <span dir="auto">{webhook.description}</span>
  );
}

function WebhookState({ webhook }: { webhook: Webhook }) {
  const t = useT();
  const enabled = webhook.state === "enabled";
  return (
    <Stack gap="1">
      <span>{t(enabled ? "client:admin.webhooks.state.enabled" : "client:admin.webhooks.state.disabled")}</span>
      {webhook.state === "disabled" && webhook.state_reason !== "admin" && (
        <Text size="sm" tone="warning">
          {t(reasonLabels[webhook.state_reason])}
        </Text>
      )}
      {webhook.failure !== undefined && (
        <Text size="sm" tone="warning">
          {t("client:admin.webhooks.failure.count", { count: webhook.failure.consecutive_failures })}
        </Text>
      )}
    </Stack>
  );
}

/** The form that sets up a webhook; once it is set up, its secret in place of the form. */
export function NewWebhook() {
  return (
    <RequirePermission permission="manage_webhooks">
      <NewWebhookForm />
    </RequirePermission>
  );
}

function NewWebhookForm() {
  const t = useT();
  const form = useWebhookForm();
  if (form.created !== undefined) {
    return (
      <Card>
        <Stack gap="4">
          <StatusMessage>{form.status}</StatusMessage>
          <SecretNotice shown={form.created} />
          <div>
            <ButtonLink href={`/admin/webhooks/${encodeURIComponent(form.created.webhook.id)}`} variant="primary">
              {t("client:admin.webhooks.open")}
            </ButtonLink>
          </div>
        </Stack>
      </Card>
    );
  }
  return (
    <Card>
      <Stack as="form" container gap="6" onSubmit={form.submit}>
        <WebhookFields draft={form.draft} change={form.change} error={form.error} />
        <FormActionBar>
          <Button type="submit" variant="primary" isPending={form.isPending}>
            {t("client:admin.webhooks.submit")}
          </Button>
        </FormActionBar>
      </Stack>
    </Card>
  );
}

/** The page of one webhook: its facts, its settings, its actions and its deliveries. */
export function WebhookDetail({ id }: { id: string }) {
  return (
    <RequirePermission permission="manage_webhooks">
      <WebhookPage id={id} />
    </RequirePermission>
  );
}

function WebhookPage({ id }: { id: string }) {
  const t = useT();
  const webhook = useWebhook(id);
  const navigate = useNavigate();
  const toast = useToast();
  const actions = useWebhookActions((deleted) => {
    void navigate({ to: "/admin/webhooks" });
    // The toast is announced too.
    toast.show({ message: t("client:admin.webhooks.deleted", { name: webhookName(deleted) }), tone: "success" });
  });
  return (
    <Stack gap="8">
      <Section surface="card" title={<WebhookTitle webhook={webhook} />}>
        <Card>
          <WebhookFacts webhook={webhook} />
        </Card>
      </Section>
      <Section surface="card" title={t("client:admin.webhooks.details.title")}>
        <Card>
          <EditWebhookForm webhook={webhook} actions={actions} />
        </Card>
      </Section>
      <Section surface="card" title={t("client:admin.webhooks.manage.title")}>
        <Card>
          <WebhookActionsPanel webhook={webhook} actions={actions} />
        </Card>
      </Section>
      <Section surface="card" title={t("client:admin.webhooks.deliveries.title")}>
        <WebhookDeliveryList id={id} />
      </Section>
    </Stack>
  );
}

/** The facts of a webhook: its URL, events, state and failures. */
function WebhookFacts({ webhook }: { webhook: Webhook }) {
  const t = useT();
  return (
    <DescriptionList columns={2}>
      <DescriptionItem term={t("client:admin.webhooks.url")}>
        <Text wrap="anywhere" mono dir="ltr">
          {webhook.url}
        </Text>
      </DescriptionItem>
      <DescriptionItem term={t("client:admin.webhooks.events")}>
        {webhook.events.map((event) => t(webhookEventLabels[event])).join(t("client:admin.webhooks.events.separator"))}
      </DescriptionItem>
      <DescriptionItem term={t("client:admin.webhooks.state")}>
        <Cluster as="span" gap="1" rowGap="0">
          <span>
            {t(
              webhook.state === "enabled"
                ? "client:admin.webhooks.state.enabled"
                : "client:admin.webhooks.state.disabled",
            )}
          </span>
          {webhook.state === "disabled" && webhook.state_reason !== "admin" && (
            <Text tone="warning">{t(reasonLabels[webhook.state_reason])}</Text>
          )}
        </Cluster>
      </DescriptionItem>
      {webhook.failure !== undefined && (
        <DescriptionItem term={t("client:admin.webhooks.failure")}>
          <Cluster as="span" gap="1" rowGap="0">
            <Text tone="warning">
              {t("client:admin.webhooks.failure.count", { count: webhook.failure.consecutive_failures })}
            </Text>
            <span>{t("client:admin.webhooks.failure.since")}</span>
            <DateTime value={webhook.failure.first_failed_at} />
          </Cluster>
        </DescriptionItem>
      )}
      <DescriptionItem term={t("client:admin.webhooks.updated_at")}>
        <DateTime value={webhook.revised_at} />
      </DescriptionItem>
    </DescriptionList>
  );
}

/** The settings of a webhook, saved together; a refusal stays on its field. */
function EditWebhookForm({ webhook, actions }: { webhook: Webhook; actions: WebhookActions }) {
  const t = useT();
  const [draft, setDraft] = useState<WebhookDraft>(() => draftOf(webhook));
  const [isSaving, setIsSaving] = useState(false);
  const [error, setError] = useState<WebhookFieldError | undefined>(undefined);
  const [result, setResult] = useState<FormActionResult | undefined>(undefined);
  const onSubmit = (event: SubmitEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (isSaving) return;
    setIsSaving(true);
    setError(undefined);
    setResult(undefined);
    void actions
      .update(webhook, { url: draft.url.trim(), description: draft.description.trim(), events: [...draft.events] })
      .then((refused) => {
        setIsSaving(false);
        if (refused === undefined) {
          setResult({ tone: "success", message: t("client:admin.webhooks.updated", { name: webhookName(webhook) }) });
          return;
        }
        setError(refused);
        if (refused.field === "form") {
          setResult({ tone: "danger", message: t("client:admin.webhooks.change_failed", { reason: refused.message }) });
        }
      });
  };
  return (
    <Stack as="form" container gap="6" onSubmit={onSubmit}>
      <WebhookFields
        draft={draft}
        change={(change) => {
          setResult(undefined);
          setDraft((previous) => ({ ...previous, ...change }));
        }}
        error={error}
      />
      <FormActionBar result={result}>
        <Button type="submit" variant="primary" isPending={isSaving}>
          {t("client:admin.webhooks.save")}
        </Button>
      </FormActionBar>
    </Stack>
  );
}

/** Enabling or disabling a webhook, a test message, a new secret, and deleting it; what they did above them. */
function WebhookActionsPanel({ webhook, actions }: { webhook: Webhook; actions: WebhookActions }) {
  const t = useT();
  const isPending = actions.pendingId === webhook.id;
  const enabled = webhook.state === "enabled";
  return (
    <Stack gap="4">
      <StatusMessage>{actions.status}</StatusMessage>
      {actions.error !== undefined && (
        <InlineError size="inherit" alert>
          {t("client:admin.webhooks.change_failed", { reason: actions.error })}
        </InlineError>
      )}
      {actions.secret !== undefined && <SecretNotice shown={actions.secret} />}
      <Cluster gap="2" align="center">
        {/* One button that disables or enables, so that focus stays on it when the state changes. */}
        <Button
          isPending={isPending}
          onPress={() => {
            actions.setEnabled(webhook, !enabled);
          }}
        >
          {t(enabled ? "client:admin.webhooks.disable" : "client:admin.webhooks.enable")}
        </Button>
        <Button
          isDisabled={isPending || !enabled}
          onPress={() => {
            actions.test(webhook);
          }}
        >
          {t("client:admin.webhooks.test")}
        </Button>
        <RotateSecret webhook={webhook} actions={actions} isPending={isPending} />
        <DeleteWebhook webhook={webhook} actions={actions} isPending={isPending} />
      </Cluster>
    </Stack>
  );
}

type ActionProps = { webhook: Webhook; actions: WebhookActions; isPending: boolean };

/** Giving a webhook a new secret, after a dialog: the receiver must take it within a day. */
function RotateSecret({ webhook, actions, isPending }: ActionProps) {
  const t = useT();
  const name = webhookName(webhook);
  const [isRotating, setIsRotating] = useState(false);
  const [failed, setFailed] = useState(false);
  return (
    <DialogTrigger>
      <Button isDisabled={isPending}>{t("client:admin.webhooks.rotate")}</Button>
      <ConfirmDialog
        tone="primary"
        title={t("client:admin.webhooks.rotate.title", { name })}
        description={t("client:admin.webhooks.rotate.description")}
        confirmLabel={t("client:admin.webhooks.rotate")}
        cancelLabel={t("client:admin.webhooks.cancel")}
        isPending={isRotating}
        {...(failed &&
          actions.error !== undefined && {
            errorMessage: t("client:admin.webhooks.change_failed", { reason: actions.error }),
          })}
        onConfirm={(close) => {
          setIsRotating(true);
          setFailed(false);
          void actions.rotate(webhook).then((rotated) => {
            setIsRotating(false);
            if (rotated) close();
            else setFailed(true);
          });
        }}
      />
    </DialogTrigger>
  );
}

/** Deleting a webhook, after an alert dialog: it cannot be undone. */
function DeleteWebhook({ webhook, actions, isPending }: ActionProps) {
  const t = useT();
  const name = webhookName(webhook);
  const [isDeleting, setIsDeleting] = useState(false);
  const [failed, setFailed] = useState(false);
  return (
    <DialogTrigger>
      <Button variant="danger" isDisabled={isPending}>
        {t("client:admin.webhooks.delete")}
      </Button>
      <ConfirmDialog
        title={t("client:admin.webhooks.delete.title", { name })}
        description={t("client:admin.webhooks.delete.description")}
        confirmLabel={t("client:admin.webhooks.delete")}
        cancelLabel={t("client:admin.webhooks.cancel")}
        isPending={isDeleting}
        {...(failed &&
          actions.error !== undefined && {
            errorMessage: t("client:admin.webhooks.change_failed", { reason: actions.error }),
          })}
        onConfirm={(close) => {
          setIsDeleting(true);
          setFailed(false);
          void actions.remove(webhook).then((deleted) => {
            setIsDeleting(false);
            if (deleted) close();
            else setFailed(true);
          });
        }}
      />
    </DialogTrigger>
  );
}
