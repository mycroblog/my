// SPDX-License-Identifier: AGPL-3.0-only
// The webhooks of the instance as a holder of `manage_webhooks` keeps them (FR-722, ADR-0060): the endpoints, one
// endpoint, setting one up (its secret answered once, returned to the caller and never kept in the cache), changing
// its URL, description and events, enabling and disabling it, deleting it, giving it a new secret (shown once too),
// sending it a test, and its deliveries of the last 14 days a page at a time, any of which is sent again. An event
// needs a permission of its own besides `manage_webhooks` (`webhookEventPermission`). Every write reads again what it
// changes: the list, the webhook, and its deliveries.
import { ProblemError, type components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { pagedQuery, usePagedList, type PagedList } from "./paged";
import { useAppData } from "./provider";
import { readOf, type ScreenRead } from "./reads";
import type { Permission } from "./rights";

/** A webhook endpoint, without its secret. */
export type AdminWebhook = components["schemas"]["AdminWebhook"];
/** An event a webhook subscribes to. */
export type WebhookEvent = components["schemas"]["WebhookEventType"];
/** A message sent to a webhook, with its attempts. */
export type WebhookDelivery = components["schemas"]["AdminWebhookDelivery"];
/** Why the server refused a webhook. */
export type WebhookRefusal = components["schemas"]["WebhookProblemReason"];
/** A new webhook. */
export type NewWebhook = components["schemas"]["CreateWebhookRequest"];
/** A change of a webhook; what it leaves out stays. */
export type WebhookChange = components["schemas"]["UpdateWebhookRequest"];

/** The events, by what they are about, in the server's order. */
export const webhookEventGroups = [
  {
    id: "reports",
    events: ["report.created", "report.resolved", "report.reopened"],
  },
  {
    id: "accounts",
    events: [
      "registration.pending",
      "account.created",
      "account.updated",
      "moderation_action.created",
      "moderation_action.lifted",
      "appeal.created",
    ],
  },
  {
    id: "domains",
    events: ["domain_moderation_action.created", "domain_moderation_action.lifted"],
  },
  { id: "posts", events: ["status.created"] },
  {
    id: "emoji",
    events: ["custom_emoji.created", "custom_emoji.updated", "custom_emoji.deleted"],
  },
  { id: "errors", events: ["error_group.created", "error_group.regressed"] },
] as const satisfies readonly { id: string; events: readonly WebhookEvent[] }[];

/** Every event, in the server's order. */
export const webhookEvents: readonly WebhookEvent[] = webhookEventGroups.flatMap((g) => g.events);

/** The permission an event needs besides `manage_webhooks`; none for `status.created`. */
export function webhookEventPermission(event: WebhookEvent): Permission | undefined {
  if (event.startsWith("report.")) return "manage_reports";
  if (event.startsWith("domain_moderation_action.")) return "manage_domains";
  if (
    event === "registration.pending" ||
    event.startsWith("account.") ||
    event.startsWith("moderation_action.") ||
    event === "appeal.created"
  )
    return "manage_accounts";
  if (event.startsWith("custom_emoji.")) return "manage_custom_emojis";
  if (event.startsWith("error_group.")) return "manage_errors";
  return undefined;
}

/** Whether a reader holding `permissions` may subscribe to `event` or stop it. */
export const mayHandleEvent = (event: WebhookEvent, permissions: readonly Permission[]): boolean => {
  const needed = webhookEventPermission(event);
  return needed === undefined || permissions.includes(needed);
};

/** The rules of a webhook's fields, as the server checks them. */
export const webhookLimits = { url: 2048, description: 200 } as const;

/** The webhooks, newest first. */
export function adminWebhooksQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/admin/webhooks");
}

/** The webhook of `id`. */
export function adminWebhookQuery({ queries }: AppData, id: string) {
  return queries.queryOptions("get", "/admin/webhooks/{id}", {
    params: { path: { id } },
  });
}

/** The deliveries of the webhook of `id`, newest first, a page at a time. */
export function webhookDeliveriesQuery({ api, queries }: AppData, id: string) {
  return pagedQuery<WebhookDelivery>(
    queries.queryOptions("get", "/admin/webhooks/{id}/deliveries", {
      params: { path: { id }, query: {} },
    }).queryKey,
    async (cursor, signal) =>
      (
        await api.GET("/admin/webhooks/{id}/deliveries", {
          params: {
            path: { id },
            query: cursor === undefined ? {} : { cursor },
          },
          signal,
        })
      ).data,
  );
}

/** Reads the webhooks, for a route's loader. */
export async function loadAdminWebhooks(data: AppData): Promise<void> {
  await data.queryClient.query(adminWebhooksQuery(data)).catch(() => undefined);
}

/** Reads the webhook of `id` and its first page of deliveries, for a route's loader. */
export async function loadAdminWebhook(data: AppData, id: string): Promise<void> {
  await Promise.all([
    data.queryClient.query(adminWebhookQuery(data, id)).catch(() => undefined),
    data.queryClient.infiniteQuery(webhookDeliveriesQuery(data, id)).catch(() => undefined),
  ]);
}

/** The webhooks, newest first, and where their read is. */
export function useAdminWebhooks(): ScreenRead & {
  readonly webhooks: readonly AdminWebhook[] | undefined;
} {
  const query = useQuery(adminWebhooksQuery(useAppData()));
  return { ...readOf([query]), webhooks: query.data?.items };
}

/** The webhook of `id`; `missing` once the server says it has none (404). */
export function useAdminWebhook(id: string): ScreenRead & {
  readonly webhook: AdminWebhook | undefined;
  readonly missing: boolean;
} {
  const query = useQuery(adminWebhookQuery(useAppData(), id));
  const missing = query.error instanceof ProblemError && query.error.status === 404;
  return { ...readOf([query]), webhook: query.data, missing };
}

