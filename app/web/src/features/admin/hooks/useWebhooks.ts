// SPDX-License-Identifier: AGPL-3.0-only
// The webhooks of the admin screens (ADR-0060, manage_webhooks): the list, setting one up, changing, enabling,
// disabling, deleting it, giving it a new secret and sending it a test; and the deliveries of one, with sending one
// again. A new secret is kept to show once, until the next change. What each change did is announced and kept in a
// StatusMessage; a refused setting stays next to its field.
import { useNotice, useToast } from "@my/ui";
import { useMutation, useQueryClient, useSuspenseInfiniteQuery, useSuspenseQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { type PagedPages, toPagedList } from "../../moderation";
import {
  createWebhookMutation,
  deleteWebhookMutation,
  redeliverWebhookMutation,
  refreshWebhookDeliveries,
  refreshWebhooks,
  rotateWebhookSecretMutation,
  setWebhookEnabledMutation,
  testWebhookMutation,
  updateWebhookMutation,
  type Webhook,
  type WebhookChange,
  webhookDeliveriesQueryOptions,
  type WebhookDelivery,
  type WebhookEventType,
  webhookProblem,
  webhookQueryOptions,
  webhooksQueryOptions,
} from "../api/webhooks";

/** The host name of a webhook's URL, or the URL itself when it does not parse. */
export function webhookHost(url: string): string {
  try {
    return new URL(url).host;
  } catch {
    return url;
  }
}

/**
 * A webhook for a sentence: its description, or the host name of its URL, isolated from the text around it
 * (U+2068 FIRST STRONG ISOLATE or U+2066 LEFT-TO-RIGHT ISOLATE … U+2069 POP DIRECTIONAL ISOLATE).
 */
export function webhookName(webhook: Pick<Webhook, "description" | "url">): string {
  return webhook.description === undefined || webhook.description === ""
    ? `⁦${webhookHost(webhook.url)}⁩`
    : `⁨${webhook.description}⁩`;
}

/** The webhooks that are not deleted, newest first. */
export function useWebhooks(): Webhook[] {
  const api = useApi();
  return useSuspenseQuery(webhooksQueryOptions(api)).data.items;
}

/** One webhook. */
export function useWebhook(id: string): Webhook {
  const api = useApi();
  return useSuspenseQuery(webhookQueryOptions(api, id)).data;
}

/** The fields of a webhook that the server may refuse, and the rest of the form. */
type WebhookField = "url" | "description" | "events" | "form";

/** Why the server refused the settings of a webhook: the field, the server's localized reason, and the permissions it names. */
export type WebhookFieldError = { field: WebhookField; message: string; missing: string[] };

/** Which field a failed change of a webhook is about. */
function fieldError(error: unknown): WebhookFieldError {
  const problem = webhookProblem(error);
  const message = errorMessage(error);
  if (problem === undefined) return { field: "form", message, missing: [] };
  const field: WebhookField =
    problem.reason === "missing_permission" || problem.reason === "events" ? "events" : (problem.reason ?? "form");
  return { field, message, missing: problem.missing };
}

/** The settings of a webhook as the form edits them. */
export type WebhookDraft = { url: string; description: string; events: readonly WebhookEventType[] };

const emptyDraft: WebhookDraft = { url: "", description: "", events: [] };

/** The draft of an existing webhook. */
export function draftOf(webhook: Webhook): WebhookDraft {
  return { url: webhook.url, description: webhook.description ?? "", events: webhook.events };
}

/** A secret shown once: the webhook it signs for, and the secret. */
export type ShownSecret = { webhook: Webhook; secret: string };

/** The form of a new webhook. A success empties it and keeps the new secret to show once. */
export function useWebhookForm() {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<WebhookDraft>(emptyDraft);
  const [created, setCreated] = useState<ShownSecret | undefined>(undefined);
  const mutation = useMutation({
    ...createWebhookMutation(api),
    onMutate: () => {
      setCreated(undefined);
    },
    onSuccess: async ({ webhook, secret }) => {
      await refreshWebhooks(queryClient);
      setDraft(emptyDraft);
      setCreated({ webhook, secret });
    },
  });
  return {
    draft,
    change: (change: Partial<WebhookDraft>) => {
      setDraft((previous) => ({ ...previous, ...change }));
    },
    submit: (event: SyntheticEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (mutation.isPending) return;
      const description = draft.description.trim();
      mutation.mutate({
        url: draft.url.trim(),
        events: [...draft.events],
        ...(description === "" ? {} : { description }),
      });
    },
    isPending: mutation.isPending,
    /** Why setting it up failed, next to its field; the draft stays. */
    error: mutation.isError ? fieldError(mutation.error) : undefined,
    /** The new webhook and its secret, until the next submit. */
    created,
    /** What the last submit did, for the form's StatusMessage. */
    status:
      created === undefined ? undefined : t("client:admin.webhooks.created", { name: webhookName(created.webhook) }),
  };
}

/** Copies a secret to the clipboard and says whether that worked in a toast. */
export function useCopySecret(): (secret: string) => void {
  const t = useT();
  const toast = useToast();
  return (secret) => {
    navigator.clipboard.writeText(secret).then(
      () => {
        // The toast is announced too.
        toast.show({ message: t("client:admin.webhooks.secret.copied"), tone: "success" });
      },
      () => {
        toast.show({ message: t("client:admin.webhooks.secret.copy_failed"), tone: "danger" });
      },
    );
  };
}

export type WebhookActions = {
  /** Changes a webhook's settings; resolves with the refused field, or undefined once it is saved. */
  update: (webhook: Webhook, change: WebhookChange) => Promise<WebhookFieldError | undefined>;
  /** Enables or disables a webhook. */
  setEnabled: (webhook: Webhook, enabled: boolean) => void;
  /** Gives a webhook a new secret, kept in `secret` to show once. */
  rotate: (webhook: Webhook) => Promise<boolean>;
  /** Sends a test message to a webhook. */
  test: (webhook: Webhook) => void;
  /** Deletes a webhook; resolves with whether it succeeded. */
  remove: (webhook: Webhook) => Promise<boolean>;
  /** The webhook being changed. */
  pendingId: string | undefined;
  /** Why the last change failed, localized, until the next change. */
  error: string | undefined;
  /** The new secret of the last rotation, until the next change. */
  secret: ShownSecret | undefined;
  /** What the last change did, for the list's StatusMessage. */
  status: string | undefined;
};

/**
 * Changing the webhooks of the list. `onDeleted(id)` is called once a deleted webhook is about to leave the list (to
 * keep focus, lib/useFocusAfterRemoval); a changed webhook keeps its row, which shows the answer.
 */
export function useWebhookActions(onDeleted: (id: string) => void): WebhookActions {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const [status, setStatus] = useState<string | undefined>(undefined);
  const [secret, setSecret] = useState<ShownSecret | undefined>(undefined);
  const report = (message: string) => {
    setStatus(message);
    notice.inPlace(message);
  };
  const update = useMutation(updateWebhookMutation(api));
  const enable = useMutation(setWebhookEnabledMutation(api));
  const rotate = useMutation(rotateWebhookSecretMutation(api));
  const test = useMutation(testWebhookMutation(api));
  const remove = useMutation(deleteWebhookMutation(api));
  const mutations = [update, enable, rotate, test, remove] as const;
  const resetAll = () => {
    for (const mutation of mutations) mutation.reset();
    setSecret(undefined);
  };
  const pendingId = update.isPending
    ? update.variables.id
    : enable.isPending
      ? enable.variables.id
      : rotate.isPending
        ? rotate.variables
        : test.isPending
          ? test.variables
          : remove.isPending
            ? remove.variables
            : undefined;
  const failed = mutations.find((mutation) => mutation.isError);
  return {
    update: async (webhook, change) => {
      resetAll();
      try {
        const saved = await update.mutateAsync({ id: webhook.id, change });
        await refreshWebhooks(queryClient);
        report(t("client:admin.webhooks.updated", { name: webhookName(saved) }));
        return undefined;
      } catch (error) {
        return fieldError(error);
      }
    },
    setEnabled: (webhook, enabled) => {
      resetAll();
      enable.mutate(
        { id: webhook.id, enabled },
        {
          onSuccess: (saved) => {
            void refreshWebhooks(queryClient).then(() => {
              report(
                t(enabled ? "client:admin.webhooks.enabled_status" : "client:admin.webhooks.disabled_status", {
                  name: webhookName(saved),
                }),
              );
            });
          },
        },
      );
    },
    rotate: async (webhook) => {
      resetAll();
      try {
        const next = await rotate.mutateAsync(webhook.id);
        await refreshWebhooks(queryClient);
        setSecret({ webhook, secret: next });
        report(t("client:admin.webhooks.rotated", { name: webhookName(webhook) }));
        return true;
      } catch {
        return false;
      }
    },
    test: (webhook) => {
      resetAll();
      test.mutate(webhook.id, {
        onSuccess: () => {
          void refreshWebhookDeliveries(queryClient, webhook.id).then(() => {
            report(t("client:admin.webhooks.tested", { name: webhookName(webhook) }));
          });
        },
      });
    },
    remove: async (webhook) => {
      resetAll();
      try {
        await remove.mutateAsync(webhook.id);
      } catch {
        return false;
      }
      onDeleted(webhook.id);
      await refreshWebhooks(queryClient);
      report(t("client:admin.webhooks.deleted", { name: webhookName(webhook) }));
      return true;
    },
    pendingId,
    error: failed === undefined ? undefined : errorMessage(failed.error),
    secret,
    status,
  };
}

/** The deliveries of a webhook, newest first. */
export function useWebhookDeliveries(id: string): PagedPages<WebhookDelivery> {
  const api = useApi();
  return toPagedList(useSuspenseInfiniteQuery(webhookDeliveriesQueryOptions(api, id)));
}

/** Sending the deliveries of a webhook again; what it did and why it failed are kept for the list's StatusMessage. */
export function useRedeliver(webhookId: string) {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const [status, setStatus] = useState<string | undefined>(undefined);
  const mutation = useMutation({
    ...redeliverWebhookMutation(api),
    onMutate: () => {
      setStatus(undefined);
    },
    onSuccess: async () => {
      await refreshWebhookDeliveries(queryClient, webhookId);
      const message = t("client:admin.webhooks.deliveries.redelivered");
      setStatus(message);
      notice.inPlace(message);
    },
  });
  return {
    redeliver: (delivery: WebhookDelivery) => {
      mutation.mutate({ id: webhookId, deliveryId: delivery.id });
    },
    pendingId: mutation.isPending ? mutation.variables.deliveryId : undefined,
    error: mutation.isError ? errorMessage(mutation.error) : undefined,
    status,
  };
}
