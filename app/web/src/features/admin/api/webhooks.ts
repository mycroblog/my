// SPDX-License-Identifier: AGPL-3.0-only
// The queries and mutations of the webhooks screens (web-client/README.md S1, ADR-0060, manage_webhooks): the endpoints
// (GET /admin/webhooks), one of them (GET /admin/webhooks/{id}) and its deliveries, newest first; setting one up,
// changing, enabling, disabling and deleting it, giving it a new secret, sending it a test message, and sending a
// delivery again. A webhook that breaks a rule is a 422 `invalid-webhook` problem whose `reason` names the field.
// Every change but the test and the redelivery is recorded in the audit log.
import { infiniteQueryOptions, type QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";
import { ApiError } from "../../../lib/errors";
import { pageQuery, refreshAuditLog } from "../../moderation";

type Schemas = components["schemas"];

export type Webhook = Schemas["AdminWebhook"];
export type WebhookEventType = Schemas["WebhookEventType"];
export type WebhookMessageType = Schemas["WebhookMessageType"];
export type WebhookDelivery = Schemas["AdminWebhookDelivery"];
export type WebhookDeliveryStatus = Schemas["AdminWebhookDeliveryStatus"];
export type WebhookAttemptOutcome = Schemas["AdminWebhookAttemptOutcome"];
export type WebhookStateReason = Schemas["AdminWebhookStateReason"];
export type NewWebhook = Schemas["CreateWebhookRequest"];
export type WebhookChange = Schemas["UpdateWebhookRequest"];
export type WebhookProblemReason = Schemas["WebhookProblemReason"];

/** The problem type of a webhook that breaks a rule. */
const invalidWebhookProblem = "/api/v1/problems/invalid-webhook";

/** The longest URL and description (CreateWebhookRequest). */
export const webhookUrlLimit = 2048;
export const webhookDescriptionLimit = 200;

/** The events a webhook subscribes to, in the order of the form (WebhookEventType). */
export const webhookEvents = [
  "report.created",
  "report.resolved",
  "report.reopened",
  "registration.pending",
  "account.created",
  "account.updated",
  "moderation_action.created",
  "moderation_action.lifted",
  "appeal.created",
  "domain_moderation_action.created",
  "domain_moderation_action.lifted",
  "status.created",
  "custom_emoji.created",
  "custom_emoji.updated",
  "custom_emoji.deleted",
] as const satisfies readonly WebhookEventType[];

/** The webhooks that are not deleted, newest first. */
export function webhooksQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/admin/webhooks");
}

/** One webhook. */
export function webhookQueryOptions(api: Api, id: string) {
  return api.query.queryOptions("get", "/admin/webhooks/{id}", { params: { path: { id } } });
}

/** The deliveries of a webhook, newest first. */
export function webhookDeliveriesQueryOptions(api: Api, id: string) {
  return infiniteQueryOptions({
    queryKey: ["get", "/admin/webhooks/{id}/deliveries", { id }, "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const { data, error } = await api.fetch.GET("/admin/webhooks/{id}/deliveries", { params: { path: { id }, query: pageQuery(pageParam) }, signal });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** The reason of an `invalid-webhook` problem and the permissions it names, or undefined for another error. */
export function webhookProblem(error: unknown): { reason: WebhookProblemReason | undefined; missing: string[] } | undefined {
  if (!(error instanceof ApiError) || error.type !== invalidWebhookProblem) return undefined;
  const reason = error.document["reason"];
  const missing = error.document["missing_permissions"];
  return {
    reason: typeof reason === "string" ? (reason as WebhookProblemReason) : undefined,
    missing: Array.isArray(missing) ? missing.filter((p): p is string => typeof p === "string") : [],
  };
}

/** Sets up a webhook (POST /admin/webhooks), as the options of useMutation. Returns it with its secret. */
export function createWebhookMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/webhooks"] as const,
    mutationFn: async (body: NewWebhook): Promise<Schemas["AdminWebhookCreated"]> => {
      const { data, error } = await api.fetch.POST("/admin/webhooks", { body });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Changes a webhook (PATCH /admin/webhooks/{id}), as the options of useMutation. */
export function updateWebhookMutation(api: Api) {
  return {
    mutationKey: ["patch", "/admin/webhooks/{id}"] as const,
    mutationFn: async ({ id, change }: { id: string; change: WebhookChange }): Promise<Webhook> => {
      const { data, error } = await api.fetch.PATCH("/admin/webhooks/{id}", { params: { path: { id } }, body: change });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Enables or disables a webhook, as the options of useMutation. */
export function setWebhookEnabledMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/webhooks/{id}/enable"] as const,
    mutationFn: async ({ id, enabled }: { id: string; enabled: boolean }): Promise<Webhook> => {
      const { data, error } = enabled
        ? await api.fetch.POST("/admin/webhooks/{id}/enable", { params: { path: { id } } })
        : await api.fetch.POST("/admin/webhooks/{id}/disable", { params: { path: { id } } });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Deletes a webhook, as the options of useMutation. */
export function deleteWebhookMutation(api: Api) {
  return {
    mutationKey: ["delete", "/admin/webhooks/{id}"] as const,
    mutationFn: async (id: string): Promise<void> => {
      // The middleware throws for every error status; a 204 has no body to read.
      await api.fetch.DELETE("/admin/webhooks/{id}", { params: { path: { id } } });
    },
  };
}

/** Gives a webhook a new secret, as the options of useMutation. Returns the secret, shown only now. */
export function rotateWebhookSecretMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/webhooks/{id}/rotate_secret"] as const,
    mutationFn: async (id: string): Promise<string> => {
      const { data, error } = await api.fetch.POST("/admin/webhooks/{id}/rotate_secret", { params: { path: { id } } });
      if (data === undefined) throw new ApiError(error);
      return data.secret;
    },
  };
}

/** Sends a test message to a webhook, as the options of useMutation. Returns its delivery. */
export function testWebhookMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/webhooks/{id}/test"] as const,
    mutationFn: async (id: string): Promise<WebhookDelivery> => {
      const { data, error } = await api.fetch.POST("/admin/webhooks/{id}/test", { params: { path: { id } } });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Sends a delivery of a webhook again, as the options of useMutation. */
export function redeliverWebhookMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/webhooks/{id}/deliveries/{delivery_id}/redeliver"] as const,
    mutationFn: async ({ id, deliveryId }: { id: string; deliveryId: string }): Promise<WebhookDelivery> => {
      const { data, error } = await api.fetch.POST("/admin/webhooks/{id}/deliveries/{delivery_id}/redeliver", {
        params: { path: { id, delivery_id: deliveryId } },
      });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/**
 * Marks what a change of a webhook changes (ADR-0045): the list and the webhook, whose state or settings the
 * answer shows, and the audit log; the queries shown now are fetched again and awaited.
 */
export async function refreshWebhooks(queryClient: QueryClient): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: ["get", "/admin/webhooks"] }),
    queryClient.invalidateQueries({ queryKey: ["get", "/admin/webhooks/{id}"] }),
    refreshAuditLog(queryClient),
  ]);
}

/** A new delivery (a test or a redelivery): the deliveries of the webhook are fetched again. */
export async function refreshWebhookDeliveries(queryClient: QueryClient, id: string): Promise<void> {
  await queryClient.invalidateQueries({ queryKey: ["get", "/admin/webhooks/{id}/deliveries", { id }] });
}