/** The deliveries of the webhook of `id`, newest first. */
export function useWebhookDeliveries(id: string): PagedList<WebhookDelivery> {
  return usePagedList(webhookDeliveriesQuery(useAppData(), id));
}

/** Why the server refused a webhook (`reason` of a 422 `invalid-webhook`), with the permissions it lacks; else none. */
export function webhookRefusal(
  error: unknown,
): { readonly reason: WebhookRefusal; readonly missing: readonly Permission[] } | undefined {
  if (!(error instanceof ProblemError) || error.status !== 422) return undefined;
  const reason: unknown = error.problem["reason"];
  if (typeof reason !== "string") return undefined;
  const missing: unknown = error.problem["missing_permissions"];
  return {
    reason: reason as WebhookRefusal,
    missing: Array.isArray(missing) ? (missing as Permission[]) : [],
  };
}

/** The writes of webhooks; each rejects with the server's problem (`webhookRefusal` says a 422's reason). */
export type AdminWebhookWrites = {
  /** Sets up a webhook, and resolves with it and its secret, which the server shows only now. */
  readonly create: (webhook: NewWebhook) => Promise<{ readonly webhook: AdminWebhook; readonly secret: string }>;
  /** Changes the webhook of `id`; an empty description removes it. */
  readonly change: (id: string, change: WebhookChange) => Promise<AdminWebhook>;
  readonly enable: (id: string) => Promise<AdminWebhook>;
  readonly disable: (id: string) => Promise<AdminWebhook>;
  /** Deletes the webhook of `id` for good. */
  readonly remove: (id: string) => Promise<void>;
  /** Gives the webhook of `id` a new secret, and resolves with it, which the server shows only now. */
  readonly rotate: (id: string) => Promise<string>;
  /** Sends the webhook of `id` a test message, and resolves with its delivery. */
  readonly test: (id: string) => Promise<WebhookDelivery>;
  /** Sends the delivery of `deliveryId` again, and resolves with it. */
  readonly redeliver: (id: string, deliveryId: string) => Promise<WebhookDelivery>;
};

/** The writes of webhooks, for `data`. */
export function adminWebhookWrites(data: AppData): AdminWebhookWrites {
  const path = (id: string) => ({ params: { path: { id } } });
  /** The list, the webhook and its deliveries read again; the webhook the server answered put in the cache first. */
  const changed = async (id: string, webhook?: AdminWebhook): Promise<void> => {
    const key = adminWebhookQuery(data, id).queryKey;
    if (webhook !== undefined) {
      await data.queryClient.cancelQueries({ queryKey: key });
      data.queryClient.setQueryData(key, webhook);
    } else void data.queryClient.invalidateQueries({ queryKey: key });
    void data.queryClient.invalidateQueries({
      queryKey: webhookDeliveriesQuery(data, id).queryKey,
    });
    await data.queryClient.invalidateQueries({
      queryKey: adminWebhooksQuery(data).queryKey,
    });
  };
  const answered = async (id: string, webhook: AdminWebhook | undefined): Promise<AdminWebhook> => {
    if (webhook === undefined) throw new Error("app-data: a write of a webhook answered without the webhook");
    await changed(id, webhook);
    return webhook;
  };
  const delivered = async (id: string, delivery: WebhookDelivery | undefined): Promise<WebhookDelivery> => {
    if (delivery === undefined) throw new Error("app-data: a delivery answered without the delivery");
    await changed(id);
    return delivery;
  };
  return {
    create: async (body) => {
      const { data: made } = await data.api.POST("/admin/webhooks", { body });
      if (made === undefined) throw new Error("app-data: a new webhook answered without the webhook");
      // The secret goes back to the screen alone; the cache holds the webhook without it.
      data.queryClient.setQueryData(adminWebhookQuery(data, made.webhook.id).queryKey, made.webhook);
      await data.queryClient.invalidateQueries({
        queryKey: adminWebhooksQuery(data).queryKey,
      });
      return { webhook: made.webhook, secret: made.secret };
    },
    change: async (id, body) =>
      answered(id, (await data.api.PATCH("/admin/webhooks/{id}", { ...path(id), body })).data),
    enable: async (id) => answered(id, (await data.api.POST("/admin/webhooks/{id}/enable", path(id))).data),
    disable: async (id) => answered(id, (await data.api.POST("/admin/webhooks/{id}/disable", path(id))).data),
    remove: async (id) => {
      await data.api.DELETE("/admin/webhooks/{id}", path(id));
      data.queryClient.removeQueries({
        queryKey: webhookDeliveriesQuery(data, id).queryKey,
      });
      await data.queryClient.invalidateQueries({
        queryKey: adminWebhooksQuery(data).queryKey,
      });
    },
    rotate: async (id) => {
      const { data: answer } = await data.api.POST("/admin/webhooks/{id}/rotate_secret", path(id));
      if (answer === undefined) throw new Error("app-data: a new secret answered without the secret");
      void changed(id);
      return answer.secret;
    },
    test: async (id) => delivered(id, (await data.api.POST("/admin/webhooks/{id}/test", path(id))).data),
    redeliver: async (id, deliveryId) =>
      delivered(
        id,
        (
          await data.api.POST("/admin/webhooks/{id}/deliveries/{delivery_id}/redeliver", {
            params: { path: { id, delivery_id: deliveryId } },
          })
        ).data,
      ),
  };
}

/** The writes of webhooks, for a screen. */
export function useAdminWebhookWrites(): AdminWebhookWrites {
  return adminWebhookWrites(useAppData());
}
